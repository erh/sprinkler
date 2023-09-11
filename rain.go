package sprinkler

import (
	"fmt"
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
