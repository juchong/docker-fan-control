package services

import (
	"math"
	"testing"

	"docker-fan-control/internal/models"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestParamFloat(t *testing.T) {
	p := models.AlgorithmParams{"min_temp": 40.0}
	if got := paramFloat(p, "min_temp", 30); got != 40 {
		t.Errorf("paramFloat present = %v, want 40", got)
	}
	if got := paramFloat(p, "missing", 12.5); got != 12.5 {
		t.Errorf("paramFloat default = %v, want 12.5", got)
	}
}

func TestIsLoadInput(t *testing.T) {
	for _, tt := range []struct {
		typ  string
		want bool
	}{
		{models.InputTypeGPULoad, true},
		{models.InputTypeCPULoad, true},
		{models.InputTypeGPUTemp, false},
		{models.InputTypeCPUTemp, false},
		{models.InputTypeBoardTemp, false},
	} {
		if got := isLoadInput(tt.typ); got != tt.want {
			t.Errorf("isLoadInput(%q) = %v, want %v", tt.typ, got, tt.want)
		}
	}
}

func TestProfileInputAxis(t *testing.T) {
	linear := &models.Profile{Algorithm: "linear", AlgorithmParams: models.AlgorithmParams{"min_temp": 30.0, "max_temp": 80.0}}
	if lo, hi := profileInputAxis(linear); lo != 30 || hi != 80 {
		t.Errorf("linear axis = (%v,%v), want (30,80)", lo, hi)
	}

	// Degenerate range falls back to 30..80.
	bad := &models.Profile{Algorithm: "linear", AlgorithmParams: models.AlgorithmParams{"min_temp": 80.0, "max_temp": 30.0}}
	if lo, hi := profileInputAxis(bad); lo != 30 || hi != 80 {
		t.Errorf("degenerate axis = (%v,%v), want (30,80)", lo, hi)
	}

	step := &models.Profile{Algorithm: "step", AlgorithmParams: models.AlgorithmParams{
		"steps": []any{
			map[string]any{"temp": 40.0, "speed": 30.0},
			map[string]any{"temp": 75.0, "speed": 100.0},
		},
	}}
	if lo, hi := profileInputAxis(step); lo != 40 || hi != 75 {
		t.Errorf("step axis = (%v,%v), want (40,75)", lo, hi)
	}

	pid := &models.Profile{Algorithm: "pid", AlgorithmParams: models.AlgorithmParams{"setpoint": 70.0}}
	if lo, hi := profileInputAxis(pid); lo != 55 || hi != 85 {
		t.Errorf("pid axis = (%v,%v), want (55,85)", lo, hi)
	}
}

func linearProfile(agg string, inputs ...models.ProfileInput) *models.Profile {
	return &models.Profile{
		Algorithm: "linear",
		AlgorithmParams: models.AlgorithmParams{
			"min_temp": 30.0, "max_temp": 80.0, "min_speed": 30.0, "max_speed": 100.0,
			"input_aggregation": agg,
		},
		Inputs: inputs,
	}
}

func TestCalculateInputValueTempOnlyUnchanged(t *testing.T) {
	c := &FanController{}
	p := linearProfile("max",
		models.ProfileInput{InputType: models.InputTypeGPUTemp, InputIndex: 0, Weight: 1},
		models.ProfileInput{InputType: models.InputTypeGPUTemp, InputIndex: 1, Weight: 1},
	)
	inputs := map[string]float64{"gpu_temp0": 37, "gpu_temp1": 32}
	if got := c.calculateInputValue(p, inputs); got != 37 {
		t.Errorf("temp-only max = %v, want 37 (unchanged by projection)", got)
	}
}

func TestCalculateInputValueLoadProjection(t *testing.T) {
	c := &FanController{}
	p := linearProfile("max", models.ProfileInput{InputType: models.InputTypeGPULoad, InputIndex: 0, Weight: 1})

	// axis 30..80: 0% → 30, 50% → 55, 100% → 80.
	for _, tt := range []struct{ load, want float64 }{{0, 30}, {50, 55}, {100, 80}} {
		got := c.calculateInputValue(p, map[string]float64{"gpu_load0": tt.load})
		if !approx(got, tt.want) {
			t.Errorf("load %v%% projected = %v, want %v", tt.load, got, tt.want)
		}
	}
}

// Regression for the live "GPU" profile: mixing GPU load and temp under weighted
// aggregation must lift the value above the min-temp floor (was ~21.6 before the
// projection fix, which pinned fans at min speed and ignored load).
func TestCalculateInputValueMixedWeightedRegression(t *testing.T) {
	c := &FanController{}
	p := linearProfile("weighted",
		models.ProfileInput{InputType: models.InputTypeGPULoad, InputIndex: 0, Weight: 0.6},
		models.ProfileInput{InputType: models.InputTypeGPULoad, InputIndex: 1, Weight: 0.6},
		models.ProfileInput{InputType: models.InputTypeGPUTemp, InputIndex: 0, Weight: 1.0},
		models.ProfileInput{InputType: models.InputTypeGPUTemp, InputIndex: 1, Weight: 1.0},
	)

	idle := c.calculateInputValue(p, map[string]float64{
		"gpu_load0": 0, "gpu_load1": 0, "gpu_temp0": 37, "gpu_temp1": 32,
	})
	// (30*0.6 + 30*0.6 + 37 + 32) / 3.2 = 105/3.2 = 32.8125
	if !approx(idle, 105.0/3.2) {
		t.Errorf("idle mixed value = %v, want %v", idle, 105.0/3.2)
	}
	if idle <= 30 {
		t.Errorf("idle value %v should exceed min_temp 30 (regression: load dragged it below the floor)", idle)
	}

	loaded := c.calculateInputValue(p, map[string]float64{
		"gpu_load0": 100, "gpu_load1": 100, "gpu_temp0": 37, "gpu_temp1": 32,
	})
	if !(loaded > idle) {
		t.Errorf("loaded value %v should exceed idle %v — profile must react to load", loaded, idle)
	}
}

func TestCalculateInputValueNoInputs(t *testing.T) {
	c := &FanController{}
	p := linearProfile("max")
	if got := c.calculateInputValue(p, map[string]float64{"gpu_temp0": 50}); got != 0 {
		t.Errorf("no configured inputs = %v, want 0", got)
	}
}
