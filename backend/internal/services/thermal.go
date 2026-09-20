package services

import (
	"fmt"
	"math"

	"docker-fan-control/internal/models"
)

// Thermal evaluation.
//
// WARNINGS are judged per device: every GPU, CPU and drive is compared against
// its OWN limit — the point at which it throttles or is out of spec — and
// WarningMargin decides how much headroom counts as "warning". Limits, in
// order of preference: an operator override for the device class, the value
// the hardware publishes (NVML thresholds for GPUs, hwmon crit for drives,
// coretemp crit for Intel CPUs), else a conservative class default. This is
// monitoring only: it changes nothing about how fans are driven.
//
// The EMERGENCY (all fans to the emergency speed) is deliberately NOT part of
// this: it remains the single rule it always was — any GPU/CPU/drive reading
// at or above emergency_temp — in both modes, so the limits feature cannot
// change fan behaviour. A device meeting that rule is reported "critical".
//
// ThermalModeLegacy reproduces the historical warning as well: the hottest
// reading >= warning_temp.

// Class defaults used when neither an override nor the hardware gives a limit.
const (
	defaultLimitGPU   = 90 // NVIDIA slowdown thresholds are typically 90–95 °C
	defaultLimitCPU   = 95 // AMD Tctl throttle point; Intel coretemp publishes its own
	defaultLimitDrive = 70 // common SATA HDD/SSD operating maximum; NVMe publishes crit
)

// ThermalConfig is the operator configuration for thermal evaluation.
type ThermalConfig struct {
	Mode          string
	WarningMargin int // °C of headroom at/below which a device is "warning" (hardware mode)
	LimitGPU      *int
	LimitCPU      *int
	LimitDrive    *int

	// Legacy warning threshold (legacy mode only) and the emergency threshold
	// (both modes — the fan-behaviour rule this feature does not touch).
	LegacyWarning   float64
	EmergencyTemp   float64
	WarningEnabled  bool
}

// thermalConfigFrom builds the config from stored settings.
func thermalConfigFrom(s *models.AppSettings) ThermalConfig {
	cfg := ThermalConfig{
		Mode:           s.ThermalLimitsMode,
		WarningMargin:  s.WarningMargin,
		LimitGPU:       s.LimitGPU,
		LimitCPU:       s.LimitCPU,
		LimitDrive:     s.LimitDrive,
		LegacyWarning:  float64(s.WarningTemp),
		EmergencyTemp:  float64(s.EmergencyTemp),
		WarningEnabled: s.WarningEnabled,
	}
	if cfg.Mode == "" {
		cfg.Mode = models.ThermalModeLegacy
	}
	return cfg
}

// ThermalEvaluation is the outcome for one cycle.
type ThermalEvaluation struct {
	State   models.ThermalState
	Devices []models.ThermalDevice // every evaluated device, worst first
}

// EvaluateThermal annotates the metrics in place (limit, source, headroom,
// status) and returns the per-device list and the overall state.
func EvaluateThermal(cfg ThermalConfig, gpus []models.GPUMetrics, sys *models.SystemMetrics) ThermalEvaluation {
	var devices []models.ThermalDevice

	for i := range gpus {
		g := &gpus[i]
		limit, src := resolveLimit(cfg, cfg.LimitGPU, g.Limit, g.LimitSource, defaultLimitGPU)
		devices = append(devices, judge(cfg, "gpu", g.Index, g.Name, float64(g.Temperature), limit, src, &g.ThermalInfo))
	}
	if sys != nil {
		for i := range sys.CPUPackages {
			c := &sys.CPUPackages[i]
			name := c.Model
			if name == "" {
				name = c.Name
			}
			limit, src := resolveLimit(cfg, cfg.LimitCPU, c.Limit, c.LimitSource, defaultLimitCPU)
			devices = append(devices, judge(cfg, "cpu", c.Index, name, c.Temperature, limit, src, &c.ThermalInfo))
		}
		for i := range sys.Drives {
			dr := &sys.Drives[i]
			// The drive's own critical threshold (hwmon) is its hardware limit.
			hw, hwSrc := dr.Limit, dr.LimitSource
			if hw == nil && dr.Crit != nil {
				hw, hwSrc = dr.Crit, "hwmon"
			}
			limit, src := resolveLimit(cfg, cfg.LimitDrive, hw, hwSrc, defaultLimitDrive)
			devices = append(devices, judge(cfg, "drive", dr.Index, dr.Model, float64(dr.Temperature), limit, src, &dr.ThermalInfo))
		}
	}

	// Worst first: least headroom in hardware mode, hottest in legacy mode.
	sortDevices(cfg, devices)

	state := models.ThermalState{
		Mode:          cfg.Mode,
		Status:        models.ThermalOK,
		WarningMargin: cfg.WarningMargin,
		EmergencyTemp: int(math.Round(cfg.EmergencyTemp)),
	}
	if len(devices) > 0 {
		w := devices[0]
		state.Worst = &w
		state.Status = w.Status
	}
	return ThermalEvaluation{State: state, Devices: devices}
}

