package services

import (
	"testing"

	"docker-fan-control/internal/models"
)

func intp(v int) *int { return &v }

func hwCfg() ThermalConfig {
	return ThermalConfig{
		Mode: models.ThermalModeHardware, WarningMargin: 10,
		LegacyWarning: 70, EmergencyTemp: 90, WarningEnabled: true,
	}
}

func TestEvaluateThermalHardwareMode(t *testing.T) {
	gpus := []models.GPUMetrics{
		{Index: 0, Name: "RTX", Temperature: 80, ThermalInfo: models.ThermalInfo{Limit: intp(93), LimitSource: "nvml"}}, // headroom 13 → ok
		{Index: 1, Name: "RTX", Temperature: 85, ThermalInfo: models.ThermalInfo{Limit: intp(93), LimitSource: "nvml"}}, // headroom 8 → warning
	}
	sys := &models.SystemMetrics{
		CPUPackages: []models.CPUPackageMetrics{{Index: 0, Name: "Tctl", Model: "TR 9960X", Temperature: 60}}, // no limit → default 95
		Drives: []models.DriveMetrics{
			{Index: 0, Model: "990 PRO", Temperature: 78, Crit: intp(85)}, // headroom 7 → warning (hwmon crit)
			{Index: 1, Model: "old sata", Temperature: 40},                 // default 70 → ok
		},
	}
	ev := EvaluateThermal(hwCfg(), gpus, sys)

	if ev.State.Status != models.ThermalWarning || ev.State.Worst == nil || ev.State.Worst.Kind != "drive" || ev.State.Worst.Index != 0 {
		t.Fatalf("worst should be the drive at 7 °C headroom: %+v", ev.State)
	}
	if gpus[0].Status != models.ThermalOK || gpus[1].Status != models.ThermalWarning {
		t.Errorf("gpu statuses: %s %s", gpus[0].Status, gpus[1].Status)
	}
	if *gpus[1].Headroom != 8 || *gpus[1].Limit != 93 || gpus[1].LimitSource != "nvml" {
		t.Errorf("gpu1 annotation: %+v", gpus[1].ThermalInfo)
	}
	if *sys.CPUPackages[0].Limit != defaultLimitCPU || sys.CPUPackages[0].LimitSource != "default" {
		t.Errorf("cpu should use the class default: %+v", sys.CPUPackages[0].ThermalInfo)
	}
	if sys.Drives[0].LimitSource != "hwmon" || sys.Drives[0].Status != models.ThermalWarning {
		t.Errorf("nvme drive: %+v", sys.Drives[0].ThermalInfo)
	}
	if *sys.Drives[1].Limit != defaultLimitDrive || sys.Drives[1].Status != models.ThermalOK {
		t.Errorf("default drive: %+v", sys.Drives[1].ThermalInfo)
	}
	// Worst-first ordering by headroom.
	if len(ev.Devices) != 5 || ev.Devices[0].Kind != "drive" || ev.Devices[1].Kind != "gpu" || ev.Devices[1].Index != 1 {
		t.Errorf("ordering: %+v", ev.Devices)
	}
	if ev.State.EmergencyTemp != 90 || ev.State.WarningMargin != 10 {
		t.Errorf("state: %+v", ev.State)
	}
}

// The emergency ("critical") is the unchanged fan-behaviour rule — a reading
// at/above emergency_temp — in BOTH modes; the per-device limit never moves
// it. A drive 2 °C below its own crit is only a warning if that is below
// emergency_temp, and a GPU below its limit but above emergency_temp is
// critical.
func TestCriticalIsTheEmergencyRuleOnly(t *testing.T) {
	cfg := hwCfg()
	gpus := []models.GPUMetrics{{Index: 0, Temperature: 91, ThermalInfo: models.ThermalInfo{Limit: intp(95)}}} // headroom 4 but >= 90
	sys := &models.SystemMetrics{Drives: []models.DriveMetrics{{Index: 0, Temperature: 83, Crit: intp(85)}}}    // 2 from crit but < 90
	ev := EvaluateThermal(cfg, gpus, sys)
	if gpus[0].Status != models.ThermalCritical {
		t.Errorf("gpu at 91 must be critical by the emergency rule: %+v", gpus[0].ThermalInfo)
	}
	if sys.Drives[0].Status != models.ThermalWarning {
		t.Errorf("drive at 83 (crit 85, emergency 90) must be a warning, not critical: %+v", sys.Drives[0].ThermalInfo)
	}
	if ev.State.Status != models.ThermalCritical {
		t.Errorf("state: %+v", ev.State)
	}
	// A per-class override changes warnings, never the emergency.
	cfg.LimitDrive = intp(80)
	ev = EvaluateThermal(cfg, nil, &models.SystemMetrics{Drives: []models.DriveMetrics{{Index: 0, Temperature: 79, Crit: intp(85)}}})
	if ev.Devices[0].Status != models.ThermalWarning || ev.Devices[0].Limit != 80 {
		t.Errorf("override: %+v", ev.Devices[0])
	}
}

// Legacy mode must make exactly the decisions the old single-threshold code
// made: hottest reading vs warning_temp / emergency_temp, warnings gated by
// warning_enabled, board temps not involved.
func TestEvaluateThermalLegacyEquivalence(t *testing.T) {
	cfg := ThermalConfig{Mode: models.ThermalModeLegacy, LegacyWarning: 70, EmergencyTemp: 90, WarningEnabled: true}
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
		if gpus[0].LimitSource != "legacy" || *gpus[0].Limit != 90 {
			t.Errorf("legacy annotation: %+v", gpus[0].ThermalInfo)
		}
	}
	// warning_enabled=false suppresses warnings but never the emergency.
	cfg.WarningEnabled = false
	ev := EvaluateThermal(cfg, []models.GPUMetrics{{Temperature: 80}}, nil)
	if ev.State.Status != models.ThermalOK {
		t.Errorf("warnings disabled: %s", ev.State.Status)
	}
	ev = EvaluateThermal(cfg, []models.GPUMetrics{{Temperature: 91}}, nil)
	if ev.State.Status != models.ThermalCritical {
		t.Errorf("emergency must not depend on warning_enabled: %s", ev.State.Status)
	}
	// Same in hardware mode.
	hw := hwCfg()
	hw.WarningEnabled = false
	ev = EvaluateThermal(hw, []models.GPUMetrics{{Temperature: 92, ThermalInfo: models.ThermalInfo{Limit: intp(93)}}}, nil)
	if ev.State.Status != models.ThermalCritical {
		t.Errorf("hardware mode emergency with warnings disabled: %s", ev.State.Status)
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
