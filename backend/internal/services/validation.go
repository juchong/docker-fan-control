package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"docker-fan-control/internal/database"
	"docker-fan-control/internal/models"
)

// ValidationError represents a validation error with details
type ValidationError struct {
	Field   string
	Message string
	Details interface{}
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error on %s: %s", e.Field, e.Message)
}

// ProfileValidator validates profile configurations
type ProfileValidator struct {
	driverRegistry *DriverRegistry
}

// NewProfileValidator creates a new profile validator
func NewProfileValidator(driverRegistry *DriverRegistry) *ProfileValidator {
	return &ProfileValidator{driverRegistry: driverRegistry}
}

// ValidateProfile validates a profile and its parameters
func (v *ProfileValidator) ValidateProfile(profile *models.Profile) error {
	var errs []*ValidationError

	// Basic field validation
	if profile.Name == "" {
		errs = append(errs, &ValidationError{
			Field:   "name",
			Message: "name is required",
		})
	}

	if profile.Algorithm == "" {
		errs = append(errs, &ValidationError{
			Field:   "algorithm",
			Message: "algorithm is required",
		})
	} else if profile.Algorithm != "linear" && profile.Algorithm != "step" && profile.Algorithm != "pid" {
		errs = append(errs, &ValidationError{
			Field:   "algorithm",
			Message: "algorithm must be one of: linear, step, pid",
		})
	}

	// Validate algorithm parameters
	if err := v.validateAlgorithmParams(profile.Algorithm, profile.AlgorithmParams); err != nil {
		errs = append(errs, err.(*ValidationError))
	}

	// Validate zones
	if err := v.validateZones(profile.Zones); err != nil {
		errs = append(errs, err.(*ValidationError))
	}

	// Validate inputs
	if err := v.validateProfileInputs(profile.Inputs); err != nil {
		errs = append(errs, err.(*ValidationError))
	}

	// Validate priority
	if profile.Priority < 0 {
		errs = append(errs, &ValidationError{
			Field:   "priority",
			Message: "priority must be >= 0",
		})
	}

	// Validate transition time
	if profile.TransitionTime < 0 || profile.TransitionTime > 300 {
		errs = append(errs, &ValidationError{
			Field:   "transition_time",
			Message: "transition time must be between 0 and 300 seconds",
		})
	}

	// Validate min run time
	if profile.MinRunTime < 0 || profile.MinRunTime > 300 {
		errs = append(errs, &ValidationError{
			Field:   "min_run_time",
			Message: "min run time must be between 0 and 300 seconds",
		})
	}

	// Validate hysteresis
	if profile.Hysteresis < 0 || profile.Hysteresis > 10 {
		errs = append(errs, &ValidationError{
			Field:   "hysteresis",
			Message: "hysteresis must be between 0 and 10",
		})
	}

	if len(errs) > 0 {
		return &ValidationError{
			Field:   "profile",
			Message: "profile validation failed",
			Details: errs,
		}
	}

	return nil
}

// validateAlgorithmParams validates algorithm-specific parameters
func (v *ProfileValidator) validateAlgorithmParams(algorithm string, params models.AlgorithmParams) error {
	switch algorithm {
	case "linear":
		return v.validateLinearParams(params)
	case "step":
		return v.validateStepParams(params)
	case "pid":
		return v.validatePIDParams(params)
	}
	return nil
}

