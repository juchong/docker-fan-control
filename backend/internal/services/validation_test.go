package services

import (
	"testing"

	"docker-fan-control/internal/models"
)

// collectFields flattens the nested ValidationError tree into field names.
func collectFields(err error) []string {
	ve, ok := err.(*ValidationError)
	if !ok {
		return nil
	}
	out := []string{ve.Field}
	if details, ok := ve.Details.([]*ValidationError); ok {
		for _, d := range details {
			out = append(out, collectFields(d)...)
		}
	}
	return out
}

func hasField(err error, field string) bool {
	for _, f := range collectFields(err) {
		if f == field {
			return true
		}
	}
	return false
}

// validLinear returns a profile with no zones (so validateZones short-circuits
// before touching the driver registry, which is nil in these tests).
func validLinear() *models.Profile {
	return &models.Profile{
		Name:      "test",
		Algorithm: "linear",
		AlgorithmParams: models.AlgorithmParams{
			"min_temp": 30.0, "max_temp": 80.0, "min_speed": 30.0, "max_speed": 100.0,
		},
		Priority:       0,
		TransitionTime: 10,
		MinRunTime:     30,
		Hysteresis:     2,
	}
}

func TestValidateProfileValid(t *testing.T) {
	v := &ProfileValidator{}
	if err := v.ValidateProfile(validLinear()); err != nil {
		t.Errorf("valid profile rejected: %v", err)
	}
}

func TestValidateProfileNameRequired(t *testing.T) {
	v := &ProfileValidator{}
	p := validLinear()
	p.Name = ""
	if err := v.ValidateProfile(p); !hasField(err, "name") {
		t.Errorf("empty name should fail on 'name': %v", err)
	}
}

func TestValidateProfilePriority(t *testing.T) {
	v := &ProfileValidator{}
	p := validLinear()
	p.Priority = -1
	if !hasField(v.ValidateProfile(p), "priority") {
		t.Error("negative priority should fail")
	}
	p.Priority = 5 // any non-negative is fine
	if err := v.ValidateProfile(p); err != nil {
		t.Errorf("priority 5 should pass: %v", err)
	}
}

func TestValidateProfileTuningBounds(t *testing.T) {
	v := &ProfileValidator{}

	for _, tt := range []struct {
		name  string
		mut   func(*models.Profile)
		field string
	}{
		{"transition too high", func(p *models.Profile) { p.TransitionTime = 400 }, "transition_time"},
		{"transition negative", func(p *models.Profile) { p.TransitionTime = -1 }, "transition_time"},
		{"min run too high", func(p *models.Profile) { p.MinRunTime = 999 }, "min_run_time"},
		{"hysteresis too high", func(p *models.Profile) { p.Hysteresis = 11 }, "hysteresis"},
		{"hysteresis negative", func(p *models.Profile) { p.Hysteresis = -1 }, "hysteresis"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := validLinear()
			tt.mut(p)
			if !hasField(v.ValidateProfile(p), tt.field) {
				t.Errorf("expected error on %q", tt.field)
			}
		})
	}

	// Boundaries are inclusive and valid.
	p := validLinear()
	p.TransitionTime, p.MinRunTime, p.Hysteresis = 300, 0, 10
	if err := v.ValidateProfile(p); err != nil {
		t.Errorf("boundary values should pass: %v", err)
	}
}

func TestValidateLinearParamBounds(t *testing.T) {
	v := &ProfileValidator{}
	for _, tt := range []struct {
		name   string
		params models.AlgorithmParams
	}{
		{"min>=max temp", models.AlgorithmParams{"min_temp": 80.0, "max_temp": 30.0, "min_speed": 30.0, "max_speed": 100.0}},
		{"temp over 150", models.AlgorithmParams{"min_temp": 30.0, "max_temp": 200.0, "min_speed": 30.0, "max_speed": 100.0}},
		{"speed over 100", models.AlgorithmParams{"min_temp": 30.0, "max_temp": 80.0, "min_speed": 30.0, "max_speed": 150.0}},
		{"missing max_temp", models.AlgorithmParams{"min_temp": 30.0, "min_speed": 30.0, "max_speed": 100.0}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := validLinear()
			p.AlgorithmParams = tt.params
			if v.ValidateProfile(p) == nil {
				t.Errorf("expected linear params to be rejected")
			}
		})
	}
}

func TestValidatePIDParamBounds(t *testing.T) {
	v := &ProfileValidator{}
	valid := models.AlgorithmParams{"setpoint": 70.0, "kp": 2.0, "ki": 0.1, "kd": 1.0, "min_speed": 30.0, "max_speed": 100.0}

	p := validLinear()
	p.Algorithm = "pid"
	p.AlgorithmParams = valid
	if err := v.ValidateProfile(p); err != nil {
		t.Errorf("valid PID rejected: %v", err)
	}

	for _, tt := range []struct {
		name   string
		params models.AlgorithmParams
	}{
		{"negative kp", models.AlgorithmParams{"setpoint": 70.0, "kp": -1.0, "ki": 0.1, "kd": 1.0, "min_speed": 30.0, "max_speed": 100.0}},
		{"ki greater than kp", models.AlgorithmParams{"setpoint": 70.0, "kp": 1.0, "ki": 5.0, "kd": 1.0, "min_speed": 30.0, "max_speed": 100.0}},
		{"setpoint over 150", models.AlgorithmParams{"setpoint": 200.0, "kp": 2.0, "ki": 0.1, "kd": 1.0, "min_speed": 30.0, "max_speed": 100.0}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pp := validLinear()
			pp.Algorithm = "pid"
			pp.AlgorithmParams = tt.params
			if v.ValidateProfile(pp) == nil {
				t.Errorf("expected PID params to be rejected")
			}
		})
	}
}

func TestValidateStepParamBounds(t *testing.T) {
	v := &ProfileValidator{}
	p := validLinear()
	p.Algorithm = "step"
	p.AlgorithmParams = models.AlgorithmParams{"steps": []any{
		map[string]any{"temp": 30.0, "speed": 30.0},
		map[string]any{"temp": 200.0, "speed": 100.0}, // temp out of range
	}}
	if v.ValidateProfile(p) == nil {
		t.Error("step temp over 150 should be rejected")
	}
}

func TestValidateInputs(t *testing.T) {
	v := &ProfileValidator{}

	p := validLinear()
	p.Inputs = []models.ProfileInput{{InputType: models.InputTypeGPUTemp, InputIndex: 0, Weight: 1}}
	if err := v.ValidateProfile(p); err != nil {
		t.Errorf("valid input rejected: %v", err)
	}

	p.Inputs = []models.ProfileInput{{InputType: models.InputTypeGPUTemp, InputIndex: 0, Weight: 0}}
	if v.ValidateProfile(p) == nil {
		t.Error("weight 0 should be rejected")
	}

	p.Inputs = []models.ProfileInput{{InputType: "bogus", InputIndex: 0, Weight: 1}}
	if v.ValidateProfile(p) == nil {
		t.Error("invalid input type should be rejected")
	}

	p.Inputs = []models.ProfileInput{
		{InputType: models.InputTypeGPULoad, InputIndex: 0, Weight: 1},
		{InputType: models.InputTypeGPULoad, InputIndex: 0, Weight: 1},
	}
	if v.ValidateProfile(p) == nil {
		t.Error("duplicate inputs should be rejected")
	}
}