// resolveLimit picks the limit for a device: override > hardware > default.
// In legacy mode the "limit" shown is the emergency temperature itself.
func resolveLimit(cfg ThermalConfig, override, hardware *int, hardwareSrc string, classDefault int) (int, string) {
	if cfg.Mode == models.ThermalModeLegacy {
		return int(math.Round(cfg.EmergencyTemp)), "legacy"
	}
	if override != nil && *override > 0 {
		return *override, "override"
	}
	if hardware != nil && *hardware > 0 {
		if hardwareSrc == "" {
			hardwareSrc = "hardware"
		}
		return *hardware, hardwareSrc
	}
	return classDefault, "default"
}

// judge classifies one device and fills its ThermalInfo. "critical" is the
// emergency rule (temp >= emergency_temp) in both modes; "warning" is
// headroom-based in hardware mode and the legacy threshold in legacy mode.
func judge(cfg ThermalConfig, kind string, index int, name string, temp float64, limit int, src string, info *models.ThermalInfo) models.ThermalDevice {
	headroom := int(math.Round(float64(limit) - temp))
	status := models.ThermalOK
	switch {
	case temp >= cfg.EmergencyTemp:
		status = models.ThermalCritical
	case !cfg.WarningEnabled:
		// warnings disabled
	case cfg.Mode == models.ThermalModeLegacy:
		if temp >= cfg.LegacyWarning {
			status = models.ThermalWarning
		}
	default:
		if headroom <= cfg.WarningMargin {
			status = models.ThermalWarning
		}
	}

	l, h := limit, headroom
	info.Limit = &l
	info.LimitSource = src
	info.Headroom = &h
	info.Status = status

	return models.ThermalDevice{
		Kind: kind, Index: index, Name: name, Temperature: temp,
		Limit: limit, Headroom: headroom, Status: status,
	}
}

// statusRank orders statuses so a critical device always sorts first, whatever
// its headroom: the emergency rule and the per-device limits are independent.
func statusRank(s string) int {
	switch s {
	case models.ThermalCritical:
		return 2
	case models.ThermalWarning:
		return 1
	}
	return 0
}

func sortDevices(cfg ThermalConfig, devices []models.ThermalDevice) {
	less := func(a, b models.ThermalDevice) bool {
		if ra, rb := statusRank(a.Status), statusRank(b.Status); ra != rb {
			return ra > rb
		}
		if cfg.Mode == models.ThermalModeLegacy {
			return a.Temperature > b.Temperature
		}
		return a.Headroom < b.Headroom
	}
	// Insertion sort: the list is tiny.
	for i := 1; i < len(devices); i++ {
		for j := i; j > 0 && less(devices[j], devices[j-1]); j-- {
			devices[j], devices[j-1] = devices[j-1], devices[j]
		}
	}
}

// DeviceLabel renders a device for log messages: "GPU 0 (NVIDIA RTX PRO 6000)".
func DeviceLabel(d models.ThermalDevice) string {
	kind := map[string]string{"gpu": "GPU", "cpu": "CPU", "drive": "Drive"}[d.Kind]
	if kind == "" {
		kind = d.Kind
	}
	if d.Name != "" {
		return fmt.Sprintf("%s %d (%s)", kind, d.Index, d.Name)
	}
	return fmt.Sprintf("%s %d", kind, d.Index)
}

// DeviceKey identifies a device for per-device state tracking.
func DeviceKey(d models.ThermalDevice) string { return fmt.Sprintf("%s:%d", d.Kind, d.Index) }
