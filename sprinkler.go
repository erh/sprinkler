package sprinkler

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/edaniels/golog"

	"go.viam.com/rdk/components/board"
	"go.viam.com/rdk/components/sensor"
	"go.viam.com/rdk/resource"
	"go.viam.com/utils"
)

var SprinklerModel = resource.DefaultModelFamily.WithModel("sprinkler")

type ZoneConfig struct {
	Pin      string
	Minutes  int
	Priority int
}

type sprinklerConfig struct {
	Board     string
	StartHour int `json:"start_hour"`
	Zones     map[string]ZoneConfig
}

func (cfg sprinklerConfig) Validate(path string) ([]string, error) {
	deps := []string{cfg.Board}

	if cfg.Board == "" {
		return nil, utils.NewConfigValidationFieldRequiredError(path, "board")
	}

	return deps, nil
}

func (cfg sprinklerConfig) totalMinutes() int {
	total := 0
	for _, z := range cfg.Zones {
		total += z.Minutes
	}
	return total
}

func (cfg sprinklerConfig) zoneOrder() []string {
	all := []string{}
	for n := range cfg.Zones {
		all = append(all, n)
	}

	sort.Slice(all, func(i, j int) bool {
		ii := all[i]
		jj := all[j]

		iii := cfg.Zones[ii]
		jjj := cfg.Zones[jj]

		if iii.Priority != jjj.Priority {
			return iii.Priority >= jjj.Priority
		}

		if iii.Minutes != jjj.Minutes {
			return iii.Minutes >= jjj.Minutes
		}

		return strings.Compare(ii, jj) < 0
	})

	return all
}

func init() {
	resource.RegisterComponent(
		sensor.API,
		SprinklerModel,
		resource.Registration[sensor.Sensor, *sprinklerConfig]{
			Constructor: newSprinkler,
		})
}

func newSprinkler(ctx context.Context, deps resource.Dependencies, config resource.Config, logger golog.Logger) (sensor.Sensor, error) {
	newConf, err := resource.NativeConfig[*sprinklerConfig](config)
	if err != nil {
		return nil, err
	}

	s := &sprinkler{config: newConf, name: config.ResourceName(), logger: logger}
	s.init()

	r, err := deps.Lookup(board.Named(s.config.Board))
	if err != nil {
		return nil, err
	}

	s.theBoard = r.(board.Board)

	for name, z := range newConf.Zones {
		p, err := s.theBoard.GPIOPinByName(z.Pin)
		if err != nil {
			return nil, fmt.Errorf("error getting pin (%s)", z.Pin)
		}
		s.pins[name] = p
	}

	go s.run()

	return s, nil
}

type sprinkler struct {
	resource.AlwaysRebuild

	config *sprinklerConfig
	name   resource.Name
	logger golog.Logger

	backgroundContext context.Context
	backgroundCancel  context.CancelFunc

	theBoard board.Board
	pins     map[string]board.GPIOPin

	statsLock     sync.Mutex
	stats         map[string]time.Duration // how many minutes each zone has been running
	running       string                   // what sprinkler is running now
	lastLoop      time.Time
	pauseTillTime time.Time
}

func (s *sprinkler) init() {
	s.pins = map[string]board.GPIOPin{}
	s.stats = map[string]time.Duration{}
}

func (s *sprinkler) Name() resource.Name {
	return s.name
}

func (s *sprinkler) Close(ctx context.Context) error {
	s.backgroundCancel()
	return nil
}

func (s *sprinkler) run() {
	s.backgroundContext, s.backgroundCancel = context.WithCancel(context.Background())

	for {
		err := s.doLoop(s.backgroundContext, time.Now())
		if err != nil {
			s.logger.Errorf("error doing sprinkler loop: %v", err)
		}

		if !utils.SelectContextOrWait(s.backgroundContext, 10*time.Second) {
			s.logger.Errorf("stopping sprinkler")
			return
		}

	}
}

