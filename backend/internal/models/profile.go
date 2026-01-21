package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

// Profile represents a fan control profile
type Profile struct {
	ID              uint            `json:"id" gorm:"primaryKey"`
	Name            string          `json:"name" gorm:"not null"`
	Description     string          `json:"description,omitempty"`
	Algorithm       string          `json:"algorithm" gorm:"not null"` // "linear", "step", "pid"
	AlgorithmParams AlgorithmParams `json:"algorithm_params" gorm:"type:json;not null"`
	IsActive        bool            `json:"is_active" gorm:"default:false"`
	Priority        int             `json:"priority" gorm:"default:0"` // Higher = takes precedence
	Zones           IntSlice        `json:"zones" gorm:"type:json"`    // Fan zones this profile controls
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`

	// Relations (deprecated, use Zones instead)
	Fans   []Fan          `json:"fans,omitempty" gorm:"many2many:profile_fans;"`
	Inputs []ProfileInput `json:"inputs,omitempty" gorm:"foreignKey:ProfileID"`
}

// IntSlice is a slice of ints that can be stored as JSON
type IntSlice []int

// Scan implements sql.Scanner for IntSlice
func (s *IntSlice) Scan(value any) error {
	if value == nil {
		*s = nil
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return errors.New("type assertion to []byte or string failed")
	}
	return json.Unmarshal(bytes, s)
}

// Value implements driver.Valuer for IntSlice
func (s IntSlice) Value() (driver.Value, error) {
	if s == nil {
		return "[]", nil
	}
	return json.Marshal(s)
}

// AlgorithmParams stores algorithm-specific configuration as JSON
type AlgorithmParams map[string]any

// Scan implements sql.Scanner for AlgorithmParams
func (a *AlgorithmParams) Scan(value any) error {
	if value == nil {
		*a = make(AlgorithmParams)
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return errors.New("type assertion to []byte or string failed")
	}
	return json.Unmarshal(bytes, a)
}

// Value implements driver.Valuer for AlgorithmParams
func (a AlgorithmParams) Value() (driver.Value, error) {
	if a == nil {
		return "{}}", nil
	}
	return json.Marshal(a)
}

// LinearParams are parameters for linear algorithm
type LinearParams struct {
	MinTemp  float64 `json:"min_temp"`  // Temperature at which min_speed is used
	MaxTemp  float64 `json:"max_temp"`  // Temperature at which max_speed is used
	MinSpeed int     `json:"min_speed"` // Minimum fan speed (0-100)
	MaxSpeed int     `json:"max_speed"` // Maximum fan speed (0-100)
}

// StepParams are parameters for step algorithm
type StepParams struct {
	Steps []StepPoint `json:"steps"` // Sorted by temp ascending
}

// StepPoint is a single point in step function
type StepPoint struct {
	Temp  float64 `json:"temp"`  // Temperature threshold
	Speed int     `json:"speed"` // Fan speed when temp >= this threshold
}

// PIDParams are parameters for PID algorithm
type PIDParams struct {
	Setpoint float64 `json:"setpoint"` // Target temperature
	Kp       float64 `json:"kp"`       // Proportional gain
	Ki       float64 `json:"ki"`       // Integral gain
	Kd       float64 `json:"kd"`       // Derivative gain
	MinSpeed int     `json:"min_speed"`
	MaxSpeed int     `json:"max_speed"`
}

// ProfileInput represents an input source for a profile
type ProfileInput struct {
	ID         uint    `json:"id" gorm:"primaryKey"`
	ProfileID  uint    `json:"profile_id" gorm:"not null"`
	InputType  string  `json:"input_type" gorm:"not null"` // "gpu_temp", "gpu_load", "cpu_temp", "cpu_load"
	InputIndex int     `json:"input_index" gorm:"default:0"`
	Weight     float64 `json:"weight" gorm:"default:1.0"`
}

// Valid input types
const (
	InputTypeGPUTemp   = "gpu_temp"
	InputTypeGPULoad   = "gpu_load"
	InputTypeCPUTemp   = "cpu_temp"
	InputTypeCPULoad   = "cpu_load"
	InputTypeMaxTemp   = "max_temp"   // Maximum of all GPU temps
	InputTypeAvgTemp   = "avg_temp"   // Average of all GPU temps
	InputTypeDriveTemp = "drive_temp" // Individual drive temperature by index
	InputTypeMaxDrive  = "max_drive"  // Maximum of all drive temps
	InputTypeMaxCPU    = "max_cpu"    // Maximum of all CPU package temps
)

// Aggregation methods
const (
	AggregationMax      = "max"
	AggregationMin      = "min"
	AggregationAvg      = "avg"
	AggregationWeighted = "weighted"
)

// CreateProfileRequest is the API request for creating a profile
type CreateProfileRequest struct {
	Name            string          `json:"name"`
	Description     string          `json:"description,omitempty"`
	Algorithm       string          `json:"algorithm"`
	AlgorithmParams AlgorithmParams `json:"algorithm_params"`
	Priority        int             `json:"priority,omitempty"`
	Zones           []int           `json:"zones,omitempty"`
	FanIDs          []uint          `json:"fan_ids,omitempty"` // Deprecated, use Zones
	Inputs          []ProfileInput  `json:"inputs,omitempty"`
}

// UpdateProfileRequest is the API request for updating a profile
type UpdateProfileRequest struct {
	Name            *string          `json:"name,omitempty"`
	Description     *string          `json:"description,omitempty"`
	Algorithm       *string          `json:"algorithm,omitempty"`
	AlgorithmParams *AlgorithmParams `json:"algorithm_params,omitempty"`
	Priority        *int             `json:"priority,omitempty"`
	Zones           *[]int           `json:"zones,omitempty"`
	FanIDs          *[]uint          `json:"fan_ids,omitempty"` // Deprecated, use Zones
	Inputs          *[]ProfileInput  `json:"inputs,omitempty"`
}

// ProfileSummary is a lightweight profile representation
type ProfileSummary struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Algorithm   string `json:"algorithm"`
	IsActive    bool   `json:"is_active"`
	Zones       []int  `json:"zones"`
	ZoneCount   int    `json:"zone_count"`
	FanCount    int    `json:"fan_count"` // Deprecated
	InputCount  int    `json:"input_count"`
}
