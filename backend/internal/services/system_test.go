package services

import (
	"os"
	"path/filepath"
	"testing"
)

// fakeHwmonChip writes <root>/class/hwmon/<dir>/name plus the given files
// (paths relative to the chip dir; "device/..." entries become a real
// directory, which EvalSymlinks resolves just like the real symlink).
func fakeHwmonChip(t *testing.T, root, dir, name string, files map[string]string) {
	t.Helper()
	chip := filepath.Join(root, "class", "hwmon", dir)
	if err := os.MkdirAll(chip, 0o755); err != nil {
		t.Fatal(err)
	}
	files["name"] = name
	for rel, content := range files {
		p := filepath.Join(chip, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if content == "<dir>" {
			if err := os.MkdirAll(p, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.WriteFile(p, []byte(content+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func newTestSystemService(t *testing.T, root string) *SystemService {
	t.Helper()
	// An empty devRoot means no block nodes, so the smartctl fallback never runs.
	return &SystemService{sysRoot: root, devRoot: t.TempDir(), driveCache: map[string]*driveInfo{}}
}

func TestDriveTemperaturesFromNVMeHwmon(t *testing.T) {
	root := t.TempDir()
	// The 990 PRO sits on a HIGHER hwmon number than the EVO Plus: ordering
	// must come from the device name, not hwmonN.
	fakeHwmonChip(t, root, "hwmon7", "nvme", map[string]string{
		"device/model":        "Samsung SSD 990 PRO 4TB                 ", // sysfs pads these
		"device/serial":       "S7KGNU0Y201325K     ",
		"device/firmware_rev": "4B2QJXD7",
		"device/nvme0n1":      "<dir>",
		"temp1_input":         "42850", "temp1_label": "Composite", "temp1_max": "81850", "temp1_crit": "84850",
		"temp2_input": "42850", "temp2_label": "Sensor 1", "temp2_max": "65261850",
		"temp3_input": "46850", "temp3_label": "Sensor 2", "temp3_max": "65261850",
	})
	fakeHwmonChip(t, root, "hwmon5", "nvme", map[string]string{
		"device/model":   "Samsung SSD 990 EVO Plus 4TB",
		"device/nvme1n1": "<dir>",
		"temp1_input":    "41850", "temp1_label": "Composite", "temp1_max": "80850", "temp1_crit": "84850",
		"temp2_input": "46850", "temp2_label": "Sensor 1",
	})
	fakeHwmonChip(t, root, "hwmon0", "k10temp", map[string]string{"temp1_input": "40000"}) // not a drive

	drives := newTestSystemService(t, root).GetDriveTemperatures()
	if len(drives) != 2 {
		t.Fatalf("expected 2 drives, got %d: %+v", len(drives), drives)
	}
	pro, evo := drives[0], drives[1]
	if pro.Device != "/dev/nvme0n1" || evo.Device != "/dev/nvme1n1" || pro.Index != 0 || evo.Index != 1 {
		t.Errorf("order/index wrong: %+v", drives)
	}
	if pro.Model != "Samsung SSD 990 PRO 4TB" || pro.Serial != "S7KGNU0Y201325K" || pro.Firmware != "4B2QJXD7" {
		t.Errorf("identity not trimmed/read: %+v", pro)
	}
	if pro.Type != "nvme" || pro.Source != "hwmon" {
		t.Errorf("type/source: %+v", pro)
	}
	// Composite is the reported temperature (rounded), with its own thresholds.
	if pro.Temperature != 43 || pro.Max == nil || *pro.Max != 82 || pro.Crit == nil || *pro.Crit != 85 {
		t.Errorf("composite/thresholds: temp=%d max=%v crit=%v", pro.Temperature, pro.Max, pro.Crit)
	}
	// The other channels are extra sensors; their placeholder thresholds are ignored.
	if len(pro.Sensors) != 2 || pro.Sensors[0].Label != "Sensor 1" || pro.Sensors[1].Temperature != 46.85 {
		t.Errorf("sensors: %+v", pro.Sensors)
	}
	if evo.Temperature != 42 || len(evo.Sensors) != 1 {
		t.Errorf("evo: %+v", evo)
	}
}

func TestDriveTemperaturesFromDrivetemp(t *testing.T) {
	root := t.TempDir()
	fakeHwmonChip(t, root, "hwmon3", "drivetemp", map[string]string{
		"device/model":     "WDC WD40EFRX-68N",
		"device/vendor":    "ATA",
		"device/block/sda": "<dir>",
		"temp1_input":      "38000", "temp1_max": "60000", "temp1_crit": "70000",
	})
	if err := os.MkdirAll(filepath.Join(root, "block", "sda", "queue"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "block", "sda", "queue", "rotational"), []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	drives := newTestSystemService(t, root).GetDriveTemperatures()
	if len(drives) != 1 {
		t.Fatalf("expected 1 drive, got %+v", drives)
	}
	d := drives[0]
	if d.Device != "/dev/sda" || d.Model != "WDC WD40EFRX-68N" || d.Type != "hdd" || d.Temperature != 38 {
		t.Errorf("drivetemp drive: %+v", d)
	}
	if d.Max == nil || *d.Max != 60 || d.Crit == nil || *d.Crit != 70 {
		t.Errorf("thresholds: max=%v crit=%v", d.Max, d.Crit)
	}
}

func TestDriveTemperaturesNoneIsEmptyNotNil(t *testing.T) {
	root := t.TempDir()
	fakeHwmonChip(t, root, "hwmon0", "k10temp", map[string]string{"temp1_input": "40000"})
	drives := newTestSystemService(t, root).GetDriveTemperatures()
	if drives == nil || len(drives) != 0 {
		t.Errorf("expected an empty, non-nil slice, got %#v", drives)
	}
}

func TestPlausibleThreshold(t *testing.T) {
	if v := plausibleThreshold("84850"); v == nil || *v != 85 {
		t.Errorf("84850 -> %v", v)
	}
	for _, bad := range []string{"", "x", "0", "65261850", "-5000"} {
		if v := plausibleThreshold(bad); v != nil {
			t.Errorf("%q should be dropped, got %d", bad, *v)
		}
	}
}