func (s *sprinkler) doLoop(ctx context.Context, now time.Time) error {

	s.statsLock.Lock()

	if now.Before(s.pauseTillTime) {
		s.statsLock.Unlock()
		s.logger.Infof("paused till %v", s.pauseTillTime)
		return s.stopAllExcept(ctx, "")
	}

	if now.Hour() < 1 || now.Hour() < s.config.StartHour {
		for n := range s.stats {
			s.stats[n] = 0
		}
		s.running = ""
		s.lastLoop = now
		s.statsLock.Unlock()
		return s.stopAllExcept(ctx, "")
	}

	if s.running != "" {
		d := s.stats[s.running]
		d += now.Sub(s.lastLoop)
		s.stats[s.running] = d
	}
	s.lastLoop = now

	prev := s.running
	s.running = s.pickNext_inlock()
	s.statsLock.Unlock()

	if prev == s.running {
		return nil
	}

	return s.stopAllExcept(ctx, s.running)
}

func (s *sprinkler) pickNext_inlock() string {
	names := s.config.zoneOrder()

	for _, n := range names {
		z := s.config.Zones[n]
		if float64(z.Minutes) >= s.stats[n].Minutes() {
			return n
		}
	}

	return ""
}

func (s *sprinkler) DoCommand(ctx context.Context, cmd map[string]interface{}) (map[string]interface{}, error) {
	cmdName := cmd["cmd"]
	if cmdName == "order" {
		return map[string]interface{}{"order": s.config.zoneOrder()}, nil
	}

	if cmdName == "pause" {
		min, ok := cmd["minutes"].(float64)
		if !ok {
			return nil, fmt.Errorf("pause command requires a 'minutes' param that is an float64, got [%v] an %T", cmd["minutes"], cmd["minutes"])
		}
		t := time.Now().Add(time.Duration(float64(time.Minute) * min))
		s.statsLock.Lock()
		s.pauseTillTime = t
		s.statsLock.Unlock()
		return map[string]interface{}{"till": t}, nil
	}

	return nil, fmt.Errorf("sprinkler do command doesn't understand cmd [%s]", cmdName)
}

func (s *sprinkler) Readings(ctx context.Context, extra map[string]interface{}) (map[string]interface{}, error) {
	m := map[string]interface{}{}

	s.statsLock.Lock()
	defer s.statsLock.Unlock()

	for n, v := range s.stats {
		m[n] = v.Minutes()
	}
	m["running"] = s.running
	return m, nil
}

func (s *sprinkler) stopAllExcept(ctx context.Context, torun string) error {
	for name := range s.pins {
		if name == torun {
			err := s.zoneOn(ctx, name)
			if err != nil {
				return fmt.Errorf("cannot turn on pin (%s) for zone (%s)", s.config.Zones[name].Pin, name)
			}
		} else {
			err := s.zoneOff(ctx, name)
			if err != nil {
				return fmt.Errorf("cannot turn off pin (%s) for zone (%s)", s.config.Zones[name].Pin, name)
			}
		}
	}
	return nil
}

func (s *sprinkler) zoneOn(ctx context.Context, zone string) error {
	p, ok := s.pins[zone]
	if !ok {
		return fmt.Errorf("why no pin for zone: %s", zone)
	}
	v, err := p.Get(ctx, nil)
	if err != nil {
		return err
	}
	if v == true {
		return nil
	}
	s.logger.Infof("turning zone on %s", zone)
	return p.Set(ctx, true, nil)
}

func (s *sprinkler) zoneOff(ctx context.Context, zone string) error {
	p, ok := s.pins[zone]
	if !ok {
		return fmt.Errorf("why no pin for zone: %s", zone)
	}
	v, err := p.Get(ctx, nil)
	if err != nil {
		return err
	}
	if v == false {
		return nil
	}
	s.logger.Infof("turning zone off %s", zone)
	return p.Set(ctx, false, nil)
}
