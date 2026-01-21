package algorithms

import (
	"sort"

	"docker-fan-control/internal/models"
)

// Step implements a step function fan curve
type Step struct {
	Steps []models.StepPoint
}

// NewStep creates a new step algorithm
func NewStep(steps []models.StepPoint) *Step {
	s := &Step{Steps: steps}
	s.sortSteps()
	return s
}

// NewStepFromParams creates a step algorithm from AlgorithmParams
func NewStepFromParams(params models.AlgorithmParams) *Step {
	s := &Step{
		Steps: []models.StepPoint{
			{Temp: 30, Speed: 30},
			{Temp: 50, Speed: 50},
			{Temp: 70, Speed: 75},
			{Temp: 80, Speed: 100},
		},
	}

	if stepsRaw, ok := params["steps"].([]any); ok {
		s.Steps = make([]models.StepPoint, 0, len(stepsRaw))
		for _, stepRaw := range stepsRaw {
			if stepMap, ok := stepRaw.(map[string]any); ok {
				point := models.StepPoint{}
				if temp, ok := stepMap["temp"].(float64); ok {
					point.Temp = temp
				}
				if speed, ok := stepMap["speed"].(float64); ok {
					point.Speed = int(speed)
				}
				s.Steps = append(s.Steps, point)
			}
		}
	}

	s.sortSteps()
	return s
}

// sortSteps sorts steps by temperature ascending
func (s *Step) sortSteps() {
	sort.Slice(s.Steps, func(i, j int) bool {
		return s.Steps[i].Temp < s.Steps[j].Temp
	})
}

// Calculate returns fan speed for given temperature
// Returns the speed of the highest step that the temperature exceeds
func (s *Step) Calculate(temp float64) int {
	if len(s.Steps) == 0 {
		return 50 // Default
	}

	// Find highest step where temp >= threshold
	speed := s.Steps[0].Speed
	for _, step := range s.Steps {
		if temp >= step.Temp {
			speed = step.Speed
		} else {
			break
		}
	}

	return Clamp(speed, 0, 100)
}

// Type returns the algorithm type
func (s *Step) Type() string {
	return "step"
}

// Params returns the algorithm parameters
func (s *Step) Params() map[string]any {
	steps := make([]map[string]any, len(s.Steps))
	for i, step := range s.Steps {
		steps[i] = map[string]any{
			"temp":  step.Temp,
			"speed": step.Speed,
		}
	}
	return map[string]any{
		"steps": steps,
	}
}

// AddStep adds a new step point
func (s *Step) AddStep(temp float64, speed int) {
	s.Steps = append(s.Steps, models.StepPoint{Temp: temp, Speed: speed})
	s.sortSteps()
}

// RemoveStep removes a step at index
func (s *Step) RemoveStep(index int) {
	if index >= 0 && index < len(s.Steps) {
		s.Steps = append(s.Steps[:index], s.Steps[index+1:]...)
	}
}
