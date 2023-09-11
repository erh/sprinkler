package sprinkler

import (
	"testing"
	"time"

	"go.viam.com/test"
)

func TestRain(t *testing.T) {
	_, err := rain("KJFK", 24)
	test.That(t, err, test.ShouldBeNil)

	var rc rainCache
	_, err = rc.rain("KJFK", 24)
	test.That(t, err, test.ShouldBeNil)
	test.That(t, len(rc.cache), test.ShouldEqual, 1)

	var old time.Time
	for _, v := range rc.cache {
		old = v.when
	}

	_, err = rc.rain("KJFK", 24)
	test.That(t, err, test.ShouldBeNil)
	test.That(t, len(rc.cache), test.ShouldEqual, 1)

	var newer time.Time
	for _, v := range rc.cache {
		newer = v.when
	}

	test.That(t, newer, test.ShouldEqual, old)

}
