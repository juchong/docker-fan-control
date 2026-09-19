package drivers

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fakeChip writes a hwmon chip directory under root with the given name and a
// set of pwm channels (each gets pwmN, pwmN_enable=enable, fanN_input).
func fakeChip(t *testing.T, root, dir, name, enable string, channels map[int][2]int) string {
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
		write("pwm"+itoa(ch), itoa(rp[0]))          // raw pwm 0-255
		write("pwm"+itoa(ch)+"_enable", enable)     // firmware value at discovery
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

func newTestDriver(t *testing.T, root string, allow ...string) *HwmonDriver {
	t.Helper()
	d := &HwmonDriver{sysfsRoot: root, allow: allow, origEnable: map[string]string{}}
	if err := d.discover(); err != nil {
		t.Fatalf("discover failed: %v", err)
	}
	return d
}

// Gigabyte-style two-chip board: the primary (it8689) lands on a HIGHER hwmon
// number than the secondary so slot order must come from the name, not hwmonN.
func gigabyteBoard(t *testing.T, root string) (primary, secondary string) {
	t.Helper()
	primary = fakeChip(t, root, "hwmon7", "it8689_9a0a0908", "2", map[int][2]int{
		1: {66, 1600}, 2: {63, 0}, 3: {63, 0}, 4: {63, 0}, 5: {66, 0},
	})
	secondary = fakeChip(t, root, "hwmon5", "it87952_9a0a0908", "2", map[int][2]int{
		1: {161, 1900}, 2: {161, 1970}, 3: {113, 0}, 4: {63, 1940}, 5: {63, 0},
	})
	fakeChip(t, root, "hwmon0", "k10temp", "", map[int][2]int{}) // sensor-only decoy
	return primary, secondary
}

func TestHwmonDiscoverSingleChipKeepsLegacyIDs(t *testing.T) {
	root := t.TempDir()
	chip := fakeChip(t, root, "hwmon2", "nct6799", "5", map[int][2]int{1: {128, 900}, 2: {64, 500}, 5: {0, 0}})
	fakeChip(t, root, "hwmon0", "acpitz", "", map[int][2]int{}) // no pwm → ignored

	d := newTestDriver(t, root)
	if len(d.chips) != 1 || d.chips[0].name != "nct6799" || d.chips[0].key != "nct6799" || d.chips[0].dir != chip {
		t.Fatalf("unexpected chips: %+v", d.chips)
	}
	if d.chips[0].family.id != "nct6775" {
		t.Errorf("family = %q, want nct6775", d.chips[0].family.id)
	}
	// Zone ID == channel number for the first chip (unchanged behaviour).
	layout := d.buildZoneLayout()
	ids := map[int]bool{}
	for _, z := range layout.Zones {
		ids[z.ID] = true
	}
	if len(ids) != 3 || !ids[1] || !ids[2] || !ids[5] {
		t.Errorf("zone IDs = %v, want {1,2,5}", ids)
	}
	if got := d.origEnable["nct6799/1"]; got != "5" {
		t.Errorf("origEnable = %q, want 5", got)
	}
	if d.modelLocked() != "nct6799" {
		t.Errorf("model = %q", d.modelLocked())
	}
}

func TestHwmonMultiChipIdentity(t *testing.T) {
	root := t.TempDir()
	gigabyteBoard(t, root)
	d := newTestDriver(t, root)

	if len(d.chips) != 2 {
		t.Fatalf("expected 2 chips, got %d", len(d.chips))
	}
	if d.chips[0].key != "it8689" || d.chips[1].key != "it87952" {
		t.Errorf("slot order/keys = %s,%s; want it8689,it87952 (name order, not hwmonN)", d.chips[0].key, d.chips[1].key)
	}
	if d.modelLocked() != "it8689+it87952" {
		t.Errorf("model = %q", d.modelLocked())
	}

	fans, err := d.DetectFans(context.Background())
	if err != nil || len(fans) != 10 {
		t.Fatalf("DetectFans = %d fans (err %v), want 10", len(fans), err)
	}
	if fans[0].SensorID != "it8689/fan1" || fans[0].Chip != "it8689" || fans[0].Channel != 1 || fans[0].ZoneID != 1 {
		t.Errorf("primary fan1 identity: %+v", fans[0])
	}
	if fans[5].SensorID != "it87952/fan1" || fans[5].Chip != "it87952" || fans[5].Channel != 1 || fans[5].ZoneID != 101 {
		t.Errorf("secondary fan1 identity: %+v", fans[5])
	}
	if fans[8].ZoneID != 104 || fans[8].RPM != 1940 {
		t.Errorf("secondary fan4: %+v", fans[8])
	}

	layout := d.buildZoneLayout()
	if len(layout.Zones) != 10 || layout.Zones[5].Chip != "it87952" || layout.Zones[5].Name != "it87952 fan1" {
		t.Errorf("zone layout: %+v", layout.Zones)
	}
	// The zone is named like the fan it drives so both pages show one name.
	if fans[5].Name != layout.Zones[5].Name {
		t.Errorf("fan name %q != zone name %q", fans[5].Name, layout.Zones[5].Name)
	}
	caps := d.capsLocked()
	if caps.MaxZones != 10 || !caps.PerZoneFirmwareFallback {
		t.Errorf("caps = %+v", caps)
	}
}

func TestHwmonAllowList(t *testing.T) {
	root := t.TempDir()
	gigabyteBoard(t, root)

	d := newTestDriver(t, root, "it87952")
	if len(d.chips) != 1 || d.chips[0].key != "it87952" {
		t.Errorf("allow-list should bind only it87952, got %+v", d.chips)
	}
	// The only chip is slot 0, so its zones are 1..5.
	if z := d.buildZoneLayout().Zones[0].ID; z != 1 {
		t.Errorf("single allowed chip zone = %d, want 1", z)
	}

	d = &HwmonDriver{sysfsRoot: root, allow: []string{"nct6796"}, origEnable: map[string]string{}}
	if err := d.discover(); err == nil {
		t.Error("expected discover to fail when no chip matches the allow-list")
	}
}

func TestHwmonLazyManualAndRelease(t *testing.T) {
	root := t.TempDir()
	primary, secondary := gigabyteBoard(t, root)
	d := newTestDriver(t, root)
	ctx := context.Background()

	// Opening the session must not touch any channel.
	if err := d.SetManualMode(ctx, true); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(primary, "pwm1_enable")); got != "2" {
		t.Errorf("SetManualMode(true) switched CPU fan to manual (enable=%s); should be lazy", got)
	}

	// Writing zone 102 (secondary pwm2) switches only that channel.
	if err := d.SetFanSpeed(ctx, 102, 50); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(secondary, "pwm2")); got != "128" {
		t.Errorf("pwm2 = %s, want 128", got)
	}
	if got := readFile(t, filepath.Join(secondary, "pwm2_enable")); got != "1" {
		t.Errorf("pwm2_enable = %s, want 1", got)
	}
	if got := readFile(t, filepath.Join(secondary, "pwm1_enable")); got != "2" {
		t.Errorf("untouched pwm1_enable = %s, want 2", got)
	}
	// Zone 2 is the primary's pwm2, not the secondary's.
	if err := d.SetFanSpeed(ctx, 2, 60); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(primary, "pwm2")); got != "153" {
		t.Errorf("primary pwm2 = %s, want 153", got)
	}
	// Absent zones are rejected.
	if err := d.SetFanSpeed(ctx, 7, 50); err == nil {
		t.Error("expected error for absent zone 7")
	}
	if err := d.SetFanSpeed(ctx, 201, 50); err == nil {
		t.Error("expected error for zone on a non-existent third chip")
	}

	// Releasing one zone restores firmware auto on it alone.
	if err := d.ReleaseZone(ctx, 102); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(secondary, "pwm2_enable")); got != "2" {
		t.Errorf("released pwm2_enable = %s, want 2", got)
	}
	if got := readFile(t, filepath.Join(primary, "pwm2_enable")); got != "1" {
		t.Errorf("primary pwm2 should still be manual, got %s", got)
	}

	// Closing the session releases everything touched.
	if err := d.SetManualMode(ctx, false); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(primary, "pwm2_enable")); got != "2" {
		t.Errorf("after close primary pwm2_enable = %s, want 2", got)
	}
	if d.IsManualMode() {
		t.Error("session should be closed")
	}
}

