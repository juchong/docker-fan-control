package services

import (
	"fmt"
	"math"
	"time"

	"docker-fan-control/internal/models"
)

// Thermal evaluation: instead of comparing the single hottest reading against
// one absolute threshold, every monitored device is judged against its OWN
// limit — the point at which it throttles or is out of spec — and the margins
// in ThermalConfig decide how much headroom counts as "warning" or "critical".
//
// Limits, in order of preference: an operator override for the device class,
// the value the hardware publishes (NVML thresholds for GPUs, hwmon crit for
// drives, coretemp crit for Intel CPUs), else a conservative class default.
// Board/VRM/PCH temperatures never take part (their aux channels read bogus).
//
// ThermalModeLegacy reproduces the historical behaviour exactly: warning when
// the hottest reading >= warning_temp, critical when >= emergency_temp.

// Class defaults used when neither an override nor the hardware gives a limit.
const (
	defaultLimitGPU   = 90 // NVIDIA slowdown thresholds are typically 90–95 °C
	defaultLimitCPU   = 95 // AMD Tctl throttle point; Intel coretemp publishes its own
	defaultLimitDrive = 70 // common SATA HDD/SSD operating maximum; NVMe publishes crit
)

// Emergency exit hysteresis (hardware mode): stay in emergency until every
// device has this much headroom beyond the emergency margin, and for at least
// emergencyMinHold, so the fans don't chatter around the threshold.
const (
	emergencyExitExtra = 5
	emergencyMinHold   = 30 * time.Second
)

// ThermalConfig is the operator configuration for thermal evaluation.
type ThermalConfig struct {
	Mode            string
	WarningMargin   int
	EmergencyMargin int
	LimitGPU        *int
	LimitCPU        *int
	LimitDrive      *int

	// Legacy single thresholds (also the fallback in hardware mode for a device
	// class with no known limit).
	LegacyWarning   float64
	LegacyEmergency float64
	WarningEnabled  bool
}

// thermalConfigFrom builds the config from stored settings.
func thermalConfigFrom(s *models.AppSettings) ThermalConfig {
	cfg := ThermalConfig{
		Mode:            s.ThermalLimitsMode,
		WarningMargin:   s.WarningMargin,
		EmergencyMargin: s.EmergencyMargin,
		LimitGPU:        s.LimitGPU,
		LimitCPU:        s.LimitCPU,
		LimitDrive:      s.LimitDrive,
		LegacyWarning:   float64(s.WarningTemp),
		LegacyEmergency: float64(s.EmergencyTemp),
		WarningEnabled:  s.WarningEnabled,
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
		d := judge(cfg, "gpu", g.Index, g.Name, float64(g.Temperature), limit, src, &g.ThermalInfo)
		devices = append(devices, d)
	}
	if sys != nil {
		for i := range sys.CPUPackages {
			c := &sys.CPUPackages[i]
			name := c.Model
			if name == "" {
				name = c.Name
			}
			limit, src := resolveLimit(cfg, cfg.LimitCPU, c.Limit, c.LimitSource, defaultLimitCPU)
			d := judge(cfg, "cpu", c.Index, name, c.Temperature, limit, src, &c.ThermalInfo)
			devices = append(devices, d)
		}
		for i := range sys.Drives {
			dr := &sys.Drives[i]
			// The drive's own critical threshold (hwmon) is its hardware limit.
			hw, hwSrc := dr.Limit, dr.LimitSource
			if hw == nil && dr.Crit != nil {
				hw, hwSrc = dr.Crit, "hwmon"
			}
			limit, src := resolveLimit(cfg, cfg.LimitDrive, hw, hwSrc, defaultLimitDrive)
			d := judge(cfg, "drive", dr.Index, dr.Model, float64(dr.Temperature), limit, src, &dr.ThermalInfo)
			devices = append(devices, d)
		}
	}

	// Worst first: least headroom in hardware mode, hottest in legacy mode.
	sortDevices(cfg, devices)

	state := models.ThermalState{
		Mode:            cfg.Mode,
		Status:          models.ThermalOK,
		WarningMargin:   cfg.WarningMargin,
		EmergencyMargin: cfg.EmergencyMargin,
	}
	if len(devices) > 0 {
		w := devices[0]
		state.Worst = &w
		state.Status = w.Status
	}
	return ThermalEvaluation{State: state, Devices: devices}
}

// resolveLimit picks the limit for a device: override > hardware > default.
// In legacy mode the "limit" is the emergency temperature itself.
func resolveLimit(cfg ThermalConfig, override, hardware *int, hardwareSrc string, classDefault int) (int, string) {
	if cfg.Mode == models.ThermalModeLegacy {
		return int(math.Round(cfg.LegacyEmergency)), "legacy"
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

// judge classifies one device and fills its ThermalInfo.
func judge(cfg ThermalConfig, kind string, index int, name string, temp float64, limit int, src string, info *models.ThermalInfo) models.ThermalDevice {
	headroom := int(math.Round(float64(limit) - temp))
	status := models.ThermalOK
	if cfg.Mode == models.ThermalModeLegacy {
		if temp >= cfg.LegacyEmergency {
			status = models.ThermalCritical
		} else if cfg.WarningEnabled && temp >= cfg.LegacyWarning {
			status = models.ThermalWarning
		}
	} else {
		if headroom <= cfg.EmergencyMargin {
			status = models.ThermalCritical
		} else if cfg.WarningEnabled && headroom <= cfg.WarningMargin {
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

func sortDevices(cfg ThermalConfig, devices []models.ThermalDevice) {
	less := func(a, b models.ThermalDevice) bool {
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

// ShouldExitEmergency reports whether an active emergency may end. Hardware
// mode applies hysteresis (extra headroom + minimum hold); legacy mode ends it
// as soon as no device is critical, exactly as before.
func ShouldExitEmergency(cfg ThermalConfig, ev ThermalEvaluation, since time.Time, now time.Time) bool {
	if cfg.Mode == models.ThermalModeLegacy {
		return ev.State.Status != models.ThermalCritical
	}
	if now.Sub(since) < emergencyMinHold {
		return false
	}
	for _, d := range ev.Devices {
		if d.Headroom <= cfg.EmergencyMargin+emergencyExitExtra {
			return false
		}
	}
	return true
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

// DeviceKey is the throttle key for per-device event logging.
func DeviceKey(d models.ThermalDevice) string { return fmt.Sprintf("%s:%d", d.Kind, d.Index) }
