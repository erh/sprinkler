package sprinkler

import (
	"context"
	"os"
	"testing"
	"time"

	"go.viam.com/rdk/components/board"
	"go.viam.com/rdk/components/board/fake"
	"go.viam.com/rdk/logging"

	"go.viam.com/test"
)

var testSimpleConfig = sprinklerConfig{
	Zones: map[string]ZoneConfig{
		"a": {Minutes: 10},
		"b": {Minutes: 20},
		"c": {Minutes: 5},
	},
}

func addDummyPins(s *sprinkler) func() {
	dir, err := os.MkdirTemp("", "sp_test")
	if err != nil {
		panic(err)
	}
	s.config.DataDir = dir
	s.init()
	s.pins = map[string]board.GPIOPin{}
	for n := range s.config.Zones {
		s.pins[n] = &fake.GPIOPin{}
	}
	return func() { os.RemoveAll(dir) }
}

func TestPickNext(t *testing.T) {
	s := sprinkler{config: &testSimpleConfig}
	f := addDummyPins(&s)
	defer f()
	test.That(t, s.pickNext_inlock(time.Now()), test.ShouldEqual, "b")
}

func TestLoop1(t *testing.T) {
	ctx := context.Background()
	s := sprinkler{config: &testSimpleConfig, logger: logging.NewTestLogger(t)}
	f := addDummyPins(&s)
	defer f()

	now := time.Now()
	test.That(t, s.doLoop(ctx, now), test.ShouldBeNil)
	test.That(t, "b", test.ShouldEqual, s.running)

	now = now.Add(time.Minute)
	test.That(t, s.doLoop(ctx, now), test.ShouldBeNil)
	test.That(t, "b", test.ShouldEqual, s.running)
	d, _ := s.stats.AmountWatered("b", now)
	test.That(t, time.Minute, test.ShouldAlmostEqual, d)

	now = now.Add(20 * time.Minute)
	test.That(t, s.doLoop(ctx, now), test.ShouldBeNil)
	d, _ = s.stats.AmountWatered("b", now)
	test.That(t, 21*time.Minute, test.ShouldAlmostEqual, d)
	test.That(t, "a", test.ShouldEqual, s.running)

}

func TestOrder(t *testing.T) {
	cfg := sprinklerConfig{
		Zones: map[string]ZoneConfig{
			"a": {Minutes: 10},
			"b": {Minutes: 10},
			"c": {Minutes: 10},
			"d": {Minutes: 10},
			"e": {Minutes: 20},
			"f": {Minutes: 20},
			"g": {Minutes: 20},
			"h": {Minutes: 15, Priority: 2},
			"i": {Minutes: 15, Priority: 2},
			"j": {Minutes: 20, Priority: 2},
			"k": {Minutes: 20, Priority: 2},
			"l": {Minutes: 1, Priority: 3},
		},
	}

	test.That(t, 171, test.ShouldEqual, cfg.totalMinutes())

	test.That(t, cfg.zoneOrder(), test.ShouldResemble, []string{"l", "j", "k", "h", "i", "e", "f", "g", "a", "b", "c", "d"})

}

func TestRainFull(t *testing.T) {
	s := sprinkler{config: &testSimpleConfig}
	f := addDummyPins(&s)
	defer f()

	now := time.Now()

	mode, err := s.doRainPrediction_inlock(now)
	test.That(t, err, test.ShouldBeNil)
	test.That(t, mode, test.ShouldEqual, rainNotConf)

	mode, err = s.doRainPrediction_inlock(now)
	test.That(t, err, test.ShouldBeNil)
	test.That(t, mode, test.ShouldEqual, rainTooSoon)

	s.lastRainCheck = time.UnixMilli(0)
	s.config.Lat = rainMagic
	s.config.Long = rainMagic

	mode, err = s.doRainPrediction_inlock(now)
	test.That(t, err, test.ShouldBeNil)
	test.That(t, mode, test.ShouldEqual, rainDidIt)

	d, _ := s.stats.AmountWatered("b", now)
	test.That(t, d, test.ShouldAlmostEqual, time.Minute*10)

	s.lastRainCheck = time.UnixMilli(0)
	mode, err = s.doRainPrediction_inlock(now)
	test.That(t, err, test.ShouldBeNil)
	test.That(t, mode, test.ShouldEqual, rainDone)

}
