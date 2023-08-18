package sprinkler

import (
	"context"
	"fmt"
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
	Pin     string
	Minutes int
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

	logger.Infof("hi %v", s)

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

	statsLock sync.Mutex
	stats     map[string]time.Duration // how many minutes each zone has been running
	running   string                   // what sprinkler is running now
	lastLoop  time.Time
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
	s.logger.Infof("doLoop now: %v", now)
	if now.Hour() < 1 || now.Hour() < s.config.StartHour {
		s.statsLock.Lock()
		for n := range s.stats {
			s.stats[n] = 0
		}
		s.running = ""
		s.lastLoop = now
		s.statsLock.Unlock()
		return s.stopAll(ctx)
	}

	s.statsLock.Lock()

	if s.running != "" {
		d := s.stats[s.running]
		d += now.Sub(s.lastLoop)
		s.stats[s.running] = d
	}
	s.lastLoop = now

	s.running = s.pickNext_inlock()
	s.statsLock.Unlock()

	err := s.stopAll(ctx)
	if err != nil {
		return err
	}
	if s.running == "" {
		return nil
	}

	return s.zoneOn(ctx, s.running)
}

func (s *sprinkler) pickNext_inlock() string {
	name := ""
	priority := 0

	for n, z := range s.config.Zones {
		if float64(z.Minutes) < s.stats[n].Minutes() {
			continue
		}

		mypriority := z.Minutes
		if mypriority > priority {
			name = n
			priority = mypriority
		}
	}
	return name
}

func (s *sprinkler) stopAll(ctx context.Context) error {
	s.logger.Infof("stopAll")
	for name := range s.pins {
		err := s.zoneOff(ctx, name)
		if err != nil {
			return fmt.Errorf("cannot turn off pin (%s) for zone (%s)", s.config.Zones[name].Pin, name)
		}
	}
	return nil
}

func (s *sprinkler) DoCommand(ctx context.Context, cmd map[string]interface{}) (map[string]interface{}, error) {
	return nil, fmt.Errorf("not implemented")
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

func (s *sprinkler) zoneOn(ctx context.Context, zone string) error {
	s.logger.Infof("zoneOn %s", zone)
	p, ok := s.pins[zone]
	if !ok {
		return fmt.Errorf("why no pin for zone: %s", zone)
	}
	return p.Set(ctx, true, nil)
}

func (s *sprinkler) zoneOff(ctx context.Context, zone string) error {
	return s.pins[zone].Set(ctx, true, nil)
}
