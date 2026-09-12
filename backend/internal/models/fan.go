package models

import (
	"time"
)

// Fan represents a detected/configured fan
type Fan struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	IPMISensorID string    `json:"ipmi_sensor_id" gorm:"column:ipmi_sensor_id;not null"` // e.g., "Fan1 RPM"
	IPMIZone     *int      `json:"ipmi_zone,omitempty" gorm:"column:ipmi_zone"`          // driver control zone (SetFanSpeed arg / GetZoneForFan)
	Channel      *int      `json:"channel,omitempty" gorm:"column:channel"`             // hardware PWM channel (telemetry/display only)
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
	IPMIZone         *int   `json:"ipmi_zone,omitempty"`       // driver control zone
	Channel          *int   `json:"channel,omitempty"`         // hardware PWM channel (display only)
	AssignedProfiles []uint `json:"assigned_profiles,omitempty"`
}

// DetectedFan represents a fan found during a driver scan. The driver is the
// single source of truth for the three identifier spaces: SensorID (the DB/
// telemetry join key), Channel (the hardware PWM channel), and ZoneID (the
// control target passed to SetFanSpeed / returned by GetZoneForFan).
type DetectedFan struct {
	SensorID  string `json:"sensor_id"`
	Name      string `json:"name"`
	RPM       int    `json:"rpm"`
	DutyCycle int    `json:"duty_cycle"` // 0-100, -1 if unknown
	Channel   int    `json:"channel"`    // hardware PWM channel (pwmN); 0 if N/A
	ZoneID    int    `json:"zone_id"`    // control zone (SetFanSpeed arg); 0 if driver reports none
	Status    string `json:"status"`     // "ok", "warning", "critical"
	Unit      string `json:"unit"`       // "RPM"
}

// UpdateFanRequest is the API request for updating a fan
type UpdateFanRequest struct {
	Label    *string `json:"label,omitempty"`
	IPMIZone *int    `json:"ipmi_zone,omitempty"`
}

// SetFanSpeedRequest is the API request for manually setting fan speed
type SetFanSpeedRequest struct {
	Percent int `json:"percent"` // 0-100
	// DurationSeconds optionally expires the manual override after N seconds
	// (0 = sticky until explicitly cleared via DELETE /fans/{id}/speed).
	DurationSeconds int `json:"duration_seconds,omitempty"`
}

// IdentifyFanRequest is the API request for identifying a fan
type IdentifyFanRequest struct {
	Duration int `json:"duration,omitempty"` // Seconds to run at 100% (default: 5)
}
