package experiment

import (
	"errors"
	"io"
	"os"
	"reflect"
	"strconv"
	"strings"
)

var ErrBadConfig error = errors.New("bad config")
var ErrMissingValue error = errors.New("missing value")

type Experiment struct {
	OnPremise  bool     `exp:"required"`
	WorkerType string   `exp:"required"`
	NumWorkers int      `exp:"required"`
	Uploads    []string
	Artifacts  []string
}

func LoadFile(path string) (*Experiment, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	return Load(f)
}

func Load(reader io.Reader) (*Experiment, error) {
	var result Experiment

	expMap, err := loadMap(reader)
	if err != nil {
		return nil, err
	}

	err = loadReflect(&result, expMap)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

func loadMap(reader io.Reader) (map[string][]string, error) {
	expBytes, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	expMap := make(map[string][]string)
	expLines := strings.SplitSeq(string(expBytes), "\n")

	for line := range expLines {
		fields := strings.Fields(line)

		if len(fields) == 0 {
			continue
		} else if len(fields) != 2 {
			return nil, ErrBadConfig
		}

		expMap[fields[0]] = append(expMap[fields[0]], fields[1])
	}

	return expMap, nil
}

func loadReflect(dest *Experiment, expMap map[string][]string) error {
	expVal := reflect.ValueOf(dest).Elem()
	expFields := expVal.Fields()

	for field, val := range expFields {
		effName := field.Name

		if field.Type.Kind() == reflect.Slice {
			effName = strings.TrimSuffix(effName, "s")
		}

		v, ok := expMap[effName]
		if !ok {
			expTag := field.Tag.Get("exp")
			if expTag == "required" {
				return ErrMissingValue
			} else {
				continue
			}
		}

		switch field.Type.Kind() {
		case reflect.Bool:
			val.SetBool(v[0] == "yes")
		case reflect.String:
			val.SetString(v[0])
		case reflect.Int:
			i, err := strconv.Atoi(v[0])
			if err != nil {
				return ErrBadConfig
			}
			val.SetInt(int64(i))
		case reflect.Slice:
			l := len(v)
			vSlice := reflect.MakeSlice(field.Type, l, l)
			arrKind := field.Type.Elem().Kind()

			for i, vv := range v {
				switch arrKind {
				case reflect.String:
					vSlice.Index(i).SetString(vv)
				}
			}

			val.Set(vSlice)
		}
	}

	return nil
}
