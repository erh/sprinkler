package sprinkler

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type DataAPI interface {
	AmountWatered(z string, now time.Time) (time.Duration, error)

	// returns the total amount watered today
	AddWatered(z string, now time.Time, amountToMark time.Duration) (time.Duration, error)
}

// ----

type durData map[string]time.Duration

type localJSONStore struct {
	root string
	data map[string]time.Duration // how many minutes each zone has been running
}

func NewLocalJSONStore(root string) (DataAPI, error) {
	s := &localJSONStore{root: root}
	s.data = map[string]time.Duration{}
	return s, nil
}

func (s *localJSONStore) fileName(now time.Time) string {
	return filepath.Join(s.root, fmt.Sprintf("data-%d-%02d-%02d.txt", now.Year(), now.Month(), now.Day()))
}

func (s *localJSONStore) readFromDisk(now time.Time) (durData, error) {
	fn := s.fileName(now)

	data, err := os.ReadFile(fn)
	if err != nil {
		if os.IsNotExist(err) {
			return durData{}, nil
		}
		return nil, err
	}

	return dataIn(string(data))
}

func (s *localJSONStore) writeToDisk(now time.Time, dd durData) error {
	fn := s.fileName(now)
	data := dataOut(dd)
	return os.WriteFile(fn, []byte(data), 0666)
}

func (s *localJSONStore) AmountWatered(z string, now time.Time) (time.Duration, error) {
	dd, err := s.readFromDisk(now)
	if err != nil {
		return 0, err
	}

	return dd[z], nil
}

func (s *localJSONStore) AddWatered(z string, now time.Time, amountToMark time.Duration) (time.Duration, error) {
	dd, err := s.readFromDisk(now)
	if err != nil {
		return 0, err
	}

	d := dd[z]
	d += amountToMark
	dd[z] = d

	s.writeToDisk(now, dd)

	return d, nil
}

func dataOut(data durData) string {
	var buffer bytes.Buffer

	for k, v := range data {
		buffer.WriteString(fmt.Sprintf("%s %.02f\n", k, v.Minutes()))
	}

	return buffer.String()
}

func dataIn(raw string) (durData, error) {
	dd := durData{}

	for _, l := range strings.Split(raw, "\n") {
		l = strings.TrimSpace(l)
		if len(l) == 0 {
			continue
		}
		x := strings.Split(l, " ")
		if len(x) != 2 {
			return dd, fmt.Errorf("invalid data line [%s]", l)
		}

		f, err := strconv.ParseFloat(x[1], 64)
		if err != nil {
			return dd, fmt.Errorf("invalid data line [%s]", l)
		}
		dd[x[0]] = time.Duration(f * float64(time.Minute))

	}
	return dd, nil
}
