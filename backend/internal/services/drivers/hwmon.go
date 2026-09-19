package drivers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"docker-fan-control/internal/models"
	"docker-fan-control/internal/services"

	"github.com/rs/zerolog/log"
)

// HwmonDriver controls fans directly via the Linux hwmon sysfs interface — the
// non-IPMI path for boards without a BMC. It binds every supported Super-I/O
// chip it finds (boards can carry more than one, e.g. Gigabyte's IT8689E +
// IT87952E pair) and exposes one control zone per PWM channel.
//
// Chip families, by hwmon "name" prefix:
//
//	nct6*  Nuvoton via the in-tree nct6775 driver (ASUS, MSI, ...):
//	       pwmN_enable 1 = manual, 5 = SmartFan IV (firmware auto)
//	it8*   ITE via it87 — in-tree, or the frankcrawford/it87 fork that newer
//	       Gigabyte chips need for control: pwmN_enable 0 = full speed,
//	       1 = manual, 2 = firmware/SmartGuardian auto
//
// Identity: zone ID = slot*zoneStride + channel, where slot is the chip's
// position in a stable name-sorted order. A single-chip board therefore keeps
// zone IDs 1..N (unchanged from before multi-chip support) and a second chip
// gets 101..1xx. SensorID = "<chip>/fanN" where chip is the hwmon name up to
// its first "_" (Gigabyte's SIV suffix "it8689_9a0a0908" → "it8689").
//
// Firmware fallback: a channel is switched to manual lazily on its first
// write; SetManualMode(true) merely opens the control session. Channels no
// profile or override ever touches stay under firmware automatic control —
// important when the CPU fan sits on a controllable channel and no profile
// covers it. SetManualMode(false) and ReleaseZone hand touched channels back
// by restoring the pwmN_enable value seen at discovery.
type HwmonDriver struct {
	*services.BaseDriver

	mu         sync.Mutex
	allow      []string // HWMON_CHIP allow-list of name prefixes; empty = every known family
	sysfsRoot  string   // hwmon class dir; defaults to /sys/class/hwmon (override in tests)
	chips      []*hwmonChip
	origEnable map[string]string // "<hwmon name>/<ch>" -> pwmN_enable seen at first discovery
	manualMode bool
}

// chipFamily captures the per-driver pwmN_enable conventions.
type chipFamily struct {
	id         string
	prefixes   []string
	autoEnable string // firmware-auto value used when the captured one is unusable
}

var chipFamilies = []chipFamily{
	{id: "nct6775", prefixes: []string{"nct6"}, autoEnable: "5"},
	{id: "it87", prefixes: []string{"it8"}, autoEnable: "2"},
}

// genericFamily is used for chips pinned by name that no family recognises.
// The hwmon sysfs convention is 0 = full speed, 1 = manual, 2+ = automatic.
var genericFamily = chipFamily{id: "generic", autoEnable: "2"}

// hwmonChip is one bound Super-I/O chip.
type hwmonChip struct {
	slot      int
	key       string // short id used in SensorIDs and zone names ("it8689")
	name      string // full hwmon name ("it8689_9a0a0908")
	dir       string // /sys/class/hwmon/hwmonN
	family    chipFamily
	channels  []int // pwm channel numbers present (1-based), sorted
	touched   map[int]bool      // channels this driver switched to manual
	commanded map[int]int       // channel -> last raw pwm we wrote
	writtenAt map[int]time.Time // channel -> time of that write
	mismatch  map[int]int       // channel -> consecutive readbacks disagreeing with commanded
}

const (
	// pwmMax is the full-scale PWM duty value exposed by hwmon (0-255).
	pwmMax = 255
	// manualEnable is the pwmN_enable value for direct PWM control in every
	// hwmon Super-I/O driver.
	manualEnable = "1"
	// zoneStride separates the zone-ID ranges of successive chips.
	zoneStride = 100
	// maxChannels bounds the pwmN scan per chip.
	maxChannels = 8
	// readbackTolerance is the raw-pwm slack allowed between what we wrote and
	// what the chip reads back before counting a mismatch.
	readbackTolerance = 2
	// overrideAfter is how many consecutive mismatching readbacks (spaced at
	// least overrideGrace after the write) flag a channel as firmware-overridden.
	overrideAfter = 3
	overrideGrace = 5 * time.Second
)