func TestHwmonReleaseFallsBackToFamilyAuto(t *testing.T) {
	// EC-driven boards read pwmN_enable=1 at discovery (in-tree it87 on
	// Gigabyte), so "restore what we saw" would leave the channel manual; the
	// family's auto value must be used instead — 2 for it87, 5 for nct6775.
	root := t.TempDir()
	ite := fakeChip(t, root, "hwmon1", "it87952", "1", map[int][2]int{1: {100, 1000}})
	nct := fakeChip(t, root, "hwmon2", "nct6799", "1", map[int][2]int{1: {100, 1000}})
	d := newTestDriver(t, root)
	ctx := context.Background()

	if err := d.SetAllFanSpeeds(ctx, 40); err != nil {
		t.Fatal(err)
	}
	if err := d.SetManualMode(ctx, false); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(ite, "pwm1_enable")); got != "2" {
		t.Errorf("it87 release enable = %s, want 2", got)
	}
	if got := readFile(t, filepath.Join(nct, "pwm1_enable")); got != "5" {
		t.Errorf("nct6775 release enable = %s, want 5", got)
	}
}

func TestHwmonIdentifyZoneRestoresPriorState(t *testing.T) {
	root := t.TempDir()
	primary, secondary := gigabyteBoard(t, root)
	d := newTestDriver(t, root)
	ctx := context.Background()

	// Owned channel: goes back to the commanded duty.
	if err := d.SetFanSpeed(ctx, 101, 40); err != nil {
		t.Fatal(err)
	}
	if err := d.IdentifyZone(ctx, 101, 5*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(secondary, "pwm1")); got != "102" {
		t.Errorf("owned channel pwm1 after identify = %s, want 102 (40%%)", got)
	}
	// Firmware-managed channel: goes back to firmware auto.
	if err := d.IdentifyZone(ctx, 1, 5*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(primary, "pwm1_enable")); got != "2" {
		t.Errorf("unowned CPU fan enable after identify = %s, want 2", got)
	}
}

