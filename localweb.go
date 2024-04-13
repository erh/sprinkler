package sprinkler

import (
	"context"
	_ "embed"
	"fmt"
	"html/template"
	"net/http"
	"strconv"

	"go.viam.com/rdk/components/sensor"
	"go.viam.com/rdk/logging"
)

//go:embed index.html
var indexHtmlBytes []byte

func RunServer(ctx context.Context, logger logging.Logger, bind string, sprinkler sensor.Sensor) error {
	s := &server{
		logger:    logger,
		sprinkler: sprinkler,
	}

	http.Handle("/", s)

	return http.ListenAndServe(bind, nil)
}

type server struct {
	logger    logging.Logger
	sprinkler sensor.Sensor
}

type zoneInfo struct {
	Name         string
	MinutesSoFar float64
	MinutesConf  int
}

type info struct {
	Zones     []zoneInfo
	Running   string
	PauseTill string
	Message   string
}

func coerceorder(v interface{}) []string {
	a, ok := v.([]string)
	if ok {
		return a
	}

	b, ok := v.([]interface{})
	if ok {
		a = []string{}
		for _, x := range b {
			a = append(a, x.(string))
		}
		return a
	}

	panic(1)
}

func (s *server) getData() (*info, error) {
	i := &info{}

	readings, err := s.sprinkler.Readings(context.Background(), map[string]interface{}{})
	if err != nil {
		return nil, err
	}

	i.Running = readings["running"].(string)
	i.PauseTill = readings["pause_till"].(string)

	ordered, err := s.sprinkler.DoCommand(context.Background(), map[string]interface{}{"cmd": "order"})
	if err != nil {
		return nil, err
	}

	for _, x := range coerceorder(ordered["order"]) {
		z := zoneInfo{Name: x}

		minRaw, ok := readings[z.Name]
		if ok {
			z.MinutesSoFar, ok = minRaw.(float64)
			if !ok {
				return nil, fmt.Errorf("got bad minutes value: [%v] type:[%T]", minRaw, minRaw)
			}
		}

		minRaw, ok = readings[z.Name+"-configured"]
		if ok {
			z.MinutesConf, ok = minRaw.(int)
			if !ok {
				xx, ok := minRaw.(float64)
				if !ok {
					return nil, fmt.Errorf("got bad minutes config value: [%v] type: %T", minRaw, minRaw)
				}
				z.MinutesConf = int(xx)
			}
		}

		i.Zones = append(i.Zones, z)
	}

	return i, nil
}

func (s *server) processData(r *http.Request) (string, error) {
	q := r.URL.Query()
	if q.Has("run") {
		z := q.Get("run")
		m, err := strconv.ParseFloat(q.Get("min"), 64)
		if err != nil {
			return "", err
		}

		_, err = s.sprinkler.DoCommand(context.Background(),
			map[string]interface{}{
				"cmd":     "run",
				"zone":    z,
				"minutes": m,
			})

		if err != nil {
			return "", fmt.Errorf("cannot run a zone: %v", err)
		}

		return fmt.Sprintf("running zone %s for %v minutes", z, m), nil
	}

	if q.Has("markZoneTime") {
		z := q.Get("markZoneTime")
		m, err := strconv.ParseFloat(q.Get("min"), 64)
		if err != nil {
			return "", err
		}

		_, err = s.sprinkler.DoCommand(context.Background(),
			map[string]interface{}{
				"cmd":     "markZoneTime",
				"zone":    z,
				"minutes": m,
			})

		if err != nil {
			return "", fmt.Errorf("cannot mark a zone: %v", err)
		}

		return fmt.Sprintf("marking zone done %s for %v minutes", z, m), nil
	}

	if q.Has("pause") {
		m, err := strconv.ParseFloat(q.Get("pause"), 64)
		if err != nil {
			return "", err
		}

		_, err = s.sprinkler.DoCommand(context.Background(),
			map[string]interface{}{
				"cmd":     "pause",
				"minutes": m,
			})

		if err != nil {
			return "", fmt.Errorf("cannot pause %v", err)
		}

		return fmt.Sprintf("paused for %v minutes", m), nil
	}

	return "", nil
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	msg, err := s.processData(r)
	if err != nil {
		http.Error(w, fmt.Sprintf("error processing data %v", err), 500)
		return
	}

	t, err := template.New("foo").Parse(string(indexHtmlBytes))
	if err != nil {
		http.Error(w, fmt.Sprintf("error parsing template %v", err), 500)
		return
	}

	info, err := s.getData()
	if err != nil {
		http.Error(w, fmt.Sprintf("error getting data from sprinkler %v", err), 500)
		return
	}
	info.Message = msg

	err = t.Execute(w, info)
	if err != nil {
		http.Error(w, fmt.Sprintf("error running template %v", err), 500)
		return
	}
}
