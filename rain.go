package sprinkler

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/icodealot/noaa"
)

type rainCacheData struct {
	totalRain float64
	when      time.Time
}

type rainCache struct {
	cache map[string]rainCacheData
	lock  sync.Mutex
}

func (rc *rainCache) rain(station string, hours int) (float64, error) {
	rc.lock.Lock()
	defer rc.lock.Unlock()

	key := fmt.Sprintf("%s-%d", station, hours)

	if rc.cache == nil {
		rc.cache = map[string]rainCacheData{}
	} else {
		old := rc.cache[key]
		if time.Since(old.when) < 10*time.Minute {
			return old.totalRain, nil
		}
	}

	r, err := rain(station, hours)
	if err != nil {
		return 0, err
	}

	rc.cache[key] = rainCacheData{r, time.Now()}
	return r, nil
}

func rain(station string, hours int) (float64, error) {
	resp, err := noaa.Observations(station)
	if err != nil {
		return 0, err
	}

	totalRain := 0.0

	for _, o := range resp.Observations {
		if time.Since(o.Timestamp) > (time.Hour * time.Duration(hours)) {
			continue
		}

		totalRain += o.PrecipitationLastHour.Value
	}
	fmt.Printf("station: %s totalRain: %v\n", station, totalRain)
	return totalRain, nil
}

const (
	rainMagic = "rainMagic"
)

// return mm
func rainPrediction(lat, long string, hours int) (float64, error) {
	if lat == rainMagic && long == rainMagic {
		return 5.0, nil
	}

	r, err := noaa.GridpointForecast(lat, long)
	if err != nil {
		return 0, fmt.Errorf("cannot get grid forecast for %v, %v %v", lat, long, err)
	}

	x := r.QuantitativePrecipitation

	if x.Uom != "wmoUnit:mm" {
		return 0, fmt.Errorf("unit is not mm %v", x.Uom)
	}

	total := 0.0
	for _, z := range x.Values {
		pcs := strings.Split(z.ValidTime, "/")
		if len(pcs) != 2 {
			return 0, fmt.Errorf("invalid time %v", z.ValidTime)
		}

		t, err := time.Parse(time.RFC3339, pcs[0])
		if err != nil {
			return 0, fmt.Errorf("bad time %v %v", pcs[0], err)
		}

		if t.Sub(time.Now()).Hours() > float64(hours) {
			continue
		}

		total += z.Value
	}

	return total, nil
}