// validateLinearParams validates linear algorithm parameters
func (v *ProfileValidator) validateLinearParams(params models.AlgorithmParams) error {
	var errs []*ValidationError

	minTemp, ok := params["min_temp"].(float64)
	if !ok {
		errs = append(errs, &ValidationError{
			Field:   "algorithm_params.min_temp",
			Message: "min_temp is required",
		})
	} else if minTemp < 0 || minTemp > 150 {
		errs = append(errs, &ValidationError{
			Field:   "algorithm_params.min_temp",
			Message: "min_temp must be between 0 and 150",
		})
	}

	maxTemp, ok := params["max_temp"].(float64)
	if !ok {
		errs = append(errs, &ValidationError{
			Field:   "algorithm_params.max_temp",
			Message: "max_temp is required",
		})
	} else if maxTemp < 0 || maxTemp > 150 {
		errs = append(errs, &ValidationError{
			Field:   "algorithm_params.max_temp",
			Message: "max_temp must be between 0 and 150",
		})
	}

	if ok && minTemp != 0 && maxTemp != 0 && minTemp >= maxTemp {
		errs = append(errs, &ValidationError{
			Field:   "algorithm_params",
			Message: "min_temp must be less than max_temp",
		})
	}

	minSpeed, ok := params["min_speed"].(float64)
	if !ok {
		errs = append(errs, &ValidationError{
			Field:   "algorithm_params.min_speed",
			Message: "min_speed is required",
		})
	} else if minSpeed < 0 || minSpeed > 100 {
		errs = append(errs, &ValidationError{
			Field:   "algorithm_params.min_speed",
			Message: "min_speed must be between 0 and 100",
		})
	}

	maxSpeed, ok := params["max_speed"].(float64)
	if !ok {
		errs = append(errs, &ValidationError{
			Field:   "algorithm_params.max_speed",
			Message: "max_speed is required",
		})
	} else if maxSpeed < 0 || maxSpeed > 100 {
		errs = append(errs, &ValidationError{
			Field:   "algorithm_params.max_speed",
			Message: "max_speed must be between 0 and 100",
		})
	}

	if ok && minSpeed != 0 && maxSpeed != 0 && minSpeed >= maxSpeed {
		errs = append(errs, &ValidationError{
			Field:   "algorithm_params",
			Message: "min_speed must be less than max_speed",
		})
	}

	if len(errs) > 0 {
		return &ValidationError{
			Field:   "algorithm_params",
			Message: "linear algorithm parameter validation failed",
			Details: errs,
		}
	}

	return nil
}

// validateStepParams validates step algorithm parameters
func (v *ProfileValidator) validateStepParams(params models.AlgorithmParams) error {
	var errs []*ValidationError

	stepsRaw, ok := params["steps"].([]any)
	if !ok {
		errs = append(errs, &ValidationError{
			Field:   "algorithm_params.steps",
			Message: "steps is required",
		})
	} else if len(stepsRaw) == 0 {
		errs = append(errs, &ValidationError{
			Field:   "algorithm_params.steps",
			Message: "at least one step is required",
		})
	} else {
		// Validate each step
		for i, stepRaw := range stepsRaw {
			stepMap, ok := stepRaw.(map[string]any)
			if !ok {
				errs = append(errs, &ValidationError{
					Field:   fmt.Sprintf("algorithm_params.steps[%d]", i),
					Message: "step must be an object",
				})
				continue
			}

			if temp, ok := stepMap["temp"].(float64); !ok {
				errs = append(errs, &ValidationError{
					Field:   fmt.Sprintf("algorithm_params.steps[%d].temp", i),
					Message: "temp is required",
				})
			} else if temp < 0 || temp > 150 {
				errs = append(errs, &ValidationError{
					Field:   fmt.Sprintf("algorithm_params.steps[%d].temp", i),
					Message: "temp must be between 0 and 150",
				})
			}

			if speed, ok := stepMap["speed"].(float64); !ok {
				errs = append(errs, &ValidationError{
					Field:   fmt.Sprintf("algorithm_params.steps[%d].speed", i),
					Message: "speed is required",
				})
			} else if speed < 0 || speed > 100 {
				errs = append(errs, &ValidationError{
					Field:   fmt.Sprintf("algorithm_params.steps[%d].speed", i),
					Message: "speed must be between 0 and 100",
				})
			}
		}
	}

	if len(errs) > 0 {
		return &ValidationError{
			Field:   "algorithm_params",
			Message: "step algorithm parameter validation failed",
			Details: errs,
		}
	}

	return nil
}

