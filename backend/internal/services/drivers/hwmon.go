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

	"docker-fan-control/internal/models"
	"docker-fan-control/internal/services"

	"github.com/rs/zerolog/log"
)

// HwmonDriver controls fans directly via the Linux hwmon sysfs interface
// (e.g. a Nuvoton Super-I/O chip driven by the in-tree nct6775 module). It is
// the non-IPMI path used on consumer/workstation boards that have no BMC, such
// as the ASUS ProArt X870E-CREATOR (hwmon name "nct6799").
//
// Unlike the vendor IPMI drivers it does NOT use an IPMIExecutor; it reads
// fanN_input / writes pwmN + pwmN_enable under a single hwmon directory. PWM is
// 0-255; manual control is pwmN_enable=1, and the firmware's automatic mode is
// restored on cleanup by writing back the enable value seen at discovery.
type HwmonDriver struct {
	*services.BaseDriver

	mu         sync.Mutex
	preferName string         // preferred hwmon "name" (e.g. "nct6799"); "" = any nct6xxx
	path       string         // resolved /sys/class/hwmon/hwmonN directory
	chipName   string         // hwmon "name" of the resolved chip
	channels   []int          // pwm channel numbers present (1-based), sorted
	origEnable map[int]string // channel -> pwmN_enable value seen at discovery
	manualMode bool
}

// pwmMax is the full-scale PWM duty value exposed by hwmon (0-255).
const pwmMax = 255

// manualEnable is the pwmN_enable value for direct (manual) PWM control in the
// nct6775 driver. The firmware/auto value (commonly 5 = SmartFan IV) is captured
// per channel at discovery and written back to restore automatic control.
const manualEnable = "1"

// NewHwmonDriver creates a hwmon driver. preferName optionally pins the hwmon
// chip "name" to bind (e.g. "nct6799"); empty auto-selects the first nct6xxx
// chip that exposes PWM channels. Discovery is best-effort here and re-run by
// Discover(); CanDetect() reports whether a usable chip was found.
func NewHwmonDriver(preferName string) *HwmonDriver {
	d := &HwmonDriver{
		preferName: strings.TrimSpace(preferName),
		origEnable: make(map[int]string),
	}

	// Best-effort discovery so the zone layout reflects real channels. main.go
	// constructs this after /sys is mounted; DetectBestDriver calls Discover too.
	_ = d.discover()

	model := d.chipName
	if model == "" {
		model = "unknown"
	}

	caps := services.DriverCapabilities{
		SupportsManualMode:       true,
		SupportsDutyCycleReading: true,
		SupportsPerZoneControl:   true,
		MaxZones:                 len(d.channels),
		MaxFans:                  len(d.channels),
		HasStaticRPMValues:       false,
	}

	d.BaseDriver = services.NewBaseDriver("Hwmon", model, caps, d.buildZoneLayout())
	return d
}

// buildZoneLayout exposes one zone per PWM channel (zone ID == fan index ==
// position in d.channels), so profiles can target each header independently.
func (d *HwmonDriver) buildZoneLayout() services.ZoneLayout {
	zones := make([]services.ZoneDefinition, 0, len(d.channels))
	for i, ch := range d.channels {
		zones = append(zones, services.ZoneDefinition{
			ID:          i,
			Name:        fmt.Sprintf("Fan %d", ch),
			FanIndices:  []int{i},
			Description: fmt.Sprintf("pwm%d on %s", ch, d.chipName),
			IsDefault:   i == 0,
		})
	}
	return services.ZoneLayout{Zones: zones}
}

// discover resolves the hwmon directory and enumerates PWM channels. Safe to
// call repeatedly. Returns an error if no usable chip is found.
func (d *HwmonDriver) discover() error {
	namePaths, _ := filepath.Glob("/sys/class/hwmon/hwmon*/name")

	type cand struct {
		dir  string
		name string
	}
	var cands []cand
	for _, np := range namePaths {
		b, err := os.ReadFile(np)
		if err != nil {
			continue
		}
		name := strings.TrimSpace(string(b))
		dir := filepath.Dir(np)
		// Must expose at least pwm1 to be controllable.
		if _, err := os.Stat(filepath.Join(dir, "pwm1")); err != nil {
			continue
		}
		if d.preferName != "" {
			if name == d.preferName {
				cands = append(cands, cand{dir, name})
			}
			continue
		}
		// Auto mode: accept Nuvoton Super-I/O chips exposed by nct6775.
		if strings.HasPrefix(name, "nct6") {
			cands = append(cands, cand{dir, name})
		}
	}

	if len(cands) == 0 {
		return fmt.Errorf("hwmon: no controllable chip found (preferName=%q)", d.preferName)
	}
	// Deterministic pick: lowest hwmon index.
	sort.Slice(cands, func(i, j int) bool { return cands[i].dir < cands[j].dir })
	chosen := cands[0]

	// Enumerate pwmN channels (1..8).
	var channels []int
	enables := make(map[int]string)
	for n := 1; n <= 8; n++ {
		if _, err := os.Stat(filepath.Join(chosen.dir, fmt.Sprintf("pwm%d", n))); err != nil {
			continue
		}
		channels = append(channels, n)
		if v, err := os.ReadFile(filepath.Join(chosen.dir, fmt.Sprintf("pwm%d_enable", n))); err == nil {
			enables[n] = strings.TrimSpace(string(v))
		}
	}
	if len(channels) == 0 {
		return fmt.Errorf("hwmon: chip %q at %s exposes no pwm channels", chosen.name, chosen.dir)
	}

	d.path = chosen.dir
	d.chipName = chosen.name
	d.channels = channels
	// Preserve original enables only the first time (don't overwrite with values
	// we may have already changed to manual on a re-discover).
	for n, v := range enables {
		if _, seen := d.origEnable[n]; !seen {
			d.origEnable[n] = v
		}
	}
	return nil
}

