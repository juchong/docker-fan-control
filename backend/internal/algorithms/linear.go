package algorithms

import (
	"docker-fan-control/internal/models"
)

// Linear implements a linear fan curve
type Linear struct {
	MinTemp  float64
	MaxTemp  float64
	MinSpeed int
	MaxSpeed int
}

// NewLinear creates a new linear algorithm
func NewLinear(minTemp, maxTemp float64, minSpeed, maxSpeed int) *Linear {
	return &Linear{
		MinTemp:  minTemp,
		MaxTemp:  maxTemp,
		MinSpeed: minSpeed,
		MaxSpeed: maxSpeed,
	}
}

// NewLinearFromParams creates a linear algorithm from AlgorithmParams
func NewLinearFromParams(params models.AlgorithmParams) *Linear {
	l := &Linear{
		MinTemp:  30,
		MaxTemp:  80,
		MinSpeed: 30,
		MaxSpeed: 100,
	}

	if v, ok := params["min_temp"].(float64); ok {
		l.MinTemp = v
	}
	if v, ok := params["max_temp"].(float64); ok {
		l.MaxTemp = v
	}
	if v, ok := params["min_speed"].(float64); ok {
		l.MinSpeed = int(v)
	}
	if v, ok := params["max_speed"].(float64); ok {
		l.MaxSpeed = int(v)
	}

	return l
}

// Calculate returns fan speed for given temperature
// Formula: speed = minSpeed + (temp - minTemp) / (maxTemp - minTemp) * (maxSpeed - minSpeed)
func (l *Linear) Calculate(temp float64) int {
	if temp <= l.MinTemp {
		return l.MinSpeed
	}
	if temp >= l.MaxTemp {
		return l.MaxSpeed
	}

	// Linear interpolation
	tempRange := l.MaxTemp - l.MinTemp
	speedRange := float64(l.MaxSpeed - l.MinSpeed)

	speed := float64(l.MinSpeed) + (temp-l.MinTemp)/tempRange*speedRange

	return Clamp(int(speed), l.MinSpeed, l.MaxSpeed)
}

// Type returns the algorithm type
func (l *Linear) Type() string {
	return "linear"
}

// Params returns the algorithm parameters
func (l *Linear) Params() map[string]any {
	return map[string]any{
		"min_temp":  l.MinTemp,
		"max_temp":  l.MaxTemp,
		"min_speed": l.MinSpeed,
		"max_speed": l.MaxSpeed,
	}
}