// validatePIDParams validates PID algorithm parameters
func (v *ProfileValidator) validatePIDParams(params models.AlgorithmParams) error {
	var errs []*ValidationError

	setpoint, ok := params["setpoint"].(float64)
	if !ok {
		errs = append(errs, &ValidationError{
			Field:   "algorithm_params.setpoint",
			Message: "setpoint is required",
		})
	} else if setpoint < 0 || setpoint > 150 {
		errs = append(errs, &ValidationError{
			Field:   "algorithm_params.setpoint",
			Message: "setpoint must be between 0 and 150",
		})
	}

	kp, ok := params["kp"].(float64)
	if !ok {
		errs = append(errs, &ValidationError{
			Field:   "algorithm_params.kp",
			Message: "kp is required",
		})
	} else if kp < 0 {
		errs = append(errs, &ValidationError{
			Field:   "algorithm_params.kp",
			Message: "kp must be >= 0",
		})
	}

	ki, ok := params["ki"].(float64)
	if !ok {
		errs = append(errs, &ValidationError{
			Field:   "algorithm_params.ki",
			Message: "ki is required",
		})
	} else if ki < 0 {
		errs = append(errs, &ValidationError{
			Field:   "algorithm_params.ki",
			Message: "ki must be >= 0",
		})
	}

	kd, ok := params["kd"].(float64)
	if !ok {
		errs = append(errs, &ValidationError{
			Field:   "algorithm_params.kd",
			Message: "kd is required",
		})
	} else if kd < 0 {
		errs = append(errs, &ValidationError{
			Field:   "algorithm_params.kd",
			Message: "kd must be >= 0",
		})
	}

	minSpeed, ok := params["min_speed"].(float64)
	if !ok {
		errs = append(errs, &ValidationError{
			Field:   "algorithm_params.min_speed",
			Message: "min_speed is required",
		})
	} else if minSpeed < 0 || minSpeed > 100 {
		errs = append(errs, &ValidationError{
			Field:   "algorithm_params.min_speed",
			Message: "min_speed must be between 0 and 100",
		})
	}

	maxSpeed, ok := params["max_speed"].(float64)
	if !ok {
		errs = append(errs, &ValidationError{
			Field:   "algorithm_params.max_speed",
			Message: "max_speed is required",
		})
	} else if maxSpeed < 0 || maxSpeed > 100 {
		errs = append(errs, &ValidationError{
			Field:   "algorithm_params.max_speed",
			Message: "max_speed must be between 0 and 100",
		})
	}

	if ok && minSpeed != 0 && maxSpeed != 0 && minSpeed >= maxSpeed {
		errs = append(errs, &ValidationError{
			Field:   "algorithm_params",
			Message: "min_speed must be less than max_speed",
		})
	}

	// Check PID stability (basic check)
	if ok && kp != 0 && ki != 0 && kd != 0 {
		// Simple heuristic: Ki should be much smaller than Kp
		if ki > kp {
			errs = append(errs, &ValidationError{
				Field:   "algorithm_params",
				Message: "ki should typically be smaller than kp for stability",
			})
		}
	}

	if len(errs) > 0 {
		return &ValidationError{
			Field:   "algorithm_params",
			Message: "PID algorithm parameter validation failed",
			Details: errs,
		}
	}

	return nil
}

// validateZones validates zone assignments
func (v *ProfileValidator) validateZones(zones []int) error {
	if len(zones) == 0 {
		// No zones specified is valid (controls all fans)
		return nil
	}

	// Use the driver already in use rather than re-running detection (which would
	// re-issue probe commands) on every profile save.
	driver, err := v.driverRegistry.GetActiveDriver(context.Background())
	if err != nil || driver == nil {
		return &ValidationError{
			Field:   "zones",
			Message: "cannot validate zones without active driver",
		}
	}

	zoneLayout := driver.GetZoneLayout()

	// Check if custom zone layout is configured
	if customLayout, err := v.getCustomZoneLayout(); err == nil && customLayout != nil {
		zoneLayout = *customLayout
	}

	// Zone IDs are driver-defined and need not form a 0..MaxZones-1 range (the
	// hwmon driver uses the PWM channel number), so validate purely by
	// membership in the layout rather than a numeric range.
	validZones := make(map[int]bool)
	for _, z := range zoneLayout.Zones {
		validZones[z.ID] = true
	}

	for _, zone := range zones {
		if !validZones[zone] {
			return &ValidationError{
				Field:   "zones",
				Message: fmt.Sprintf("zone %d does not exist in the driver's zone layout", zone),
			}
		}
	}

	return nil
}

// getCustomZoneLayout retrieves custom zone layout from settings
func (v *ProfileValidator) getCustomZoneLayout() (*models.ZoneLayout, error) {
	val, err := database.GetSetting(models.SettingZoneLayout)
	if err != nil {
		return nil, err
	}

	if val == nil {
		return nil, nil
	}

	// Handle different data types
	var zoneLayout models.ZoneLayout
	switch v := val.(type) {
	case string:
		if err := json.Unmarshal([]byte(v), &zoneLayout); err != nil {
			return nil, err
		}
	case map[string]interface{}:
		jsonData, _ := json.Marshal(v)
		if err := json.Unmarshal(jsonData, &zoneLayout); err != nil {
			return nil, err
		}
	case []byte:
		if err := json.Unmarshal(v, &zoneLayout); err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("unsupported zone layout data type")
	}

	return &zoneLayout, nil
}