// NewHwmonDriver creates a hwmon driver. chips is the optional HWMON_CHIP
// allow-list: comma-separated hwmon name prefixes ("it8689,it87952" or
// "nct6799"); empty binds every chip of a known family that exposes PWM
// channels. Discovery is best-effort here and re-run by Discover().
func NewHwmonDriver(chips string) *HwmonDriver {
	d := &HwmonDriver{origEnable: make(map[string]string)}
	for _, p := range strings.Split(chips, ",") {
		if p = strings.TrimSpace(p); p != "" {
			d.allow = append(d.allow, p)
		}
	}

	// Best-effort discovery so the zone layout reflects real channels. main.go
	// constructs this after /sys is mounted; DetectBestDriver calls Discover too.
	_ = d.discover()
	d.BaseDriver = services.NewBaseDriver("Hwmon", d.modelLocked(), d.capsLocked(), d.buildZoneLayout())
	return d
}

// ---- discovery ------------------------------------------------------------

func (d *HwmonDriver) root() string {
	if d.sysfsRoot != "" {
		return d.sysfsRoot
	}
	return "/sys/class/hwmon"
}

// accepts reports whether a hwmon chip name should be bound, and which family
// governs it.
func (d *HwmonDriver) accepts(name string) (chipFamily, bool) {
	fam := genericFamily
	for _, f := range chipFamilies {
		for _, p := range f.prefixes {
			if strings.HasPrefix(name, p) {
				fam = f
			}
		}
	}
	if len(d.allow) == 0 {
		return fam, fam.id != genericFamily.id
	}
	for _, p := range d.allow {
		if strings.HasPrefix(name, p) {
			return fam, true
		}
	}
	return fam, false
}

// discover resolves every acceptable chip and its PWM channels. Safe to call
// repeatedly. Returns an error if no controllable chip is found. Caller holds
// mu (or is the constructor).
func (d *HwmonDriver) discover() error {
	namePaths, _ := filepath.Glob(filepath.Join(d.root(), "hwmon*/name"))

	var found []*hwmonChip
	for _, np := range namePaths {
		b, err := os.ReadFile(np)
		if err != nil {
			continue
		}
		name := strings.TrimSpace(string(b))
		fam, ok := d.accepts(name)
		if !ok {
			continue
		}
		dir := filepath.Dir(np)
		var channels []int
		for n := 1; n <= maxChannels; n++ {
			if _, err := os.Stat(filepath.Join(dir, fmt.Sprintf("pwm%d", n))); err == nil {
				channels = append(channels, n)
			}
		}
		if len(channels) == 0 {
			continue // sensor-only chip (or a name pinned by mistake)
		}
		found = append(found, &hwmonChip{
			name: name, dir: dir, family: fam, channels: channels,
			touched: map[int]bool{}, commanded: map[int]int{}, writtenAt: map[int]time.Time{}, mismatch: map[int]int{},
		})
	}
	if len(found) == 0 {
		return fmt.Errorf("hwmon: no controllable chip found (allow=%v)", d.allow)
	}

	// Stable order: by name, then directory — hwmonN numbering is not stable
	// across boots, chip names are.
	sort.Slice(found, func(i, j int) bool {
		if found[i].name != found[j].name {
			return found[i].name < found[j].name
		}
		return found[i].dir < found[j].dir
	})

	// Short keys, made unique.
	keyCount := map[string]int{}
	for _, c := range found {
		c.key = shortName(c.name)
		keyCount[c.key]++
	}
	for _, c := range found {
		if keyCount[c.key] > 1 {
			c.key = c.name
		}
	}
	seen := map[string]int{}
	for i, c := range found {
		c.slot = i
		if seen[c.key] > 0 {
			c.key = fmt.Sprintf("%s@%d", c.key, i)
		}
		seen[c.key]++
	}

	// Capture firmware enable values the first time a chip/channel is seen, and
	// carry per-channel state across a re-discover so a channel we already put
	// in manual mode is still known to be ours.
	prev := map[string]*hwmonChip{}
	for _, c := range d.chips {
		prev[c.name] = c
	}
	for _, c := range found {
		for _, ch := range c.channels {
			k := c.name + "/" + strconv.Itoa(ch)
			if _, ok := d.origEnable[k]; !ok {
				if v, err := os.ReadFile(filepath.Join(c.dir, fmt.Sprintf("pwm%d_enable", ch))); err == nil {
					d.origEnable[k] = strings.TrimSpace(string(v))
				}
			}
		}
		if p, ok := prev[c.name]; ok {
			c.touched, c.commanded, c.writtenAt, c.mismatch = p.touched, p.commanded, p.writtenAt, p.mismatch
		}
	}
	d.chips = found
	return nil
}

