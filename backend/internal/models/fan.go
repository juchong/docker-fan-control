package models

import (
	"time"
)

// Fan represents a detected/configured fan
type Fan struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	IPMISensorID string    `json:"ipmi_sensor_id" gorm:"column:ipmi_sensor_id;not null"` // e.g., "Fan1 RPM"
	IPMIZone     *int      `json:"ipmi_zone,omitempty" gorm:"column:ipmi_zone"`          // PWM zone (0-7 typically)
	Label        string    `json:"label,omitempty"`                                      // User-defined label
	DetectedName string    `json:"detected_name,omitempty" gorm:"column:detected_name"`  // Auto-detected name
	MinRPM       *int      `json:"min_rpm,omitempty" gorm:"column:min_rpm"`              // Observed minimum RPM
	MaxRPM       *int      `json:"max_rpm,omitempty" gorm:"column:max_rpm"`              // Observed maximum RPM
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// DisplayName returns the label if set, otherwise the detected name
func (f *Fan) DisplayName() string {
	if f.Label != "" {
		return f.Label
	}
	if f.DetectedName != "" {
		return f.DetectedName
	}
	return f.IPMISensorID
}

// FanStatus represents current fan readings
type FanStatus struct {
	ID               uint   `json:"id"`
	IPMISensorID     string `json:"ipmi_sensor_id"`
	Label            string `json:"label"`
	CurrentRPM       int    `json:"current_rpm"`
	CurrentDuty      int    `json:"current_duty"`              // Actual duty cycle percentage (0-100)
	CurrentPercent   int    `json:"current_percent,omitempty"` // If available
	TargetPercent    *int   `json:"target_percent,omitempty"`  // Current target from profile
	ManualOverride   bool   `json:"manual_override"`
	IPMIZone         *int   `json:"ipmi_zone,omitempty"`       // Use ipmi_zone for consistency with Fan model
	AssignedProfiles []uint `json:"assigned_profiles,omitempty"`
}

// DetectedFan represents a fan found during IPMI scan
type DetectedFan struct {
	SensorID string `json:"sensor_id"`
	Name     string `json:"name"`
	RPM      int    `json:"rpm"`
	Status   string `json:"status"` // "ok", "warning", "critical"
	Unit     string `json:"unit"`   // "RPM"
}

// UpdateFanRequest is the API request for updating a fan
type UpdateFanRequest struct {
	Label    *string `json:"label,omitempty"`
	IPMIZone *int    `json:"ipmi_zone,omitempty"`
}

// SetFanSpeedRequest is the API request for manually setting fan speed
type SetFanSpeedRequest struct {
	Percent int `json:"percent"` // 0-100
}

// IdentifyFanRequest is the API request for identifying a fan
type IdentifyFanRequest struct {
	Duration int `json:"duration,omitempty"` // Seconds to run at 100% (default: 5)
}
