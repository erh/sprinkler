package sprinkler

import (
	"os"
	"testing"
	"time"

	"go.viam.com/test"
)

func TestDataPrimitives(t *testing.T) {
	dd := durData{
		"a": 150 * time.Second,
	}

	test.That(t, dataOut(dd), test.ShouldEqual, "a 2.50\n")

	dd2, err := dataIn(dataOut(dd))
	test.That(t, err, test.ShouldBeNil)
	test.That(t, dd2, test.ShouldResemble, dd)
}

func TestLocalJSONStore(t *testing.T) {
	dir, err := os.MkdirTemp("", "json_test")
	test.That(t, err, test.ShouldBeNil)
	defer os.RemoveAll(dir)

	s, err := NewLocalJSONStore(dir)
	test.That(t, err, test.ShouldBeNil)

	now := time.Now()

	d, err := s.AmountWatered("a", now)
	test.That(t, err, test.ShouldBeNil)
	test.That(t, d, test.ShouldEqual, 0)

	d, err = s.AddWatered("a", now, time.Minute)
	test.That(t, err, test.ShouldBeNil)
	test.That(t, d, test.ShouldEqual, time.Minute)

	d, err = s.AddWatered("a", now, time.Minute)
	test.That(t, err, test.ShouldBeNil)
	test.That(t, d, test.ShouldEqual, 2*time.Minute)

	d, err = s.AmountWatered("a", now)
	test.That(t, err, test.ShouldBeNil)
	test.That(t, d, test.ShouldEqual, 2*time.Minute)

	// test opening new store
	s2, err := NewLocalJSONStore(dir)
	test.That(t, err, test.ShouldBeNil)

	d, err = s2.AmountWatered("a", now)
	test.That(t, err, test.ShouldBeNil)
	test.That(t, d, test.ShouldEqual, 2*time.Minute)
}

func BenchmarkJSONStore(b *testing.B) {
	dir, err := os.MkdirTemp("", "json_test")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(dir)

	s, err := NewLocalJSONStore(dir)
	if err != nil {
		b.Fatal(err)
	}

	now := time.Now()

	for n := 0; n < b.N; n++ {
		_, err = s.AddWatered("a", now, time.Minute)
		if err != nil {
			b.Fatal(err)
		}
	}
}
