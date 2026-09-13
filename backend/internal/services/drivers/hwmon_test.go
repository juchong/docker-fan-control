package drivers

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// fakeChip writes a hwmon chip directory under root with the given name and a
// set of pwm channels (each gets pwmN, pwmN_enable, fanN_input).
func fakeChip(t *testing.T, root, dir, name string, channels map[int][2]int) string {
	t.Helper()
	chipDir := filepath.Join(root, dir)
	if err := os.MkdirAll(chipDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(fn, val string) {
		if err := os.WriteFile(filepath.Join(chipDir, fn), []byte(val+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("name", name)
	for ch, rp := range channels {
		write("pwm"+itoa(ch), itoa(rp[0]))       // raw pwm 0-255
		write("pwm"+itoa(ch)+"_enable", "5")     // firmware auto at discovery
		write("fan"+itoa(ch)+"_input", itoa(rp[1])) // rpm
	}
	return chipDir
}

func itoa(i int) string { return strconv.Itoa(i) }

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(b))
}

func TestHwmonDiscover(t *testing.T) {
	root := t.TempDir()
	// Target chip: nct6799 with pwm1..3.
	chip := fakeChip(t, root, "hwmon2", "nct6799", map[int][2]int{
		1: {128, 900}, 2: {64, 500}, 3: {0, 0},
	})
	// Decoy: a temperature chip with no pwm — must be ignored.
	fakeChip(t, root, "hwmon0", "acpitz", map[int][2]int{})
	// Remove the decoy's pwm-less requirement: acpitz has no pwm1, so discover skips it.

	d := &HwmonDriver{sysfsRoot: root, origEnable: map[int]string{}}
	if err := d.discover(); err != nil {
		t.Fatalf("discover failed: %v", err)
	}
	if d.chipName != "nct6799" {
		t.Errorf("chipName = %q, want nct6799", d.chipName)
	}
	if d.path != chip {
		t.Errorf("path = %q, want %q", d.path, chip)
	}
	if len(d.channels) != 3 || d.channels[0] != 1 || d.channels[2] != 3 {
		t.Errorf("channels = %v, want [1 2 3]", d.channels)
	}
	// origEnable captured from pwmN_enable at discovery.
	if d.origEnable[1] != "5" {
		t.Errorf("origEnable[1] = %q, want 5", d.origEnable[1])
	}
}

func TestHwmonZoneLayoutIDIsChannel(t *testing.T) {
	root := t.TempDir()
	// pwm1 must exist (discovery's "controllable" marker); use channels 1 and 5
	// so the zone ID for the second channel (5) differs from its slice index (1).
	fakeChip(t, root, "hwmon0", "nct6799", map[int][2]int{1: {0, 0}, 5: {0, 0}})
	d := &HwmonDriver{sysfsRoot: root, origEnable: map[int]string{}}
	if err := d.discover(); err != nil {
		t.Fatal(err)
	}
	layout := d.buildZoneLayout()
	if len(layout.Zones) != 2 {
		t.Fatalf("expected 2 zones, got %d", len(layout.Zones))
	}
	// Zone ID must equal the hardware channel number, not the slice index.
	ids := map[int]bool{layout.Zones[0].ID: true, layout.Zones[1].ID: true}
	if !ids[1] || !ids[5] {
		t.Errorf("zone IDs = %v, want {1,5}", ids)
	}
}

func TestHwmonDetectFansAndReadings(t *testing.T) {
	root := t.TempDir()
	// pwm 255 → 100%, pwm 128 → ~50%.
	fakeChip(t, root, "hwmon0", "nct6799", map[int][2]int{1: {255, 1200}, 2: {128, 600}})
	d := &HwmonDriver{sysfsRoot: root, origEnable: map[int]string{}}
	if err := d.discover(); err != nil {
		t.Fatal(err)
	}

	fans, err := d.DetectFans(context.Background())
	if err != nil || len(fans) != 2 {
		t.Fatalf("DetectFans = %v (err %v), want 2 fans", fans, err)
	}
	if fans[0].SensorID != "fan1" || fans[0].Channel != 1 || fans[0].ZoneID != 1 {
		t.Errorf("fan[0] identity wrong: %+v", fans[0])
	}
	if fans[0].RPM != 1200 || fans[0].DutyCycle != 100 {
		t.Errorf("fan[0] rpm/duty = %d/%d, want 1200/100", fans[0].RPM, fans[0].DutyCycle)
	}

	readings, err := d.GetFanReadings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if readings["fan2"].RPM != 600 || readings["fan2"].DutyCycle != 50 {
		t.Errorf("fan2 reading = %+v, want rpm 600 duty 50", readings["fan2"])
	}
}

func TestHwmonSetFanSpeed(t *testing.T) {
	root := t.TempDir()
	chip := fakeChip(t, root, "hwmon0", "nct6799", map[int][2]int{1: {0, 0}, 2: {0, 0}})
	d := &HwmonDriver{sysfsRoot: root, origEnable: map[int]string{}}
	if err := d.discover(); err != nil {
		t.Fatal(err)
	}

	if err := d.SetFanSpeed(context.Background(), 2, 50); err != nil {
		t.Fatalf("SetFanSpeed: %v", err)
	}
	// 50% → pctToPWM = (50*255+50)/100 = 128; enable set to manual "1".
	if got := readFile(t, filepath.Join(chip, "pwm2")); got != "128" {
		t.Errorf("pwm2 = %q, want 128", got)
	}
	if got := readFile(t, filepath.Join(chip, "pwm2_enable")); got != "1" {
		t.Errorf("pwm2_enable = %q, want 1 (manual)", got)
	}

	// A channel that isn't present must be rejected.
	if err := d.SetFanSpeed(context.Background(), 7, 50); err == nil {
		t.Error("expected error setting absent channel 7")
	}
}

func TestPWMConversions(t *testing.T) {
	for _, tt := range []struct{ pct, pwm int }{{0, 0}, {50, 128}, {100, 255}} {
		if got := pctToPWM(tt.pct); got != tt.pwm {
			t.Errorf("pctToPWM(%d) = %d, want %d", tt.pct, got, tt.pwm)
		}
	}
	for _, tt := range []struct{ pwm, pct int }{{0, 0}, {128, 50}, {255, 100}} {
		if got := pwmToPct(tt.pwm); got != tt.pct {
			t.Errorf("pwmToPct(%d) = %d, want %d", tt.pwm, got, tt.pct)
		}
	}
	// Clamping.
	if pctToPWM(150) != 255 || pctToPWM(-5) != 0 {
		t.Error("pctToPWM clamp failed")
	}
}

func TestHwmonPreferNameMismatch(t *testing.T) {
	root := t.TempDir()
	fakeChip(t, root, "hwmon0", "nct6799", map[int][2]int{1: {0, 0}})
	d := &HwmonDriver{sysfsRoot: root, preferName: "nct6796", origEnable: map[int]string{}}
	if err := d.discover(); err == nil {
		t.Error("expected discover to fail when preferName does not match any chip")
	}
}
