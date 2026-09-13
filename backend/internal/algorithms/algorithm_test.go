package algorithms

import (
	"math"
	"testing"

	"docker-fan-control/internal/models"
)

func TestLinearCalculate(t *testing.T) {
	l := NewLinear(30, 80, 30, 100)
	cases := []struct {
		temp float64
		want int
	}{
		{20, 30},  // below min → min speed
		{30, 30},  // at min
		{55, 65},  // midpoint: 30 + (25/50)*70 = 65
		{80, 100}, // at max
		{95, 100}, // above max → max speed
	}
	for _, c := range cases {
		if got := l.Calculate(c.temp); got != c.want {
			t.Errorf("Linear.Calculate(%v) = %d, want %d", c.temp, got, c.want)
		}
	}
}

func TestLinearFromParamsDefaults(t *testing.T) {
	l := NewLinearFromParams(models.AlgorithmParams{})
	if l.MinTemp != 30 || l.MaxTemp != 80 || l.MinSpeed != 30 || l.MaxSpeed != 100 {
		t.Errorf("unexpected defaults: %+v", l)
	}
	l = NewLinearFromParams(models.AlgorithmParams{"min_temp": 40.0, "max_temp": 90.0, "min_speed": 20.0, "max_speed": 90.0})
	if l.MinTemp != 40 || l.MaxTemp != 90 || l.MinSpeed != 20 || l.MaxSpeed != 90 {
		t.Errorf("params not applied: %+v", l)
	}
}

func TestStepCalculate(t *testing.T) {
	s := NewStep([]models.StepPoint{
		{Temp: 30, Speed: 30},
		{Temp: 50, Speed: 50},
		{Temp: 70, Speed: 75},
		{Temp: 80, Speed: 100},
	})
	cases := []struct {
		temp float64
		want int
	}{
		{10, 30},  // below first step → first speed
		{30, 30},  // exactly first step
		{49, 30},  // between steps → lower step holds
		{50, 50},  // exactly second step
		{79, 75},  // between → 70-step holds
		{100, 100}, // above last → last speed
	}
	for _, c := range cases {
		if got := s.Calculate(c.temp); got != c.want {
			t.Errorf("Step.Calculate(%v) = %d, want %d", c.temp, got, c.want)
		}
	}
}

func TestStepSortsUnorderedInput(t *testing.T) {
	s := NewStep([]models.StepPoint{
		{Temp: 80, Speed: 100},
		{Temp: 30, Speed: 30},
		{Temp: 50, Speed: 50},
	})
	if got := s.Calculate(55); got != 50 {
		t.Errorf("unsorted steps not handled: Calculate(55) = %d, want 50", got)
	}
}

func TestPIDAtSetpoint(t *testing.T) {
	p := NewPID(70, 2, 0.1, 1, 30, 100)
	// First call at setpoint: error 0, no derivative history → base speed (mid).
	if got := p.Calculate(70); got != 65 {
		t.Errorf("PID at setpoint = %d, want 65 (base)", got)
	}
}

func TestPIDAboveSetpointRaisesSpeed(t *testing.T) {
	p := NewPID(70, 2, 0, 0, 30, 100) // P-only
	got := p.Calculate(80)            // err=10, pTerm=20 → 65+20 = 85
	if got != 85 {
		t.Errorf("PID above setpoint = %d, want 85", got)
	}
}

func TestPIDKiZeroNoBlowup(t *testing.T) {
	// Ki==0 must not divide by zero in the anti-windup clamp.
	p := NewPID(70, 2, 0, 1, 30, 100)
	for i := 0; i < 50; i++ {
		out := p.Calculate(85)
		if math.IsInf(float64(out), 0) || math.IsNaN(float64(out)) {
			t.Fatalf("PID with Ki=0 produced non-finite output: %d", out)
		}
		if out < 30 || out > 100 {
			t.Fatalf("PID output out of bounds: %d", out)
		}
	}
}