// shortName trims a hwmon name at its first underscore ("it8689_9a0a0908" →
// "it8689"); names without one are returned unchanged ("nct6799").
func shortName(name string) string {
	if i := strings.IndexByte(name, '_'); i > 0 {
		return name[:i]
	}
	return name
}

func (d *HwmonDriver) modelLocked() string {
	if len(d.chips) == 0 {
		return "unknown"
	}
	keys := make([]string, 0, len(d.chips))
	for _, c := range d.chips {
		keys = append(keys, c.key)
	}
	return strings.Join(keys, "+")
}

func (d *HwmonDriver) channelCountLocked() int {
	n := 0
	for _, c := range d.chips {
		n += len(c.channels)
	}
	return n
}

func (d *HwmonDriver) capsLocked() services.DriverCapabilities {
	n := d.channelCountLocked()
	return services.DriverCapabilities{
		SupportsManualMode:       true,
		SupportsDutyCycleReading: true,
		SupportsPerZoneControl:   true,
		PerZoneFirmwareFallback:  true,
		MaxZones:                 n,
		MaxFans:                  n,
		HasStaticRPMValues:       false,
	}
}

// buildZoneLayout exposes one zone per PWM channel across all chips. The zone
// ID encodes chip slot and hardware channel (never a slice position), so a
// profile's stored zone keeps addressing the same physical header even if the
// set of present channels changes between detections.
func (d *HwmonDriver) buildZoneLayout() services.ZoneLayout {
	zones := make([]services.ZoneDefinition, 0, d.channelCountLocked())
	idx := 0
	for _, c := range d.chips {
		for _, ch := range c.channels {
			zones = append(zones, services.ZoneDefinition{
				ID:          zoneID(c.slot, ch),
				Name:        fmt.Sprintf("%s pwm%d", c.key, ch),
				FanIndices:  []int{idx},
				Description: fmt.Sprintf("pwm%d on %s (%s)", ch, c.name, c.dir),
				IsDefault:   idx == 0,
				Chip:        c.key,
			})
			idx++
		}
	}
	return services.ZoneLayout{Zones: zones}
}

func zoneID(slot, ch int) int { return slot*zoneStride + ch }

// lookupLocked resolves a zone ID to its chip and channel.
func (d *HwmonDriver) lookupLocked(zone int) (*hwmonChip, int, bool) {
	if zone <= 0 {
		return nil, 0, false
	}
	slot, ch := zone/zoneStride, zone%zoneStride
	if slot >= len(d.chips) || ch == 0 {
		return nil, 0, false
	}
	c := d.chips[slot]
	for _, have := range c.channels {
		if have == ch {
			return c, ch, true
		}
	}
	return nil, 0, false
}

// ---- file helpers -------------------------------------------------------

func (c *hwmonChip) attr(ch int, suffix string) string {
	return filepath.Join(c.dir, fmt.Sprintf("pwm%d%s", ch, suffix))
}

func (c *hwmonChip) sensorID(ch int) string { return fmt.Sprintf("%s/fan%d", c.key, ch) }