func TestHwmonReadbackOverrideDetection(t *testing.T) {
	root := t.TempDir()
	_, secondary := gigabyteBoard(t, root)
	d := newTestDriver(t, root)
	ctx := context.Background()

	if err := d.SetFanSpeed(ctx, 101, 80); err != nil {
		t.Fatal(err)
	}
	// Simulate the EC rewriting the register, well after our write.
	if err := os.WriteFile(filepath.Join(secondary, "pwm1"), []byte("120\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	d.chips[1].writtenAt[1] = time.Now().Add(-time.Minute)

	for i := 0; i < overrideAfter-1; i++ {
		if _, err := d.GetFanReadings(ctx); err != nil {
			t.Fatal(err)
		}
		if w := d.DriverWarnings(); len(w) != 0 {
			t.Fatalf("warned after %d mismatches: %v", i+1, w)
		}
	}
	if _, err := d.GetFanReadings(ctx); err != nil {
		t.Fatal(err)
	}
	w := d.DriverWarnings()
	if len(w) != 1 || !strings.Contains(w[0], "it87952 pwm1") {
		t.Errorf("expected one override warning for it87952 pwm1, got %v", w)
	}
	// A matching readback clears it.
	if err := os.WriteFile(filepath.Join(secondary, "pwm1"), []byte("204\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := d.GetFanReadings(ctx); err != nil {
		t.Fatal(err)
	}
	if w := d.DriverWarnings(); len(w) != 0 {
		t.Errorf("warning should clear once readback matches, got %v", w)
	}
}

func TestHwmonReadingsModeAndUnreadableDuty(t *testing.T) {
	root := t.TempDir()
	_, secondary := gigabyteBoard(t, root)
	// it87 returns ENODATA for H2RAM channels in firmware mode: emulate an
	// unreadable pwm4 (a directory can't be read as a file, but still stats).
	if err := os.Remove(filepath.Join(secondary, "pwm4")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(secondary, "pwm4"), 0o755); err != nil {
		t.Fatal(err)
	}
	d := newTestDriver(t, root)
	ctx := context.Background()

	r, err := d.GetFanReadings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := r["it87952/fan4"]; got.DutyCycle != -1 || got.RPM != 1940 || got.ControlMode != "firmware" {
		t.Errorf("fan4 reading = %+v, want duty -1 / rpm 1940 / firmware", got)
	}
	if got := r["it87952/fan1"]; got.DutyCycle != 63 || got.ControlMode != "firmware" {
		t.Errorf("fan1 reading = %+v, want duty 63 / firmware", got)
	}
	if err := d.SetFanSpeed(ctx, 101, 100); err != nil {
		t.Fatal(err)
	}
	r, _ = d.GetFanReadings(ctx)
	if got := r["it87952/fan1"]; got.DutyCycle != 100 || got.ControlMode != "manual" {
		t.Errorf("after set, fan1 reading = %+v, want duty 100 / manual", got)
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
	if pctToPWM(150) != 255 || pctToPWM(-5) != 0 {
		t.Error("pctToPWM clamp failed")
	}
	if shortName("it8689_9a0a0908") != "it8689" || shortName("nct6799") != "nct6799" {
		t.Error("shortName")
	}
}
