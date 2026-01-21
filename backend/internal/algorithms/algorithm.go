package algorithms

import (
	"docker-fan-control/internal/models"
)

// Algorithm defines the interface for fan control algorithms
type Algorithm interface {
	// Calculate returns target fan speed (0-100%) given input temperature
	Calculate(temp float64) int

	// Type returns the algorithm type identifier
	Type() string

	// Params returns the algorithm parameters
	Params() map[string]any
}

// NewAlgorithm creates an algorithm from profile data
func NewAlgorithm(algorithmType string, params models.AlgorithmParams) Algorithm {
	switch algorithmType {
	case "linear":
		return NewLinearFromParams(params)
	case "step":
		return NewStepFromParams(params)
	case "pid":
		return NewPIDFromParams(params)
	default:
		// Default to linear with sensible defaults
		return NewLinear(30, 80, 30, 100)
	}
}

// AggregateInputs aggregates multiple input values based on aggregation method
func AggregateInputs(values []float64, method string, weights []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	switch method {
	case models.AggregationMax:
		max := values[0]
		for _, v := range values[1:] {
			if v > max {
				max = v
			}
		}
		return max

	case models.AggregationMin:
		min := values[0]
		for _, v := range values[1:] {
			if v < min {
				min = v
			}
		}
		return min

	case models.AggregationAvg:
		sum := 0.0
		for _, v := range values {
			sum += v
		}
		return sum / float64(len(values))

	case models.AggregationWeighted:
		if len(weights) != len(values) {
			// Fall back to average if weights don't match
			return AggregateInputs(values, models.AggregationAvg, nil)
		}
		weightedSum := 0.0
		totalWeight := 0.0
		for i, v := range values {
			weightedSum += v * weights[i]
			totalWeight += weights[i]
		}
		if totalWeight == 0 {
			return 0
		}
		return weightedSum / totalWeight

	default:
		return AggregateInputs(values, models.AggregationMax, nil)
	}
}

// Clamp ensures a value is within min and max bounds
func Clamp(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
