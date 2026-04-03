package experiment

import (
	"fmt"
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	expReader := strings.NewReader(`OnPremise yes
					WorkerType H100
					NumWorkers 2
					Upload /home/xyz
					Artifact weights.h5`)
	exp, err := Load(expReader)
	if err != nil {
		t.Error(err)
		return
	}

	fmt.Printf("%+v\n", exp)
}

func TestLoadMissingValue(t *testing.T) {
	expReader := strings.NewReader(`OnPremise yes
					NumWorkers 2
					Upload /home/xyz
					Artifact weights.h5`)
	_, err := Load(expReader)
	if err != ErrMissingValue {
		t.Errorf("expected to get '%s', but got '%s'",
			ErrMissingValue,
			err,
		)
	}
}
