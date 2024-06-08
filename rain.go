package sprinkler

import (
	"fmt"
	"strings"
	//"sync"
	"time"

	"github.com/icodealot/noaa"
)

/*
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
*/
const (
	rainMagic = "rainMagic"
)

// hours: the number of hours into the future to look
// return mmOfRain, maxTemp
func rainPrediction(lat, long string, hours int) (float64, float64, error) {
	if lat == rainMagic && long == rainMagic {
		return 5.0, 26.0, nil
	}

	r, err := noaa.GridpointForecast(lat, long)
	if err != nil {
		return 0, 0, fmt.Errorf("cannot get grid forecast for %v, %v %v", lat, long, err)
	}

	x := r.QuantitativePrecipitation

	if x.Uom != "wmoUnit:mm" {
		return 0, 0, fmt.Errorf("unit is not mm %v", x.Uom)
	}

	end := time.Now().Add(time.Duration(hours) * time.Hour)

	total := 0.0
	for _, z := range x.Values {
		t, err := parseTime(z.ValidTime)
		if err != nil {
			return 0, 0, err
		}

		if t.After(end) {
			continue
		}

		total += z.Value
	}

	x = r.Temperature
	if x.Uom != "wmoUnit:degC" {
		return 0, 0, fmt.Errorf("bad unit for time %v", x.Uom)
	}

	maxTemp := 0.0
	for _, z := range x.Values {
		t, err := parseTime(z.ValidTime)
		if err != nil {
			return 0, 0, err
		}

		if t.After(end) {
			continue
		}

		if z.Value > maxTemp {
			maxTemp = z.Value
		}
	}
	fmt.Printf("yo %v\n", maxTemp)
	return total, maxTemp, nil
}

func parseTime(s string) (time.Time, error) {
	pcs := strings.Split(s, "/")
	if len(pcs) != 2 {
		return time.Time{}, fmt.Errorf("invalid time %v", s)
	}

	t, err := time.Parse(time.RFC3339, pcs[0])
	if err != nil {
		return t, fmt.Errorf("bad time %v %v", pcs[0], err)
	}

	return t, nil
}
