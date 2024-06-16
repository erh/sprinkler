package sprinkler

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"go.viam.com/rdk/components/board"
	"go.viam.com/rdk/components/sensor"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
	"go.viam.com/utils"
)

var SprinklerModel = resource.ModelNamespace("erh").WithFamily("sprinkler").WithModel("sprinkler")

type ZoneConfig struct {
	Pin      string
	Minutes  int
	Priority int
}

type sprinklerConfig struct {
	Board     string
	StartHour int    `json:"start_hour"`
	DataDir   string `json:"data_dir"`
	Zones     map[string]ZoneConfig
	Lat       string
	Long      string
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

func newSprinkler(ctx context.Context, deps resource.Dependencies, config resource.Config, logger logging.Logger) (sensor.Sensor, error) {
	newConf, err := resource.NativeConfig[*sprinklerConfig](config)
	if err != nil {
		return nil, err
	}

	s := &sprinkler{config: newConf, name: config.ResourceName(), logger: logger}
	err = s.init()
	if err != nil {
		return nil, err
	}

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
	go RunServer(ctx, logger, ":9999", s)

	return s, nil
}

type sprinkler struct {
	resource.AlwaysRebuild

	config *sprinklerConfig
	name   resource.Name
	logger logging.Logger

	backgroundContext context.Context
	backgroundCancel  context.CancelFunc

	theBoard board.Board
	pins     map[string]board.GPIOPin

	statsLock     sync.Mutex
	stats         DataAPI
	running       string // what sprinkler is running now
	lastLoop      time.Time
	pauseTillTime time.Time
	forceZone     string
	forceTill     time.Time

	lastRainCheck time.Time
}

func (s *sprinkler) init() error {
	s.pins = map[string]board.GPIOPin{}
	var err error
	if s.config.DataDir == "" {
		s.config.DataDir = "sprinkler_data"
	}

	err = os.MkdirAll(s.config.DataDir, os.ModePerm)
	if err != nil {
		return err
	}

	s.stats, err = NewLocalJSONStore(s.config.DataDir)
	if err != nil {
		return err
	}
	return nil
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

		if !utils.SelectContextOrWait(s.backgroundContext, 1*time.Second) {
			s.logger.Errorf("stopping sprinkler")
			return
		}

	}
}

const (
	rainTooSoon int = 1
	rainDone        = 2
	rainNotConf     = 3
	rainDidIt       = 4
)

func (s *sprinkler) doRainPrediction_inlock(now time.Time) (int, error) {

	if now.Sub(s.lastRainCheck) < (time.Minute * 10) {
		return rainTooSoon, nil
	}
	s.lastRainCheck = now

	amt, err := s.stats.AmountWatered("rain_sensor", now)
	if err != nil {
		return 0, err
	}

	if amt > 0 {
		return rainDone, nil
	}

	if s.config.Lat == "" || s.config.Long == "" {
		return rainNotConf, nil
	}

	rain, maxTempReal, err := rainPrediction(s.config.Lat, s.config.Long, 24)
	if err != nil {
		return 0, err
	}

	fmt.Printf("weather rain: %v temp: %v\n", rain, maxTempReal)

	tempAdjust := maxTempReal - 21
	if tempAdjust < 0 {
		tempAdjust = 0
	}
	tempAdjust = tempAdjust / 10 // so 90f gets you about 10c diff, so maxTemp here is 2

	for _, n := range s.config.zoneOrder() {
		z := s.config.Zones[n]

		totalToAdd := time.Duration(0)

		if rain > 0 {
			toAdd := time.Duration(float64(time.Minute) * float64(z.Minutes) * rain / 10)
			totalToAdd += toAdd
			fmt.Printf("remove %v minutes to zone %v because it rained (%v)\n", toAdd, n, rain)
		}

		if tempAdjust > 0 {
			toAdd := time.Duration(tempAdjust * float64(z.Minutes) * float64(time.Minute))
			totalToAdd -= toAdd
			fmt.Printf("adding %v minutes to zone %v because it's hot (%v)\n", toAdd, n, maxTempReal)
		}
		_, err = s.stats.AddWatered(n, now, totalToAdd)
		if err != nil {
			return 0, err
		}

	}

	s.stats.AddWatered("rain_sensor", now, time.Second+time.Duration(rain*float64(time.Minute)))
	return rainDidIt, nil
}