// validateProfileInputs validates profile input configurations
func (v *ProfileValidator) validateProfileInputs(inputs []models.ProfileInput) error {
	if len(inputs) == 0 {
		// No inputs specified is valid (will use default)
		return nil
	}

	var errs []*ValidationError

	// Check for duplicate inputs
	seen := make(map[string]bool)
	for i, input := range inputs {
		key := fmt.Sprintf("%s-%d", input.InputType, input.InputIndex)
		if seen[key] {
			errs = append(errs, &ValidationError{
				Field:   fmt.Sprintf("inputs[%d]", i),
				Message: "duplicate input",
			})
		}
		seen[key] = true

		// Validate input type
		validTypes := map[string]bool{
			models.InputTypeGPUTemp:   true,
			models.InputTypeGPULoad:   true,
			models.InputTypeCPUTemp:   true,
			models.InputTypeCPULoad:   true,
			models.InputTypeDriveTemp: true,
			models.InputTypeBoardTemp: true,
		}

		if !validTypes[input.InputType] {
			errs = append(errs, &ValidationError{
				Field:   fmt.Sprintf("inputs[%d].input_type", i),
				Message: fmt.Sprintf("invalid input type: %s", input.InputType),
			})
		}

		// Validate weight
		if input.Weight <= 0 {
			errs = append(errs, &ValidationError{
				Field:   fmt.Sprintf("inputs[%d].weight", i),
				Message: "weight must be > 0",
			})
		}
	}

	if len(errs) > 0 {
		return &ValidationError{
			Field:   "inputs",
			Message: "profile input validation failed",
			Details: errs,
		}
	}

	return nil
}

// ValidateProfileRequest validates a profile creation request
func (v *ProfileValidator) ValidateProfileRequest(req *models.CreateProfileRequest) error {
	// Convert request to profile for validation (with defaults for the optional
	// tuning fields, overridden when the request supplies them).
	profile := &models.Profile{
		Name:             req.Name,
		Description:      req.Description,
		Algorithm:       req.Algorithm,
		AlgorithmParams: req.AlgorithmParams,
		Priority:        req.Priority,
		Zones:           req.Zones,
		Inputs:          req.Inputs,
		SmoothTransition: true,
		TransitionTime:   10,
		MinRunTime:       30,
		Hysteresis:       2.0,
	}
	if req.SmoothTransition != nil {
		profile.SmoothTransition = *req.SmoothTransition
	}
	if req.TransitionTime != nil {
		profile.TransitionTime = *req.TransitionTime
	}
	if req.MinRunTime != nil {
		profile.MinRunTime = *req.MinRunTime
	}
	if req.Hysteresis != nil {
		profile.Hysteresis = *req.Hysteresis
	}

	return v.ValidateProfile(profile)
}

// ValidateProfileUpdate validates a profile update request
func (v *ProfileValidator) ValidateProfileUpdate(req *models.UpdateProfileRequest) error {
	// Create a minimal profile for validation
	profile := &models.Profile{
		Algorithm:       "", // Will be set if provided
		AlgorithmParams: make(models.AlgorithmParams),
		Priority:        0,
		Zones:           []int{},
		Inputs:          []models.ProfileInput{},
	}

	if req.Name != nil {
		profile.Name = *req.Name
	}
	if req.Algorithm != nil {
		profile.Algorithm = *req.Algorithm
	}
	if req.AlgorithmParams != nil {
		profile.AlgorithmParams = *req.AlgorithmParams
	}
	if req.Priority != nil {
		profile.Priority = *req.Priority
	}
	if req.Zones != nil {
		profile.Zones = *req.Zones
	}
	if req.Inputs != nil {
		profile.Inputs = *req.Inputs
	}
	if req.TransitionTime != nil {
		profile.TransitionTime = *req.TransitionTime
	}
	if req.MinRunTime != nil {
		profile.MinRunTime = *req.MinRunTime
	}
	if req.Hysteresis != nil {
		profile.Hysteresis = *req.Hysteresis
	}

	return v.ValidateProfile(profile)
}

// IsValidationError checks if an error is a ValidationError
func IsValidationError(err error) bool {
	_, ok := err.(*ValidationError)
	return ok
}

// GetValidationErrors extracts ValidationError details from nested errors
func GetValidationErrors(err error) []*ValidationError {
	var errors []*ValidationError

	if verr, ok := err.(*ValidationError); ok {
		if details, ok := verr.Details.([]*ValidationError); ok {
			errors = append(errors, details...)
		} else {
			errors = append(errors, verr)
		}
	}

	return errors
}