func readInt(path string) (int, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, false // includes ENODATA, which it87 returns for H2RAM channels in auto mode
	}
	v, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return 0, false
	}
	return v, true
}

func readStr(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func writeAttr(path, val string) error {
	// hwmon attributes take a plain decimal string + newline.
	if err := os.WriteFile(path, []byte(val+"\n"), 0o644); err != nil {
		return fmt.Errorf("hwmon write %s=%s: %w", path, val, err)
	}
	return nil
}

func pctToPWM(percent int) int {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	return (percent*pwmMax + 50) / 100
}

func pwmToPct(pwm int) int {
	if pwm < 0 {
		pwm = 0
	}
	if pwm > pwmMax {
		pwm = pwmMax
	}
	return (pwm*100 + pwmMax/2) / pwmMax
}

// ---- channel control ------------------------------------------------------

// setChannel puts a channel in manual mode (only if it isn't already) and
// writes the duty, recording what was commanded for readback verification.
func (c *hwmonChip) setChannel(ch, percent int) error {
	if en := readStr(c.attr(ch, "_enable")); en != manualEnable {
		if err := writeAttr(c.attr(ch, "_enable"), manualEnable); err != nil {
			return err
		}
	}
	pwm := pctToPWM(percent)
	if err := writeAttr(c.attr(ch, ""), strconv.Itoa(pwm)); err != nil {
		return err
	}
	c.touched[ch] = true
	c.commanded[ch] = pwm
	c.writtenAt[ch] = time.Now()
	c.mismatch[ch] = 0
	return nil
}

// release hands a channel back to firmware automatic control by restoring the
// enable value captured at discovery (or the family's auto value when that was
// itself manual/full-speed or unknown).
func (c *hwmonChip) release(ch int, orig string) error {
	val := orig
	if val == "" || val == manualEnable || val == "0" {
		val = c.family.autoEnable
	}
	if err := writeAttr(c.attr(ch, "_enable"), val); err != nil {
		return err
	}
	delete(c.touched, ch)
	delete(c.commanded, ch)
	delete(c.writtenAt, ch)
	delete(c.mismatch, ch)
	return nil
}

func (d *HwmonDriver) origEnableFor(c *hwmonChip, ch int) string {
	return d.origEnable[c.name+"/"+strconv.Itoa(ch)]
}

// controlMode classifies a channel from its live enable value.
func (c *hwmonChip) controlMode(ch int) string {
	switch readStr(c.attr(ch, "_enable")) {
	case manualEnable, "0":
		return models.FanControlManual
	case "":
		return ""
	default:
		return models.FanControlFirmware
	}
}

// checkReadback compares the live pwm with what we last commanded on channels
// we own, counting consecutive disagreements (after a grace period so the
// driver's own update interval can't false-trip). Caller holds mu.
func (c *hwmonChip) checkReadback(ch int) {
	want, ok := c.commanded[ch]
	if !ok || time.Since(c.writtenAt[ch]) < overrideGrace {
		return
	}
	got, ok := readInt(c.attr(ch, ""))
	if !ok {
		return
	}
	if diff := got - want; diff > readbackTolerance || diff < -readbackTolerance {
		c.mismatch[ch]++
		if c.mismatch[ch] == overrideAfter {
			log.Warn().Str("chip", c.key).Int("channel", ch).Int("commanded", want).Int("readback", got).
				Msg("hwmon: firmware is overriding PWM writes on this channel")
		}
	} else {
		c.mismatch[ch] = 0
	}
}

// ---- IPMIDriver interface ----------------------------------------------

// CanDetect reports whether at least one controllable hwmon PWM chip is present.
func (d *HwmonDriver) CanDetect(ctx context.Context) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.channelCountLocked() > 0 {
		return true
	}
	return d.discover() == nil
}