func (s *sprinkler) doLoop(ctx context.Context, now time.Time) error {

	s.statsLock.Lock()

	if s.running != "" { // note: this has to be first
		amount := now.Sub(s.lastLoop)
		fmt.Printf("adding %v to %v\n", amount, s.running)
		_, err := s.stats.AddWatered(s.running, now, amount)
		if err != nil {
			return err
		}
	}
	s.lastLoop = now

	_, err := s.doRainPrediction_inlock(now)
	if err != nil {
		s.logger.Warnf("cannot do rain prediction %v", err)
	}

	if now.Before(s.forceTill) && s.forceZone != "" {
		z := s.forceZone
		s.running = s.forceZone
		s.statsLock.Unlock()

		s.logger.Infof("forcing zone %s till %v", z, s.forceTill)
		return s.stopAllExcept(ctx, z)
	}

	if now.Before(s.pauseTillTime) {
		s.running = ""
		s.statsLock.Unlock()
		s.logger.Infof("paused till %v", s.pauseTillTime)
		return s.stopAllExcept(ctx, "")
	}

	if now.Hour() < 1 || now.Hour() < s.config.StartHour {
		s.running = ""
		s.lastLoop = now
		s.statsLock.Unlock()
		return s.stopAllExcept(ctx, "")
	}

	prev := s.running
	s.running = s.pickNext_inlock(now)
	s.statsLock.Unlock()

	if prev == s.running {
		return nil
	}

	return s.stopAllExcept(ctx, s.running)
}

func (s *sprinkler) pickNext_inlock(now time.Time) string {
	names := s.config.zoneOrder()
	for _, n := range names {
		z := s.config.Zones[n]
		d, err := s.stats.AmountWatered(n, now)
		if err != nil {
			panic(err)
		}

		min := float64(z.Minutes)

		if min >= d.Minutes() {
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

	if cmdName == "run" {
		min, ok := cmd["minutes"].(float64)
		if !ok {
			return nil, fmt.Errorf("pause command requires a 'minutes' param that is an float64, got [%v] an %T", cmd["minutes"], cmd["minutes"])
		}
		t := time.Now().Add(time.Duration(float64(time.Minute) * min))
		z, ok := cmd["zone"].(string)
		if !ok {
			return nil, fmt.Errorf("zone isn't a string")
		}

		s.statsLock.Lock()
		s.forceZone = z
		s.forceTill = t
		s.statsLock.Unlock()

		return map[string]interface{}{"till": t}, nil
	}

	if cmdName == "markZoneTime" {
		min, ok := cmd["minutes"].(float64)
		if !ok {
			return nil, fmt.Errorf("pause command requires a 'minutes' param that is an float64, got [%v] an %T", cmd["minutes"], cmd["minutes"])
		}

		z, ok := cmd["zone"].(string)
		if !ok {
			return nil, fmt.Errorf("zone isn't a string")
		}

		s.statsLock.Lock()
		_, err := s.stats.AddWatered(z, time.Now(), time.Duration((float64(time.Minute) * min)))
		s.statsLock.Unlock()

		return map[string]interface{}{}, err
	}

	return nil, fmt.Errorf("sprinkler do command doesn't understand cmd [%s]", cmdName)
}

func (s *sprinkler) Readings(ctx context.Context, extra map[string]interface{}) (map[string]interface{}, error) {
	m := map[string]interface{}{}

	now := time.Now()

	s.statsLock.Lock()
	defer s.statsLock.Unlock()

	for _, n := range s.config.zoneOrder() {
		v, err := s.stats.AmountWatered(n, now)
		if err != nil {
			return nil, err
		}
		m[n] = v.Minutes()
		m[fmt.Sprintf("%s-configured", n)] = s.config.Zones[n].Minutes
	}
	m["running"] = s.running

	if time.Now().Before(s.pauseTillTime) {
		m["pause_till"] = s.pauseTillTime.Format(time.UnixDate)
	} else {
		m["pause_till"] = ""
	}

	m["force_zone"] = s.forceZone
	m["force_till"] = s.forceTill.Format(time.UnixDate)

	v, err := s.stats.AmountWatered("rain", now)
	if err != nil {
		return nil, err
	}
	m["rain"] = v.Minutes()

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
