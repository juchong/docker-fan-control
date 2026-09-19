package services

import (
	"testing"
	"time"

	"docker-fan-control/internal/models"
)

func intp(v int) *int { return &v }

func hwCfg() ThermalConfig {
	return ThermalConfig{
		Mode: models.ThermalModeHardware, WarningMargin: 15, EmergencyMargin: 5,
		LegacyWarning: 70, LegacyEmergency: 90, WarningEnabled: true,
	}
}

func TestEvaluateThermalHardwareMode(t *testing.T) {
	gpus := []models.GPUMetrics{
		{Index: 0, Name: "RTX", Temperature: 75, ThermalInfo: models.ThermalInfo{Limit: intp(93), LimitSource: "nvml"}}, // headroom 18 → ok
		{Index: 1, Name: "RTX", Temperature: 80, ThermalInfo: models.ThermalInfo{Limit: intp(93), LimitSource: "nvml"}}, // headroom 13 → warning
	}
	sys := &models.SystemMetrics{
		CPUPackages: []models.CPUPackageMetrics{{Index: 0, Name: "Tctl", Model: "TR 9960X", Temperature: 60}}, // no limit → default 95, headroom 35
		Drives: []models.DriveMetrics{
			{Index: 0, Model: "990 PRO", Temperature: 82, Crit: intp(85)},  // headroom 3 → critical (hwmon crit)
			{Index: 1, Model: "old sata", Temperature: 40},                  // default 70 → ok
		},
	}
	ev := EvaluateThermal(hwCfg(), gpus, sys)

	if ev.State.Status != models.ThermalCritical || ev.State.Worst == nil || ev.State.Worst.Kind != "drive" || ev.State.Worst.Index != 0 {
		t.Fatalf("worst should be the drive at 3 °C headroom: %+v", ev.State)
	}
	if gpus[0].Status != models.ThermalOK || gpus[1].Status != models.ThermalWarning {
		t.Errorf("gpu statuses: %s %s", gpus[0].Status, gpus[1].Status)
	}
	if *gpus[1].Headroom != 13 || *gpus[1].Limit != 93 || gpus[1].LimitSource != "nvml" {
		t.Errorf("gpu1 annotation: %+v", gpus[1].ThermalInfo)
	}
	if *sys.CPUPackages[0].Limit != defaultLimitCPU || sys.CPUPackages[0].LimitSource != "default" {
		t.Errorf("cpu should use the class default: %+v", sys.CPUPackages[0].ThermalInfo)
	}
	if sys.Drives[0].LimitSource != "hwmon" || sys.Drives[0].Status != models.ThermalCritical {
		t.Errorf("nvme drive: %+v", sys.Drives[0].ThermalInfo)
	}
	if *sys.Drives[1].Limit != defaultLimitDrive || sys.Drives[1].Status != models.ThermalOK {
		t.Errorf("default drive: %+v", sys.Drives[1].ThermalInfo)
	}
	// Worst-first ordering by headroom.
	if len(ev.Devices) != 5 || ev.Devices[0].Kind != "drive" || ev.Devices[1].Kind != "gpu" || ev.Devices[1].Index != 1 {
		t.Errorf("ordering: %+v", ev.Devices)
	}
}

func TestEvaluateThermalOverrideWins(t *testing.T) {
	cfg := hwCfg()
	cfg.LimitGPU = intp(80)
	gpus := []models.GPUMetrics{{Index: 0, Temperature: 76, ThermalInfo: models.ThermalInfo{Limit: intp(93), LimitSource: "nvml"}}}
	ev := EvaluateThermal(cfg, gpus, nil)
	if *gpus[0].Limit != 80 || gpus[0].LimitSource != "override" || gpus[0].Status != models.ThermalCritical {
		t.Errorf("override should win and put 76 °C within 5 of 80: %+v", gpus[0].ThermalInfo)
	}
	if ev.State.Status != models.ThermalCritical {
		t.Errorf("state: %+v", ev.State)
	}
}