// Discover (re)resolves the chips and refreshes the zone layout.
func (d *HwmonDriver) Discover(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.discover(); err != nil {
		return err
	}
	d.BaseDriver = services.NewBaseDriver("Hwmon", d.modelLocked(), d.capsLocked(), d.buildZoneLayout())
	for _, c := range d.chips {
		log.Info().Str("chip", c.name).Str("key", c.key).Str("family", c.family.id).Str("path", c.dir).
			Int("slot", c.slot).Ints("pwm_channels", c.channels).Msg("hwmon chip discovered")
	}
	return nil
}

// DetectFans returns one entry per PWM channel on every chip. It is the single
// source of the SensorID<->Chip/Channel<->ZoneID mapping.
func (d *HwmonDriver) DetectFans(ctx context.Context) ([]models.DetectedFan, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	fans := make([]models.DetectedFan, 0, d.channelCountLocked())
	for _, c := range d.chips {
		for _, ch := range c.channels {
			rpm, _ := readInt(filepath.Join(c.dir, fmt.Sprintf("fan%d_input", ch)))
			duty := -1
			if pwm, ok := readInt(c.attr(ch, "")); ok {
				duty = pwmToPct(pwm)
			}
			fans = append(fans, models.DetectedFan{
				SensorID:  c.sensorID(ch),
				Name:      c.sensorID(ch),
				RPM:       rpm,
				DutyCycle: duty,
				Chip:      c.key,
				Channel:   ch,
				ZoneID:    zoneID(c.slot, ch),
				Unit:      "RPM",
				Status:    "ok",
			})
		}
	}
	return fans, nil
}

// GetFanReadings returns RPM, duty and control mode per channel, keyed by the
// SensorID the driver emits everywhere (services.FanReadingProvider). Duty is
// -1 when the chip cannot report it (it87 H2RAM channels in firmware mode).
// It also runs the readback verification for channels we own.
func (d *HwmonDriver) GetFanReadings(ctx context.Context) (map[string]services.FanReading, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make(map[string]services.FanReading, d.channelCountLocked())
	for _, c := range d.chips {
		for _, ch := range c.channels {
			rpm, _ := readInt(filepath.Join(c.dir, fmt.Sprintf("fan%d_input", ch)))
			duty := -1
			if pwm, ok := readInt(c.attr(ch, "")); ok {
				duty = pwmToPct(pwm)
			}
			c.checkReadback(ch)
			out[c.sensorID(ch)] = services.FanReading{RPM: rpm, DutyCycle: duty, ControlMode: c.controlMode(ch)}
		}
	}
	return out, nil
}

// GetFanSpeeds returns sensor id -> RPM for every channel.
func (d *HwmonDriver) GetFanSpeeds(ctx context.Context) (map[string]int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make(map[string]int, d.channelCountLocked())
	for _, c := range d.chips {
		for _, ch := range c.channels {
			rpm, _ := readInt(filepath.Join(c.dir, fmt.Sprintf("fan%d_input", ch)))
			out[c.sensorID(ch)] = rpm
		}
	}
	return out, nil
}

// GetFanDutyCycles returns global fan index (0-based, DetectFans order) -> duty
// percent. Only the legacy IPMI reading path uses this; hwmon callers get duty
// from GetFanReadings.
func (d *HwmonDriver) GetFanDutyCycles(ctx context.Context) (map[int]int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make(map[int]int, d.channelCountLocked())
	idx := 0
	for _, c := range d.chips {
		for _, ch := range c.channels {
			if pwm, ok := readInt(c.attr(ch, "")); ok {
				out[idx] = pwmToPct(pwm)
			}
			idx++
		}
	}
	return out, nil
}

// SetFanSpeed sets one zone (chip slot + PWM channel) to percent. zone<0 sets all.
func (d *HwmonDriver) SetFanSpeed(ctx context.Context, zone int, percent int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if zone < 0 {
		return d.setAllLocked(percent)
	}
	c, ch, ok := d.lookupLocked(zone)
	if !ok {
		return fmt.Errorf("hwmon: zone %d is not a present PWM channel", zone)
	}
	if err := c.setChannel(ch, percent); err != nil {
		return err
	}
	d.manualMode = true
	log.Debug().Str("chip", c.key).Int("channel", ch).Int("zone", zone).Int("percent", percent).Msg("hwmon set fan speed")
	return nil
}

