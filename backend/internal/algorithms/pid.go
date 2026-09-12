package algorithms

import (
	"docker-fan-control/internal/models"
)

// PID implements a PID controller for fan speed
type PID struct {
	Setpoint float64 // Target temperature
	Kp       float64 // Proportional gain
	Ki       float64 // Integral gain
	Kd       float64 // Derivative gain
	MinSpeed int
	MaxSpeed int

	// State
	integral float64
	lastErr  float64
	lastTemp float64
}

// NewPID creates a new PID algorithm
func NewPID(setpoint, kp, ki, kd float64, minSpeed, maxSpeed int) *PID {
	return &PID{
		Setpoint: setpoint,
		Kp:       kp,
		Ki:       ki,
		Kd:       kd,
		MinSpeed: minSpeed,
		MaxSpeed: maxSpeed,
	}
}

// NewPIDFromParams creates a PID algorithm from AlgorithmParams
func NewPIDFromParams(params models.AlgorithmParams) *PID {
	p := &PID{
		Setpoint: 70,
		Kp:       2.0,
		Ki:       0.1,
		Kd:       1.0,
		MinSpeed: 30,
		MaxSpeed: 100,
	}

	if v, ok := params["setpoint"].(float64); ok {
		p.Setpoint = v
	}
	if v, ok := params["kp"].(float64); ok {
		p.Kp = v
	}
	if v, ok := params["ki"].(float64); ok {
		p.Ki = v
	}
	if v, ok := params["kd"].(float64); ok {
		p.Kd = v
	}
	if v, ok := params["min_speed"].(float64); ok {
		p.MinSpeed = int(v)
	}
	if v, ok := params["max_speed"].(float64); ok {
		p.MaxSpeed = int(v)
	}

	return p
}

// Calculate returns fan speed for given temperature using PID control
// The PID tries to maintain temperature at setpoint
// When temp > setpoint, error is positive, increasing fan speed
// When temp < setpoint, error is negative, decreasing fan speed
func (p *PID) Calculate(temp float64) int {
	// Calculate error (positive when temp is above setpoint)
	err := temp - p.Setpoint

	// Proportional term
	pTerm := p.Kp * err

	// Integral term (with anti-windup). Skip entirely when Ki==0 to avoid a
	// divide-by-zero (+Inf) in the windup clamp and needless accumulation.
	iTerm := 0.0
	if p.Ki != 0 {
		p.integral += err
		maxIntegral := float64(p.MaxSpeed-p.MinSpeed) / p.Ki
		if p.integral > maxIntegral {
			p.integral = maxIntegral
		} else if p.integral < -maxIntegral {
			p.integral = -maxIntegral
		}
		iTerm = p.Ki * p.integral
	}

	// Derivative term (rate of change of temperature)
	dTerm := 0.0
	if p.lastTemp != 0 {
		dTerm = p.Kd * (temp - p.lastTemp)
	}
	p.lastTemp = temp
	p.lastErr = err

	// Calculate output
	// Base speed at setpoint + PID adjustment
	baseSpeed := float64(p.MinSpeed+p.MaxSpeed) / 2
	output := baseSpeed + pTerm + iTerm + dTerm

	return Clamp(int(output), p.MinSpeed, p.MaxSpeed)
}

// Type returns the algorithm type
func (p *PID) Type() string {
	return "pid"
}

// Params returns the algorithm parameters
func (p *PID) Params() map[string]any {
	return map[string]any{
		"setpoint":  p.Setpoint,
		"kp":        p.Kp,
		"ki":        p.Ki,
		"kd":        p.Kd,
		"min_speed": p.MinSpeed,
		"max_speed": p.MaxSpeed,
	}
}

// Reset resets the PID controller state
func (p *PID) Reset() {
	p.integral = 0
	p.lastErr = 0
	p.lastTemp = 0
}

// SetSetpoint changes the target temperature
func (p *PID) SetSetpoint(setpoint float64) {
	p.Setpoint = setpoint
}

// SetGains updates the PID gains
func (p *PID) SetGains(kp, ki, kd float64) {
	p.Kp = kp
	p.Ki = ki
	p.Kd = kd
}
