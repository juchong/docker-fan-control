package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
)

// Setting represents a key-value setting
type Setting struct {
	Key   string     `json:"key" gorm:"primaryKey"`
	Value SettingVal `json:"value" gorm:"type:json;not null"`
}

// SettingVal is a JSON value for settings
type SettingVal struct {
	Data any `json:"value"`
}

// Scan implements sql.Scanner for SettingVal
func (s *SettingVal) Scan(value any) error {
	if value == nil {
		s.Data = nil
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

// Value implements driver.Valuer for SettingVal
func (s SettingVal) Value() (driver.Value, error) {
	return json.Marshal(s)
}

// Setting keys
const (
	SettingIPMIMode          = "ipmi_mode"           // "local" or "lan"
	SettingIPMIHost          = "ipmi_host"           // BMC IP address
	SettingIPMIUser          = "ipmi_user"           // BMC username
	SettingIPMIPass          = "ipmi_pass"           // BMC password (encrypted)
	SettingIPMICommandFormat = "ipmi_command_format" // "auto", "asrock_romed8", "asrock_legacy", "dell", "supermicro"
	SettingControlInterval   = "control_interval"    // seconds
	SettingStartupMode       = "startup_mode"        // "resume", "full", "percent"
	SettingStartupPercent    = "startup_percent"     // if startup_mode is "percent"
	SettingEmergencyTemp     = "emergency_temp"      // temperature threshold
	SettingEmergencySpeed    = "emergency_speed"     // fan speed when emergency
	SettingWarningTemp       = "warning_temp"        // temperature for warning logs
	SettingWarningEnabled    = "warning_enabled"     // whether to log warnings
	SettingSafetyOnShutdown  = "safety_on_shutdown"  // set fans to 100% on shutdown

	// Thermal limits (headroom-based, per device). "legacy" reproduces the
	// single warning_temp/emergency_temp comparison; "hardware" derives each
	// device's limit (NVML, hwmon, coretemp, or a class default) and applies
	// the margins below to its headroom.
	SettingThermalLimitsMode = "thermal_limits_mode" // "hardware" or "legacy"
	SettingWarningMargin     = "warning_margin"      // °C of headroom at/below which a device is "warning"
	SettingEmergencyMargin   = "emergency_margin"    // °C of headroom at/below which a device is "critical"
	SettingLimitGPU          = "limit_gpu"           // optional absolute per-class limit overrides (°C)
	SettingLimitCPU          = "limit_cpu"
	SettingLimitDrive        = "limit_drive"

	// Motherboard settings
	SettingMotherboardVendor = "motherboard_vendor" // Motherboard vendor (e.g., "ASRock Rack")
	SettingMotherboardModel  = "motherboard_model"  // Motherboard model (e.g., "ROMED8-2T")
	SettingMotherboardDriver = "motherboard_driver" // Manually selected driver
	SettingZoneLayout        = "zone_layout"        // Custom zone layout (JSON)
)

// ZoneLayout represents a customizable zone layout
type ZoneLayout struct {
	Zones []ZoneDefinition `json:"zones"`
}

// ZoneDefinition represents a fan zone definition
type ZoneDefinition struct {
	ID          int      `json:"id"`
	Name        string   `json:"name"`
	FanIndices  []int    `json:"fan_indices"`
	Description string   `json:"description,omitempty"`
	IsDefault   bool     `json:"is_default"`
	Color       string   `json:"color,omitempty"` // Optional color for UI
	Chip        string   `json:"chip,omitempty"`  // controller chip (multi-chip hwmon boards)
}

// IPMI command format values
const (
	IPMIFormatAuto         = "auto"
	IPMIFormatAsrockROMED8 = "asrock_romed8"
	IPMIFormatAsrockLegacy = "asrock_legacy"
	IPMIFormatDell         = "dell"
	IPMIFormatSupermicro   = "supermicro"
)

// AppSettings represents all application settings
type AppSettings struct {
	IPMIMode          string `json:"ipmi_mode"`
	IPMIHost          string `json:"ipmi_host,omitempty"`
	IPMIUser          string `json:"ipmi_user,omitempty"`
	IPMICommandFormat string `json:"ipmi_command_format"` // "auto", "asrock_romed8", "asrock_legacy", "dell", "supermicro"
	ControlInterval   int    `json:"control_interval"`    // seconds
	StartupMode       string `json:"startup_mode"`
	StartupPercent    int    `json:"startup_percent,omitempty"`
	EmergencyTemp     int    `json:"emergency_temp"`
	EmergencySpeed    int    `json:"emergency_speed"`
	WarningTemp       int    `json:"warning_temp"`
	WarningEnabled    bool   `json:"warning_enabled"`
	SafetyOnShutdown  bool   `json:"safety_on_shutdown"` // Set fans to 100% when stopping

	// Thermal limits (see SettingThermalLimitsMode). Overrides are nil when unset.
	ThermalLimitsMode string `json:"thermal_limits_mode"`
	WarningMargin     int    `json:"warning_margin"`
	EmergencyMargin   int    `json:"emergency_margin"`
	LimitGPU          *int   `json:"limit_gpu,omitempty"`
	LimitCPU          *int   `json:"limit_cpu,omitempty"`
	LimitDrive        *int   `json:"limit_drive,omitempty"`

	// Motherboard-specific settings
	MotherboardVendor string      `json:"motherboard_vendor,omitempty"`
	MotherboardModel  string      `json:"motherboard_model,omitempty"`
	MotherboardDriver string      `json:"motherboard_driver,omitempty"`
	ZoneLayout        *ZoneLayout `json:"zone_layout,omitempty"`
}

// UpdateSettingsRequest is the API request for updating settings
type UpdateSettingsRequest struct {
	IPMIMode          *string `json:"ipmi_mode,omitempty"`
	IPMIHost          *string `json:"ipmi_host,omitempty"`
	IPMIUser          *string `json:"ipmi_user,omitempty"`
	IPMIPass          *string `json:"ipmi_pass,omitempty"`           // Only set if changing
	IPMICommandFormat *string `json:"ipmi_command_format,omitempty"` // "auto", "asrock_romed8", etc.
	ControlInterval   *int    `json:"control_interval,omitempty"`
	StartupMode       *string `json:"startup_mode,omitempty"`
	StartupPercent    *int    `json:"startup_percent,omitempty"`
	EmergencyTemp     *int    `json:"emergency_temp,omitempty"`
	EmergencySpeed    *int    `json:"emergency_speed,omitempty"`
	WarningTemp       *int    `json:"warning_temp,omitempty"`
	WarningEnabled    *bool   `json:"warning_enabled,omitempty"`
	SafetyOnShutdown  *bool   `json:"safety_on_shutdown,omitempty"`

	// Thermal limits. A per-class override of 0 clears it (back to hardware/default).
	ThermalLimitsMode *string `json:"thermal_limits_mode,omitempty"`
	WarningMargin     *int    `json:"warning_margin,omitempty"`
	EmergencyMargin   *int    `json:"emergency_margin,omitempty"`
	LimitGPU          *int    `json:"limit_gpu,omitempty"`
	LimitCPU          *int    `json:"limit_cpu,omitempty"`
	LimitDrive        *int    `json:"limit_drive,omitempty"`

	// Motherboard-specific settings
	MotherboardVendor *string `json:"motherboard_vendor,omitempty"`
	MotherboardModel  *string `json:"motherboard_model,omitempty"`
	MotherboardDriver *string `json:"motherboard_driver,omitempty"`
	ZoneLayout        *ZoneLayout `json:"zone_layout,omitempty"`
}

// Monitoring represents current system monitoring data
type Monitoring struct {
	GPUs       []GPUMetrics    `json:"gpus"`
	System     SystemMetrics   `json:"system"`
	Fans       []FanStatus     `json:"fans"`
	Controller ControllerState `json:"controller"`
}

// GPUMetrics represents metrics for a single GPU
// ThermalInfo is the per-device thermal annotation shared by GPU, CPU and
// drive metrics: the device's limit (throttle/critical point), where it came
// from, the headroom to it, and the resulting status. All omitted when the
// controller has not evaluated the device.
type ThermalInfo struct {
	Limit       *int   `json:"limit,omitempty"`        // °C
	LimitSource string `json:"limit_source,omitempty"` // "nvml", "hwmon", "coretemp", "default", "override", "legacy"
	Headroom    *int   `json:"headroom,omitempty"`     // °C to the limit (negative = over)
	Status      string `json:"status,omitempty"`       // ThermalOK / ThermalWarning / ThermalCritical
}

// Thermal statuses.
const (
	ThermalOK       = "ok"
	ThermalWarning  = "warning"
	ThermalCritical = "critical"

	ThermalModeHardware = "hardware"
	ThermalModeLegacy   = "legacy"
)

// ThermalDevice identifies the device closest to its limit.
type ThermalDevice struct {
	Kind        string  `json:"kind"` // "gpu", "cpu", "drive"
	Index       int     `json:"index"`
	Name        string  `json:"name"`
	Temperature float64 `json:"temperature"`
	Limit       int     `json:"limit"`
	Headroom    int     `json:"headroom"`
	Status      string  `json:"status"`
}

// ThermalState summarises the last thermal evaluation.
type ThermalState struct {
	Mode            string         `json:"mode"`
	Status          string         `json:"status"` // worst device status
	EmergencyActive bool           `json:"emergency_active"`
	WarningMargin   int            `json:"warning_margin"`
	EmergencyMargin int            `json:"emergency_margin"`
	Worst           *ThermalDevice `json:"worst,omitempty"`
}

type GPUMetrics struct {
	ThermalInfo
	Index       int     `json:"index"`
	Name        string  `json:"name"`
	Temperature int     `json:"temperature"` // Celsius
	Load        int     `json:"load"`        // Percent
	FanSpeed    int     `json:"fan_speed"`   // Percent (0-100)
	MemoryUsed  uint64  `json:"memory_used"`
	MemoryTotal uint64  `json:"memory_total"`
	PowerUsage  float64 `json:"power_usage,omitempty"`  // Watts
	PowerLimit  float64 `json:"power_limit,omitempty"`  // Watts
}

// SystemMetrics represents system-wide metrics
type SystemMetrics struct {
	CPUPackages []CPUPackageMetrics `json:"cpu_packages"`
	Drives      []DriveMetrics      `json:"drives"`
	BoardTemps  []BoardTempMetrics  `json:"board_temps"` // motherboard/VRM/chipset (nct6xxx)
	CPULoad     float64             `json:"cpu_load"`     // Percent
	MemoryUsed  uint64              `json:"memory_used"`
	MemoryTotal uint64              `json:"memory_total"`
	CPUTemp *float64 `json:"cpu_temp,omitempty"` // Celsius (max of all packages)
}

// BoardTempMetrics represents a motherboard temperature sensor (Super-I/O).
type BoardTempMetrics struct {
	Index       int     `json:"index"`
	Name        string  `json:"name"`        // hwmon label, e.g. "SYSTIN", "AUXTIN0"
	Temperature float64 `json:"temperature"` // Celsius
}

// CPUPackageMetrics represents metrics for a single CPU package/socket
type CPUPackageMetrics struct {
	ThermalInfo
	Index       int     `json:"index"`
	Name        string  `json:"name"`        // sensor label, e.g., "Package id 0", "Tctl"
	Model       string  `json:"model"`       // marketing name, e.g., "AMD Ryzen 9 9950X"
	Temperature float64 `json:"temperature"` // Celsius
}

// DriveMetrics represents metrics for a single storage drive
type DriveMetrics struct {
	ThermalInfo
	Index       int           `json:"index"`
	Device      string        `json:"device"`             // e.g., "/dev/sda", "/dev/nvme0n1"
	Model       string        `json:"model"`              // e.g., "WD Red 4TB"
	Serial      string        `json:"serial,omitempty"`   // Drive serial number
	Firmware    string        `json:"firmware,omitempty"` // Firmware revision, when the source reports it
	Type        string        `json:"type"`               // "hdd", "ssd", "nvme"
	Temperature int           `json:"temperature"`        // Celsius (NVMe: the Composite channel)
	Max         *int          `json:"max,omitempty"`      // the drive's own warning threshold, if reported
	Crit        *int          `json:"crit,omitempty"`     // the drive's own critical threshold, if reported
	Sensors     []DriveSensor `json:"sensors,omitempty"`  // additional channels (NVMe "Sensor 1", "Sensor 2")
	Source      string        `json:"source,omitempty"`   // "hwmon" or "smartctl"
}

// DriveSensor is one extra temperature channel of a drive.
type DriveSensor struct {
	Label       string  `json:"label"`
	Temperature float64 `json:"temperature"` // Celsius
}

// ControllerState represents the fan controller state
type ControllerState struct {
	Running          bool     `json:"running"`
	ActiveProfiles   []string `json:"active_profiles,omitempty"`    // Names of all active profiles
	ActiveProfileIDs []uint   `json:"active_profile_ids,omitempty"` // IDs of all active profiles
	LastUpdate       string   `json:"last_update,omitempty"`
	ManualMode       bool     `json:"manual_mode"`
	
	// Motherboard information
	MotherboardVendor string `json:"motherboard_vendor,omitempty"`
	MotherboardModel  string `json:"motherboard_model,omitempty"`
	MotherboardDriver string `json:"motherboard_driver,omitempty"`
	
	// Current driver information
	DriverVendor       string            `json:"driver_vendor,omitempty"`
	DriverModel        string            `json:"driver_model,omitempty"`
	DriverCapabilities DriverCapabilities `json:"driver_capabilities,omitempty"`
	DriverWarnings     []string          `json:"driver_warnings,omitempty"` // live driver health issues (e.g. firmware overriding writes)
	Thermal            *ThermalState     `json:"thermal,omitempty"`         // last thermal evaluation (per-device limits)
}

// DriverCapabilities represents driver capabilities
type DriverCapabilities struct {
	SupportsManualMode       bool `json:"supports_manual_mode"`
	SupportsDutyCycleReading bool `json:"supports_duty_cycle_reading"`
	SupportsPerZoneControl   bool `json:"supports_per_zone_control"`
	FirmwareFallback         bool `json:"firmware_fallback"` // untargeted zones stay on the firmware curve
	MaxZones                 int  `json:"max_zones"`
	MaxFans                  int  `json:"max_fans"`
	HasStaticRPMValues       bool `json:"has_static_rpm_values"`
}