// SetAllFanSpeeds sets every channel on every chip to percent (emergency,
// safety-on-shutdown and explicit startup modes).
func (d *HwmonDriver) SetAllFanSpeeds(ctx context.Context, percent int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.setAllLocked(percent)
}

func (d *HwmonDriver) setAllLocked(percent int) error {
	var firstErr error
	for _, c := range d.chips {
		for _, ch := range c.channels {
			if err := c.setChannel(ch, percent); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	if firstErr == nil {
		d.manualMode = true
	}
	return firstErr
}

// SetManualMode opens (enabled) or closes the control session. Opening does
// not touch any channel — each is switched to manual lazily on its first
// write, so untargeted channels keep their firmware curve. Closing restores
// firmware automatic control on every channel this driver touched.
func (d *HwmonDriver) SetManualMode(ctx context.Context, enabled bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if enabled {
		d.manualMode = true
		log.Info().Msg("hwmon control session opened (channels switch to manual on first write)")
		return nil
	}
	var firstErr error
	released := 0
	for _, c := range d.chips {
		for _, ch := range c.channels {
			if !c.touched[ch] {
				continue
			}
			if err := c.release(ch, d.origEnableFor(c, ch)); err != nil && firstErr == nil {
				firstErr = err
			} else {
				released++
			}
		}
	}
	if firstErr == nil {
		d.manualMode = false
		log.Info().Int("channels_released", released).Msg("hwmon control session closed; firmware automatic control restored")
	}
	return firstErr
}

// IsManualMode reports whether a control session is open.
func (d *HwmonDriver) IsManualMode() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.manualMode
}

// ---- optional interfaces --------------------------------------------------

// ReleaseZone returns one zone to firmware automatic control (services.ZoneReleaser).
func (d *HwmonDriver) ReleaseZone(ctx context.Context, zone int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	c, ch, ok := d.lookupLocked(zone)
	if !ok {
		return fmt.Errorf("hwmon: zone %d is not a present PWM channel", zone)
	}
	if err := c.release(ch, d.origEnableFor(c, ch)); err != nil {
		return err
	}
	log.Info().Str("chip", c.key).Int("channel", ch).Msg("hwmon zone released to firmware control")
	return nil
}

// IdentifyZone spins one zone at 100% for duration, then puts it back exactly
// as it was: the previously commanded duty if this driver owned the channel,
// otherwise firmware automatic control (services.ZoneIdentifier).
func (d *HwmonDriver) IdentifyZone(ctx context.Context, zone int, duration time.Duration) error {
	d.mu.Lock()
	c, ch, ok := d.lookupLocked(zone)
	if !ok {
		d.mu.Unlock()
		return fmt.Errorf("hwmon: zone %d is not a present PWM channel", zone)
	}
	prevPWM, owned := c.commanded[ch]
	if err := c.setChannel(ch, 100); err != nil {
		d.mu.Unlock()
		return err
	}
	d.mu.Unlock()

	select {
	case <-ctx.Done():
		// fall through to restore
	case <-time.After(duration):
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if owned {
		return c.setChannel(ch, pwmToPct(prevPWM))
	}
	return c.release(ch, d.origEnableFor(c, ch))
}

// DriverWarnings lists channels whose firmware is overriding our PWM writes
// (services.HealthReporter). Empty when everything we command sticks.
func (d *HwmonDriver) DriverWarnings() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []string
	for _, c := range d.chips {
		for _, ch := range c.channels {
			if c.mismatch[ch] >= overrideAfter {
				want := c.commanded[ch]
				got, _ := readInt(c.attr(ch, ""))
				out = append(out, fmt.Sprintf("firmware is overriding fan writes on %s pwm%d (commanded %d%%, reads %d%%)",
					c.key, ch, pwmToPct(want), pwmToPct(got)))
			}
		}
	}
	return out
}