// Legacy mode must make exactly the decisions the old single-threshold code
// made: hottest reading vs warning_temp / emergency_temp, warnings gated by
// warning_enabled, board temps not involved.
func TestEvaluateThermalLegacyEquivalence(t *testing.T) {
	cfg := ThermalConfig{Mode: models.ThermalModeLegacy, LegacyWarning: 70, LegacyEmergency: 90, WarningEnabled: true}
	cases := []struct {
		gpu, cpu, drive float64
		want            string
	}{
		{50, 60, 40, models.ThermalOK},
		{69.9, 60, 40, models.ThermalOK},
		{70, 60, 40, models.ThermalWarning},
		{50, 75, 40, models.ThermalWarning},
		{50, 60, 89.9, models.ThermalWarning},
		{50, 60, 90, models.ThermalCritical},
		{95, 60, 40, models.ThermalCritical},
	}
	for _, tc := range cases {
		gpus := []models.GPUMetrics{{Index: 0, Temperature: int(tc.gpu), ThermalInfo: models.ThermalInfo{Limit: intp(93)}}}
		sys := &models.SystemMetrics{
			CPUPackages: []models.CPUPackageMetrics{{Index: 0, Temperature: tc.cpu}},
			Drives:      []models.DriveMetrics{{Index: 0, Temperature: int(tc.drive), Crit: intp(85)}},
		}
		ev := EvaluateThermal(cfg, gpus, sys)
		if ev.State.Status != tc.want {
			t.Errorf("legacy gpu=%v cpu=%v drive=%v: got %s want %s", tc.gpu, tc.cpu, tc.drive, ev.State.Status, tc.want)
		}
		// In legacy mode the hardware limit is irrelevant: a drive at 89 °C is
		// only a warning even though its own crit is 85.
		if gpus[0].LimitSource != "legacy" || *gpus[0].Limit != 90 {
			t.Errorf("legacy annotation: %+v", gpus[0].ThermalInfo)
		}
	}
	// warning_enabled=false suppresses warnings but never emergencies.
	cfg.WarningEnabled = false
	ev := EvaluateThermal(cfg, []models.GPUMetrics{{Temperature: 80}}, nil)
	if ev.State.Status != models.ThermalOK {
		t.Errorf("warnings disabled: %s", ev.State.Status)
	}
	ev = EvaluateThermal(cfg, []models.GPUMetrics{{Temperature: 91}}, nil)
	if ev.State.Status != models.ThermalCritical {
		t.Errorf("emergency must not depend on warning_enabled: %s", ev.State.Status)
	}
}

func TestShouldExitEmergencyHysteresis(t *testing.T) {
	cfg := hwCfg()
	now := time.Now()
	since := now.Add(-time.Minute)
	ev := func(headroom int) ThermalEvaluation {
		return ThermalEvaluation{Devices: []models.ThermalDevice{{Headroom: headroom}}}
	}
	// Needs margin(5) + extra(5) = more than 10 °C of headroom.
	if ShouldExitEmergency(cfg, ev(10), since, now) {
		t.Error("10 °C headroom is not enough to exit (needs > 10)")
	}
	if !ShouldExitEmergency(cfg, ev(11), since, now) {
		t.Error("11 °C headroom after a minute should exit")
	}
	// Minimum hold time.
	if ShouldExitEmergency(cfg, ev(30), now.Add(-10*time.Second), now) {
		t.Error("must hold for at least 30 s")
	}
	// Legacy mode: exits as soon as nothing is critical, no hysteresis.
	legacy := ThermalConfig{Mode: models.ThermalModeLegacy, LegacyEmergency: 90}
	if !ShouldExitEmergency(legacy, ThermalEvaluation{State: models.ThermalState{Status: models.ThermalWarning}}, now, now) {
		t.Error("legacy mode exits immediately when not critical")
	}
	if ShouldExitEmergency(legacy, ThermalEvaluation{State: models.ThermalState{Status: models.ThermalCritical}}, since, now) {
		t.Error("legacy mode stays while critical")
	}
}

func TestDeviceLabel(t *testing.T) {
	if got := DeviceLabel(models.ThermalDevice{Kind: "gpu", Index: 0, Name: "NVIDIA RTX PRO 6000"}); got != "GPU 0 (NVIDIA RTX PRO 6000)" {
		t.Errorf("label = %q", got)
	}
	if got := DeviceLabel(models.ThermalDevice{Kind: "drive", Index: 1}); got != "Drive 1" {
		t.Errorf("label = %q", got)
	}
}