// ---- file helpers -------------------------------------------------------

func (d *HwmonDriver) attr(ch int, suffix string) string {
	return filepath.Join(d.path, fmt.Sprintf("pwm%d%s", ch, suffix))
}

func (d *HwmonDriver) readInt(path string) (int, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	v, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return 0, false
	}
	return v, true
}

func (d *HwmonDriver) writeAttr(path, val string) error {
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

// ---- IPMIDriver interface ----------------------------------------------

// CanDetect reports whether a usable hwmon PWM chip is present.
func (d *HwmonDriver) CanDetect(ctx context.Context) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.path != "" && len(d.channels) > 0 {
		return true
	}
	return d.discover() == nil
}

// Discover (re)resolves the chip and refreshes the zone layout.
func (d *HwmonDriver) Discover(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.discover(); err != nil {
		return err
	}
	// Refresh metadata now that channels are known.
	caps := d.GetCapabilities()
	caps.MaxZones = len(d.channels)
	caps.MaxFans = len(d.channels)
	d.BaseDriver = services.NewBaseDriver("Hwmon", d.chipName, caps, d.buildZoneLayout())
	log.Info().Str("chip", d.chipName).Str("path", d.path).Ints("pwm_channels", d.channels).Msg("hwmon driver discovered")
	return nil
}

// DetectFans returns one entry per PWM channel, with live tach RPM.
func (d *HwmonDriver) DetectFans(ctx context.Context) ([]models.DetectedFan, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	fans := make([]models.DetectedFan, 0, len(d.channels))
	for _, ch := range d.channels {
		rpm, _ := d.readInt(filepath.Join(d.path, fmt.Sprintf("fan%d_input", ch)))
		fans = append(fans, models.DetectedFan{
			SensorID: fmt.Sprintf("fan%d", ch),
			Name:     fmt.Sprintf("fan%d", ch),
			RPM:      rpm,
			Unit:     "RPM",
			Status:   "ok",
		})
	}
	return fans, nil
}

// GetFanSpeeds returns sensor name -> RPM for every channel.
func (d *HwmonDriver) GetFanSpeeds(ctx context.Context) (map[string]int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make(map[string]int, len(d.channels))
	for _, ch := range d.channels {
		rpm, _ := d.readInt(filepath.Join(d.path, fmt.Sprintf("fan%d_input", ch)))
		out[fmt.Sprintf("fan%d", ch)] = rpm
	}
	return out, nil
}

// GetFanDutyCycles returns fan index (0-based, == zone ID) -> duty percent.
func (d *HwmonDriver) GetFanDutyCycles(ctx context.Context) (map[int]int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make(map[int]int, len(d.channels))
	for i, ch := range d.channels {
		if pwm, ok := d.readInt(d.attr(ch, "")); ok {
			out[i] = pwmToPct(pwm)
		}
	}
	return out, nil
}

// setChannel ensures manual mode on a channel and writes the duty.
func (d *HwmonDriver) setChannel(ch, percent int) error {
	if err := d.writeAttr(d.attr(ch, "_enable"), manualEnable); err != nil {
		return err
	}
	return d.writeAttr(d.attr(ch, ""), strconv.Itoa(pctToPWM(percent)))
}

// SetFanSpeed sets one zone (== fan index) to percent. zone<0 sets all.
func (d *HwmonDriver) SetFanSpeed(ctx context.Context, zone int, percent int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if zone < 0 {
		return d.setAllLocked(percent)
	}
	if zone >= len(d.channels) {
		return fmt.Errorf("hwmon: zone %d out of range (have %d channels)", zone, len(d.channels))
	}
	ch := d.channels[zone]
	if err := d.setChannel(ch, percent); err != nil {
		return err
	}
	d.manualMode = true
	log.Debug().Int("zone", zone).Int("pwm_channel", ch).Int("percent", percent).Msg("hwmon set fan speed")
	return nil
}

// SetAllFanSpeeds sets every channel to percent.
func (d *HwmonDriver) SetAllFanSpeeds(ctx context.Context, percent int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.setAllLocked(percent)
}

func (d *HwmonDriver) setAllLocked(percent int) error {
	var firstErr error
	for _, ch := range d.channels {
		if err := d.setChannel(ch, percent); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if firstErr == nil {
		d.manualMode = true
	}
	return firstErr
}

// SetManualMode switches all channels to manual PWM (enabled) or restores the
// firmware's automatic mode captured at discovery (disabled).
func (d *HwmonDriver) SetManualMode(ctx context.Context, enabled bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	var firstErr error
	for _, ch := range d.channels {
		val := manualEnable
		if !enabled {
			// Restore the auto value we saw at discovery; fall back to "5"
			// (SmartFan IV), the common nct6775 automatic mode.
			val = d.origEnable[ch]
			if val == "" || val == manualEnable {
				val = "5"
			}
		}
		if err := d.writeAttr(d.attr(ch, "_enable"), val); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if firstErr == nil {
		d.manualMode = enabled
		log.Info().Bool("enabled", enabled).Msg("hwmon set manual mode")
	}
	return firstErr
}

// IsManualMode reports the last manual-mode state set by this driver.
func (d *HwmonDriver) IsManualMode() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.manualMode
}