func TestPIDIntegralPersistsAcrossCalls(t *testing.T) {
	p := NewPID(70, 1, 0.5, 0, 30, 100) // Kd=0 to isolate the integral
	first := p.Calculate(80)            // integral=10 → 65+10+5 = 80
	second := p.Calculate(80)           // integral=20 → 65+10+10 = 85
	if !(second > first) {
		t.Errorf("integral state did not accumulate: first=%d second=%d", first, second)
	}

	// Reset returns to the fresh-state response.
	p.Reset()
	if got := p.Calculate(80); got != first {
		t.Errorf("after Reset, Calculate(80) = %d, want %d", got, first)
	}
}

func TestPIDAntiWindupClamped(t *testing.T) {
	p := NewPID(70, 1, 10, 0, 30, 100) // maxIntegral = (100-30)/10 = 7
	var out int
	for i := 0; i < 200; i++ {
		out = p.Calculate(90)
	}
	if out != 100 {
		t.Errorf("expected saturated output 100, got %d", out)
	}
	// A single call after long saturation must still be clamped (no runaway).
	if got := p.Calculate(90); got != 100 {
		t.Errorf("post-saturation output = %d, want 100", got)
	}
}

func TestNewAlgorithmFactory(t *testing.T) {
	if a := NewAlgorithm("linear", models.AlgorithmParams{}); a.Type() != "linear" {
		t.Errorf("linear factory returned %s", a.Type())
	}
	if a := NewAlgorithm("step", models.AlgorithmParams{}); a.Type() != "step" {
		t.Errorf("step factory returned %s", a.Type())
	}
	if a := NewAlgorithm("pid", models.AlgorithmParams{}); a.Type() != "pid" {
		t.Errorf("pid factory returned %s", a.Type())
	}
	if a := NewAlgorithm("bogus", models.AlgorithmParams{}); a.Type() != "linear" {
		t.Errorf("unknown algorithm should default to linear, got %s", a.Type())
	}
}

func TestAggregateInputs(t *testing.T) {
	vals := []float64{40, 60, 20}
	if got := AggregateInputs(vals, models.AggregationMax, nil); got != 60 {
		t.Errorf("max = %v, want 60", got)
	}
	if got := AggregateInputs(vals, models.AggregationMin, nil); got != 20 {
		t.Errorf("min = %v, want 20", got)
	}
	if got := AggregateInputs(vals, models.AggregationAvg, nil); got != 40 {
		t.Errorf("avg = %v, want 40", got)
	}
	// Weighted: (40*1 + 60*3 + 20*1) / 5 = 240/5 = 48
	if got := AggregateInputs(vals, models.AggregationWeighted, []float64{1, 3, 1}); got != 48 {
		t.Errorf("weighted = %v, want 48", got)
	}
}

func TestAggregateInputsEdgeCases(t *testing.T) {
	if got := AggregateInputs(nil, models.AggregationMax, nil); got != 0 {
		t.Errorf("empty inputs = %v, want 0", got)
	}
	// Mismatched weight count falls back to average.
	if got := AggregateInputs([]float64{10, 30}, models.AggregationWeighted, []float64{1}); got != 20 {
		t.Errorf("mismatched weights should avg to 20, got %v", got)
	}
	// Zero total weight → 0.
	if got := AggregateInputs([]float64{10, 30}, models.AggregationWeighted, []float64{0, 0}); got != 0 {
		t.Errorf("zero total weight = %v, want 0", got)
	}
	// Unknown method → max.
	if got := AggregateInputs([]float64{5, 9, 2}, "nonsense", nil); got != 9 {
		t.Errorf("unknown method should max to 9, got %v", got)
	}
}

func TestClamp(t *testing.T) {
	if Clamp(150, 0, 100) != 100 {
		t.Error("clamp high failed")
	}
	if Clamp(-5, 0, 100) != 0 {
		t.Error("clamp low failed")
	}
	if Clamp(42, 0, 100) != 42 {
		t.Error("clamp passthrough failed")
	}
}
