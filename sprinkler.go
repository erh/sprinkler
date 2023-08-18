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
	Pin string
	Minutes int
}

type sprinklerConfig struct {
	Board string
	StartHour int `json:"start_hour"`
	MaxTimeSliceMinutes int `json:"max_time_slice_minutes"`
	Zones map[string]ZoneConfig
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

	s := &sprinkler{config: newConf, name: config.ResourceName()}

	s.pins = map[string]board.GPIOPin{}
	s.stats = map[string]float64{}

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
	
	return s, nil
}

type sprinkler struct {
	resource.AlwaysRebuild
	
	config *sprinklerConfig
	name resource.Name
	
	theBoard board.Board
	pins map[string]board.GPIOPin

	statsLock sync.Mutex
	stats map[string]float64 // how many minutes each zone has been running
	running string // what sprinker is running now
	
}

func (s *sprinkler) Name() resource.Name {
	return s.name
}

func (s *sprinkler) Close(ctx context.Context) error {
	// TODO shut down thread
	return nil
}

func (s *sprinkler) doLoop(ctx context.Context, now time.Time) error {
	if now.Hour() < 1 || now.Hour() < s.config.StartHour {
		s.stats = map[string]float64{}
		s.running = ""
		return s.stopAll(ctx)
	}

	panic(1)
	
	return nil
}

func (s *sprinkler) stopAll(ctx context.Context) error {
	for name, p := range s.pins {
		err := p.Set(ctx, false, nil)
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
		m[n] = v
	}
	m["running"] = s.running

	return m, nil
}
