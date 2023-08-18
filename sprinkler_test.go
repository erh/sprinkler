package sprinkler

import (
	"context"
	"testing"
	"time"

	"github.com/edaniels/golog"

	"go.viam.com/rdk/components/board"
	"go.viam.com/rdk/components/board/fake"

	"go.viam.com/test"
)

var testSimpleConfig = sprinklerConfig{
	Zones: map[string]ZoneConfig{
		"a": {Minutes: 10},
		"b": {Minutes: 20},
		"c": {Minutes: 5},
	},
}

func addDummyPins(s *sprinkler) {
	s.init()
	s.pins = map[string]board.GPIOPin{}
	for n := range s.config.Zones {
		s.pins[n] = &fake.GPIOPin{}
	}
}

func TestPickNext(t *testing.T) {
	s := sprinkler{config: &testSimpleConfig}
	test.That(t, "b", test.ShouldEqual, s.pickNext_inlock())
}

func TestLoop1(t *testing.T) {
	ctx := context.Background()
	s := sprinkler{config: &testSimpleConfig, logger: golog.NewTestLogger(t)}
	addDummyPins(&s)

	now := time.Now()
	test.That(t, s.doLoop(ctx, now), test.ShouldBeNil)
	test.That(t, "b", test.ShouldEqual, s.running)

	now = now.Add(time.Minute)
	test.That(t, s.doLoop(ctx, now), test.ShouldBeNil)
	test.That(t, "b", test.ShouldEqual, s.running)
	test.That(t, time.Minute, test.ShouldAlmostEqual, s.stats["b"])

	now = now.Add(20 * time.Minute)
	test.That(t, s.doLoop(ctx, now), test.ShouldBeNil)
	test.That(t, 21*time.Minute, test.ShouldAlmostEqual, s.stats["b"])
	test.That(t, "a", test.ShouldEqual, s.running)

}
