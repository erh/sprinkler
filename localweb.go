package sprinkler

import (
	"context"
	_ "embed"
	"fmt"
	"html/template"
	"net/http"

	"github.com/edaniels/golog"
	"go.viam.com/rdk/components/sensor"
)

//go:embed index.html
var indexHtmlBytes []byte

func RunServer(ctx context.Context, logger golog.Logger, bind string, sprinkler sensor.Sensor) error {
	s := &server{
		logger:    logger,
		sprinkler: sprinkler,
	}

	http.Handle("/", s)

	return http.ListenAndServe(bind, nil)
}

type server struct {
	logger    golog.Logger
	sprinkler sensor.Sensor
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	t, err := template.New("foo").Parse(string(indexHtmlBytes))
	if err != nil {
		http.Error(w, fmt.Sprintf("error parsing template %v", err), 500)
		return
	}

	sprinklerReturnValue, err := s.sprinkler.Readings(context.Background(), map[string]interface{}{})
	if err != nil {
		http.Error(w, fmt.Sprintf("error getting data from sprinkler %v", err), 500)
		return
	}
	s.logger.Infof("sprinkler Readings return value: %+v", sprinklerReturnValue)

	err = t.Execute(w, nil)
	if err != nil {
		http.Error(w, fmt.Sprintf("error running template %v", err), 500)
		return
	}
}
