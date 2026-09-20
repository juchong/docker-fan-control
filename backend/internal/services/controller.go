package services

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"docker-fan-control/internal/algorithms"
	"docker-fan-control/internal/database"
	"docker-fan-control/internal/models"

	"github.com/rs/zerolog/log"
)

// manualOverride is a per-fan manual speed. A zero expiresAt means sticky until
// explicitly cleared via ClearManualOverride.
type manualOverride struct {
	percent   int
	expiresAt time.Time
}

// FanController manages the main fan control loop
type FanController struct {
	ipmi      *IPMIService
	gpu       *GPUService
	system    *SystemService
	logger    *EventLogger
	interval  time.Duration
	running   atomic.Bool
	stopCh    chan struct{}
	mu        sync.RWMutex

	// Current state
	manualOverrides map[uint]manualOverride // fan ID -> override
	staleZoneWarned map[int]bool            // zones already reported as missing from the driver layout
	lastSpeeds      map[int]int  // zone -> percent
	zoneLastChanged map[int]time.Time // zone -> last change time
	zoneTargetSpeeds map[int]int  // zone -> target speed (for smoothing)
	emergencyTemp   float64
	emergencySpeed  int
	warningTemp    float64
	warningEnabled bool

	// Persistent per-profile algorithm state (so PID integral/derivative survive
	// across cycles). Touched only from the control goroutine.
	algoInstances    map[uint]algorithms.Algorithm
	algoSig          map[uint]string   // profileID -> algo type + params signature
	profileLastInput map[uint]float64  // profileID -> last aggregated input (hysteresis)

	// Startup + shutdown behavior
	startupMode      string
	startupPercent   int
	safetyOnShutdown bool

	// Sensor-loss tracking: consecutive cycles where a temp-driven profile is
	// active but no temperature could be read. everSawTemp arms the check so a
	// boot with not-yet-ready sensors doesn't false-trip.
	noTempCycles int
	everSawTemp  bool

	lastWarnLogged time.Time // throttle for repeated temperature-warning events

	// Thermal evaluation against per-device limits (see thermal.go).
	// thermalCfg comes from settings; thermal is the last cycle's result (read
	// by GetState under mu); the rest is control-goroutine state.
	thermalCfg      ThermalConfig
	thermal         *models.ThermalState
	emergencyActive bool
	emergencySince  time.Time
	deviceStatus    map[string]string    // per-device last status (events on change)
	deviceLastLog   map[string]time.Time // per-device last event time (periodic reminder)
	limitsLogged    bool                 // effective limits logged once per config
	lastGPUs        []models.GPUMetrics  // metrics gathered this cycle (control goroutine only)
	lastSystem      *models.SystemMetrics
}

// NewFanController creates a new fan controller
func NewFanController(ipmi *IPMIService, gpu *GPUService, system *SystemService, logger *EventLogger) *FanController {
	return &FanController{
		ipmi:            ipmi,
		gpu:             gpu,
		system:          system,
		logger:          logger,
		interval:        5 * time.Second,
		stopCh:          make(chan struct{}),
		manualOverrides: make(map[uint]manualOverride),
		lastSpeeds:      make(map[int]int),
		zoneLastChanged: make(map[int]time.Time),
		zoneTargetSpeeds: make(map[int]int),
		algoInstances:    make(map[uint]algorithms.Algorithm),
		algoSig:          make(map[uint]string),
		profileLastInput: make(map[uint]float64),
		emergencyTemp:   90,
		emergencySpeed:  100,
		warningTemp:     70,
		warningEnabled:  true,
		startupMode:     "resume",
		startupPercent:  50,
		safetyOnShutdown: true,
	}
}

// Start launches the fan control loop
func (c *FanController) Start(ctx context.Context) error {
	if c.running.Load() {
		return nil
	}

	// Load settings
	c.loadSettings()

	// Enable manual fan control mode
	if err := c.ipmi.SetManualMode(ctx, true); err != nil {
		log.Warn().Err(err).Msg("Failed to enable manual fan mode, continuing anyway")
	}

	c.running.Store(true)
	c.stopCh = make(chan struct{})

	// Establish a known-safe speed immediately so fans never idle while firmware
	// auto-control is disabled and before a profile has taken over, then run one
	// cycle now so a configured profile converges at t=0 instead of after a full
	// interval.
	c.applyStartupSpeed(ctx)
	c.controlCycle(ctx)

	go c.controlLoop(ctx)

	c.logger.LogSystemEvent("Fan control service started", models.JSONMap{
		"interval":     c.interval.String(),
		"startup_mode": c.startupMode,
	})

	return nil
}

// applyStartupSpeed sets an initial safe fan speed based on the configured
// startup mode. safeFloor prevents commanding a stall-inducing low speed.
func (c *FanController) applyStartupSpeed(ctx context.Context) {
	const safeFloor = 30
	switch c.startupMode {
	case "full":
		_ = c.ipmi.SetAllFanSpeeds(ctx, 100)
	case "percent":
		pct := c.startupPercent
		if pct < safeFloor {
			pct = safeFloor
		}
		if pct > 100 {
			pct = 100
		}
		_ = c.ipmi.SetAllFanSpeeds(ctx, pct)
	default: // "resume"
		// If a profile is active the immediate control cycle sets speeds;
		// otherwise apply a safe floor so fans don't idle at an unknown duty.
		var count int64
		database.DB.Model(&models.Profile{}).Where("is_active = ?", true).Count(&count)
		if count == 0 {
			if c.driverKeepsFirmwareFallback() {
				log.Info().Msg("No active profile; fans stay under firmware automatic control (driver keeps untargeted zones on the firmware curve)")
				return
			}
			_ = c.ipmi.SetAllFanSpeeds(ctx, safeFloor)
		}
	}
}

// presentZones returns the set of zone IDs the active driver exposes, or nil
// when there is no driver/layout to filter against.
func (c *FanController) presentZones() map[int]bool {
	d := c.ipmi.GetCurrentDriver()
	if d == nil {
		return nil
	}
	layout := d.GetZoneLayout()
	if len(layout.Zones) == 0 {
		return nil
	}
	set := make(map[int]bool, len(layout.Zones))
	for _, z := range layout.Zones {
		set[z.ID] = true
	}
	return set
}

// warnStaleZone reports, once per zone, that a profile targets a zone the
// active driver does not expose.
func (c *FanController) warnStaleZone(zone int, profileID uint) {
	c.mu.Lock()
	if c.staleZoneWarned == nil {
		c.staleZoneWarned = make(map[int]bool)
	}
	warned := c.staleZoneWarned[zone]
	c.staleZoneWarned[zone] = true
	c.mu.Unlock()
	if warned {
		return
	}
	log.Warn().Int("zone", zone).Uint("profile_id", profileID).
		Msg("Profile targets a zone the active driver does not expose; skipping it — edit the profile's target zones")
	c.logger.LogSystemEvent("Profile targets a missing zone", models.JSONMap{
		"zone":       zone,
		"profile_id": profileID,
	})
}

// driverKeepsFirmwareFallback reports whether zones the controller never
// commands remain under firmware automatic control (hwmon). Such drivers need
// no all-fans startup floor: an untargeted fan is never left unmanaged.
func (c *FanController) driverKeepsFirmwareFallback() bool {
	d := c.ipmi.GetCurrentDriver()
	return d != nil && d.GetCapabilities().PerZoneFirmwareFallback
}

// Stop gracefully stops the fan control loop, honoring the SafetyOnShutdown
// setting (100% latch vs. restore automatic/firmware control).
func (c *FanController) Stop() {
	c.StopWithSafety(c.safetyOnShutdown)
}

// StopWithSafety stops the controller with optional safety mode (fans to 100%)
func (c *FanController) StopWithSafety(setSafeSpeed bool) {
	if !c.running.Load() {
		return
	}

	c.running.Store(false)
	close(c.stopCh)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Safety: Set fans to 100% before stopping
	if setSafeSpeed {
		log.Info().Msg("Setting fans to 100% for safety on shutdown")
		// Ensure manual mode is enabled so our speed setting is respected
		if err := c.ipmi.SetManualMode(ctx, true); err != nil {
			log.Warn().Err(err).Msg("Failed to ensure manual mode for safety shutdown")
		}
		if err := c.ipmi.SetAllFanSpeeds(ctx, 100); err != nil {
			log.Error().Err(err).Msg("Failed to set safe fan speed on shutdown")
			c.logger.LogIPMIError("safety shutdown fan speed", err)
		} else {
			c.logger.LogSystemEvent("Safety: fans set to 100% on shutdown", nil)
		}
		// Leave manual mode enabled so fans stay at 100%
	} else {
		// Only restore automatic fan control if safety mode is disabled
		if err := c.ipmi.SetManualMode(ctx, false); err != nil {
			log.Warn().Err(err).Msg("Failed to restore automatic fan control")
		}
	}

	c.logger.LogSystemEvent("Fan control service stopped", nil)
}

// IsRunning returns whether the controller is running
func (c *FanController) IsRunning() bool {
	return c.running.Load()
}

// SetInterval updates the control loop interval
func (c *FanController) SetInterval(interval time.Duration) {
	c.mu.Lock()
	c.interval = interval
	c.mu.Unlock()
}

// ActivateProfile activates a specific profile (allows multiple active)
func (c *FanController) ActivateProfile(profileID uint) {
	database.DB.Model(&models.Profile{}).Where("id = ?", profileID).Update("is_active", true)
}

// DeactivateProfile deactivates a specific profile
func (c *FanController) DeactivateProfile(profileID uint) {
	database.DB.Model(&models.Profile{}).Where("id = ?", profileID).Update("is_active", false)
}

// SetManualOverride sets a manual speed override for a fan. duration<=0 makes it
// sticky (cleared only via ClearManualOverride or a new override).
func (c *FanController) SetManualOverride(fanID uint, percent int, duration time.Duration) {
	c.mu.Lock()
	ov := manualOverride{percent: percent}
	if duration > 0 {
		ov.expiresAt = time.Now().Add(duration)
	}
	c.manualOverrides[fanID] = ov
	c.mu.Unlock()
}

// ClearManualOverride removes a manual override for a fan. The zone's command
// history is forgotten so the next control cycle re-sends the profile target
// (the "only send on change" check would otherwise leave the override value in
// place), and if no active profile targets the zone it is handed back to
// firmware automatic control on drivers that support that.
func (c *FanController) ClearManualOverride(ctx context.Context, fanID uint, zone *int) {
	c.mu.Lock()
	delete(c.manualOverrides, fanID)
	if zone != nil {
		delete(c.lastSpeeds, *zone)
		delete(c.zoneLastChanged, *zone)
		delete(c.zoneTargetSpeeds, *zone)
	}
	c.mu.Unlock()

	if zone == nil || c.zoneTargeted(*zone) {
		return
	}
	if err := c.ipmi.ReleaseZone(ctx, *zone); err != nil {
		log.Warn().Err(err).Int("zone", *zone).Msg("Failed to release zone to firmware control")
	}
}

// zoneTargeted reports whether any active profile targets the zone. Errors
// read as "targeted" so a DB hiccup never releases a fan a profile controls.
func (c *FanController) zoneTargeted(zone int) bool {
	var profiles []models.Profile
	if err := database.DB.Where("is_active = ?", true).Find(&profiles).Error; err != nil {
		return true
	}
	for _, p := range profiles {
		for _, z := range p.Zones {
			if z == zone {
				return true
			}
		}
	}
	return false
}

// HasManualOverride reports whether a fan has an active (non-expired) override,
// lazily removing an expired one.
func (c *FanController) HasManualOverride(fanID uint) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	ov, ok := c.manualOverrides[fanID]
	if !ok {
		return false
	}
	if !ov.expiresAt.IsZero() && time.Now().After(ov.expiresAt) {
		delete(c.manualOverrides, fanID)
		return false
	}
	return true
}

// GetZoneTarget returns the last commanded speed for a zone, if one exists.
func (c *FanController) GetZoneTarget(zone int) (int, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.lastSpeeds[zone]
	return v, ok
}

// loadSettings loads settings from database
func (c *FanController) loadSettings() {
	settings, err := database.GetAllSettings()
	if err != nil {
		log.Warn().Err(err).Msg("Failed to load settings")
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.interval = time.Duration(settings.ControlInterval) * time.Second
	c.emergencyTemp = float64(settings.EmergencyTemp)
	c.emergencySpeed = settings.EmergencySpeed
	c.warningTemp = float64(settings.WarningTemp)
	c.warningEnabled = settings.WarningEnabled
	c.thermalCfg = thermalConfigFrom(settings)
	c.limitsLogged = false
	log.Info().Str("mode", c.thermalCfg.Mode).Int("warning_margin", c.thermalCfg.WarningMargin).
		Float64("legacy_warning", c.thermalCfg.LegacyWarning).Float64("emergency_temp", c.thermalCfg.EmergencyTemp).
		Msg("Thermal limits configured")
	if settings.StartupMode != "" {
		c.startupMode = settings.StartupMode
	}
	if settings.StartupPercent > 0 {
		c.startupPercent = settings.StartupPercent
	}
	c.safetyOnShutdown = settings.SafetyOnShutdown
}

// controlLoop runs the main control loop
func (c *FanController) controlLoop(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.controlCycle(ctx)
			
			// Update ticker if interval changed
			c.mu.RLock()
			newInterval := c.interval
			c.mu.RUnlock()
			ticker.Reset(newInterval)
		}
	}
}

// getAlgorithm returns a persistent algorithm instance for a profile, recreating
// it only when the algorithm type or its params change (which resets stateful
// controllers like PID). Called only from the control goroutine, so the maps
// need no locking.
func (c *FanController) getAlgorithm(p *models.Profile) algorithms.Algorithm {
	sigBytes, _ := json.Marshal(p.AlgorithmParams) // Go marshals map keys sorted → deterministic
	sig := p.Algorithm + "|" + string(sigBytes)
	if c.algoInstances[p.ID] == nil || c.algoSig[p.ID] != sig {
		c.algoInstances[p.ID] = algorithms.NewAlgorithm(p.Algorithm, p.AlgorithmParams)
		c.algoSig[p.ID] = sig
	}
	return c.algoInstances[p.ID]
}

// pruneAlgoState drops per-profile state for profiles that are no longer active.
// Control-goroutine only.
func (c *FanController) pruneAlgoState(active map[uint]bool) {
	for id := range c.algoInstances {
		if !active[id] {
			delete(c.algoInstances, id)
			delete(c.algoSig, id)
			delete(c.profileLastInput, id)
		}
	}
}

// isTempInput reports whether an input type is a temperature source.
func isTempInput(t string) bool {
	switch t {
	case models.InputTypeGPUTemp, models.InputTypeCPUTemp, models.InputTypeDriveTemp, models.InputTypeBoardTemp:
		return true
	}
	return false
}

// anyProfileUsesTemp reports whether any active profile declares a temperature
// input (used to decide whether losing all sensors is an emergency).
func anyProfileUsesTemp(profiles []models.Profile) bool {
	for i := range profiles {
		for _, in := range profiles[i].Inputs {
			if isTempInput(in.InputType) {
				return true
			}
		}
	}
	return false
}

func absFloat(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// controlCycle performs one control cycle
func (c *FanController) controlCycle(ctx context.Context) {
	// Gather inputs
	inputs := c.gatherInputs()

	_, tempCount := c.getMaxTemperature(inputs)
	if tempCount > 0 {
		c.everSawTemp = true
	}

	// Load ALL active profiles (supports multiple simultaneous profiles)
	var profiles []models.Profile
	if err := database.DB.Preload("Inputs").Where("is_active = ?", true).Find(&profiles).Error; err != nil {
		log.Error().Err(err).Msg("Failed to load active profiles")
		return
	}

	// Prune per-profile algorithm state for profiles no longer active.
	activeIDs := make(map[uint]bool, len(profiles))
	for i := range profiles {
		activeIDs[profiles[i].ID] = true
	}
	c.pruneAlgoState(activeIDs)

	// Thermal evaluation: per-device warnings against each device's own limit
	// (monitoring only), plus the emergency rule, which is unchanged: any
	// GPU/CPU/drive reading at or above emergency_temp forces all zones to the
	// emergency speed for as long as it holds.
	cfg := c.thermalCfgSnapshot()
	ev := EvaluateThermal(cfg, c.lastGPUs, c.lastSystem)
	c.logEffectiveLimits(ev)
	c.logThermalTransitions(cfg, ev)
	if ev.State.Status == models.ThermalCritical {
		if !c.emergencyActive {
			c.emergencyActive = true
			c.emergencySince = time.Now()
		}
		c.noTempCycles = 0
		c.setAllFans(ctx, c.emergencySpeed)
		c.publishThermal(ev.State, true)
		return
	}
	if c.emergencyActive {
		c.emergencyActive = false
		held := time.Since(c.emergencySince).Round(time.Second)
		log.Info().Dur("held", held).Msg("Thermal emergency cleared; profiles resume control")
		c.logger.LogSystemEvent("Thermal emergency cleared", models.JSONMap{"held_for": held.String()})
	}
	c.publishThermal(ev.State, false)

	// Sensor loss: a temp-driven profile is active but no temperature could be
	// read. Armed only after valid temps have been seen at least once (so a boot
	// with not-yet-ready sensors, or a deliberately load-only profile, does not
	// false-trip), and only after a short grace window.
	if tempCount == 0 && c.everSawTemp && anyProfileUsesTemp(profiles) {
		c.noTempCycles++
		if c.noTempCycles >= 3 {
			log.Error().Int("cycles", c.noTempCycles).Msg("Temperature sensors lost; forcing emergency fan speed")
			c.logger.LogSystemEvent("Temperature sensor loss - emergency fan speed", models.JSONMap{"emergency_speed": c.emergencySpeed})
			c.setAllFans(ctx, c.emergencySpeed)
			return
		}
	} else {
		c.noTempCycles = 0
	}


	if len(profiles) == 0 {
		return // No active profiles
	}

	// Sort profiles by priority (highest first)
	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].Priority > profiles[j].Priority
	})

	// Index profiles by ID for per-zone tuning lookups.
	profileByID := make(map[uint]*models.Profile, len(profiles))
	for i := range profiles {
		profileByID[profiles[i].ID] = &profiles[i]
	}

	// Collect zone targets from all profiles
	// Higher priority profiles win, then highest speed wins for same priority
	zoneTargets := make(map[int]int)
	zoneControllingProfile := make(map[int]uint) // Track which profile controls each zone
	zoneControllingPriority := make(map[int]int)  // Track priority of controlling profile
	zoneConflictCount := make(map[int]int)        // Track conflict count per zone
	hasAnyZones := false

	for _, profile := range profiles {
		// Calculate aggregated input value for this profile
		inputValue := c.calculateInputValue(&profile, inputs)

		// Hysteresis: for non-PID algorithms, ignore small input changes to avoid
		// threshold chatter (PID self-damps, so it always sees the raw value).
		if profile.Algorithm != "pid" && profile.Hysteresis > 0 {
			if last, ok := c.profileLastInput[profile.ID]; ok && absFloat(inputValue-last) < profile.Hysteresis {
				inputValue = last
			}
		}
		c.profileLastInput[profile.ID] = inputValue

		// Persistent algorithm instance (preserves PID integral/derivative state).
		algo := c.getAlgorithm(&profile)
		targetSpeed := algo.Calculate(inputValue)

		// Apply to zones from Zones field (new way - takes priority)
		if len(profile.Zones) > 0 {
			hasAnyZones = true
			for _, zone := range profile.Zones {
			// Check if this zone already has a controlling profile with higher priority
			if existingPriority, ok := zoneControllingPriority[zone]; ok {
				// Only update if this profile has higher priority
				if profile.Priority > existingPriority {
					zoneTargets[zone] = targetSpeed
					zoneControllingProfile[zone] = profile.ID
					zoneControllingPriority[zone] = profile.Priority
					log.Info().Int("zone", zone).Uint("profile_id", profile.ID).Str("profile_name", profile.Name).Int("priority", profile.Priority).Int("old_priority", existingPriority).Msg("Profile conflict resolved: higher priority profile took control")
					zoneConflictCount[zone]++
				} else if profile.Priority == existingPriority && targetSpeed > zoneTargets[zone] {
					// If same priority, use highest speed
					zoneTargets[zone] = targetSpeed
					log.Info().Int("zone", zone).Uint("profile_id", profile.ID).Str("profile_name", profile.Name).Int("priority", profile.Priority).Int("speed", targetSpeed).Msg("Profile conflict resolved: same priority, higher speed selected")
					zoneConflictCount[zone]++
				} else {
					log.Warn().Int("zone", zone).Uint("profile_id", profile.ID).Str("profile_name", profile.Name).Int("priority", profile.Priority).Int("existing_priority", existingPriority).Msg("Profile conflict: lower priority profile skipped")
					zoneConflictCount[zone]++
				}
				} else {
					// No existing profile, claim this zone
					zoneTargets[zone] = targetSpeed
					zoneControllingProfile[zone] = profile.ID
					zoneControllingPriority[zone] = profile.Priority
					log.Debug().Int("zone", zone).Uint("profile_id", profile.ID).Int("priority", profile.Priority).Msg("Profile claimed zone")
				}
			}
			// Continue to next profile - zones are handled above
			continue
		}
		
		// If no zones configured, skip this profile
		continue
		}

	// If no zones assigned, skip this cycle (require zones to be configured)
	if !hasAnyZones {
		log.Warn().Msg("No profiles with zone assignments found")
		c.logger.LogSystemEvent("No profiles with zone assignments", models.JSONMap{
			"active_profiles": len(profiles),
		})
		return
	}

	// Apply zone targets, honoring each controlling profile's min-run-time,
	// transition time and smoothing settings (previously hardcoded 30s/10s).
	// Zones the active driver doesn't expose (a profile saved on a previous
	// board, say) are skipped with one warning per zone — retrying the write
	// every cycle would only flood the event log.
	present := c.presentZones()
	for zone, targetSpeed := range zoneTargets {
		if present != nil && !present[zone] {
			c.warnStaleZone(zone, zoneControllingProfile[zone])
			continue
		}
		prof := profileByID[zoneControllingProfile[zone]]
		minRun := 30 * time.Second
		transitionTime := 10 * time.Second
		smooth := true
		if prof != nil {
			if prof.MinRunTime > 0 {
				minRun = time.Duration(prof.MinRunTime) * time.Second
			}
			if prof.TransitionTime > 0 {
				transitionTime = time.Duration(prof.TransitionTime) * time.Second
			}
			smooth = prof.SmoothTransition
		}

		c.mu.RLock()
		lastSpeed := c.lastSpeeds[zone]
		lastChanged := c.zoneLastChanged[zone]
		currentTarget := c.zoneTargetSpeeds[zone]
		c.mu.RUnlock()

		// Minimum run time: the speed is already applied in manual mode, so just
		// hold it (no need to re-issue the command every cycle).
		if !lastChanged.IsZero() && time.Since(lastChanged) < minRun {
			continue
		}

		// Smoothing (per profile). Skip on first application.
		finalSpeed := targetSpeed
		if smooth && currentTarget != targetSpeed && !lastChanged.IsZero() {
			elapsed := time.Since(lastChanged)
			progress := float64(elapsed) / float64(transitionTime)
			if progress > 1.0 {
				progress = 1.0
			}
			finalSpeed = int(float64(lastSpeed) + (float64(targetSpeed)-float64(lastSpeed))*progress)
			if (targetSpeed > lastSpeed && finalSpeed > targetSpeed) || (targetSpeed < lastSpeed && finalSpeed < targetSpeed) {
				finalSpeed = targetSpeed
			}
		}

		// Only send command if speed changes significantly (more than 5%)
		if finalSpeed != lastSpeed && abs(finalSpeed-lastSpeed) >= 5 {
			if err := c.ipmi.SetFanSpeed(ctx, zone, finalSpeed); err != nil {
				c.logger.LogIPMIError("set fan speed", err)
			} else {
				c.mu.Lock()
				c.lastSpeeds[zone] = finalSpeed
				c.zoneLastChanged[zone] = time.Now()
				c.zoneTargetSpeeds[zone] = targetSpeed
				c.mu.Unlock()
				log.Info().Int("zone", zone).Int("percent", finalSpeed).Int("target", targetSpeed).Msg("Fan speed updated")
			}
		}
	}

	// Log zone conflicts if any
	for zone, count := range zoneConflictCount {
		if count > 0 {
			log.Warn().Int("zone", zone).Int("conflicts", count).Msg("Profile conflicts detected in this control cycle")
			c.logger.LogSystemEvent("Profile conflict detected", models.JSONMap{
				"zone": zone,
				"conflict_count": count,
				"controlling_profile": zoneControllingProfile[zone],
				"controlling_priority": zoneControllingPriority[zone],
			})
		}
	}

}

// gatherInputs collects all input metrics
func (c *FanController) gatherInputs() map[string]float64 {
	inputs := make(map[string]float64)
	c.lastGPUs, c.lastSystem = nil, nil

	// GPU metrics
	if gpuMetrics, err := c.gpu.GetMetrics(); err == nil {
		c.lastGPUs = gpuMetrics
		for _, m := range gpuMetrics {
			idx := strconv.Itoa(m.Index)
			inputs[models.InputTypeGPUTemp+idx] = float64(m.Temperature)
			inputs[models.InputTypeGPULoad+idx] = float64(m.Load)
		}

	}

	// System metrics
	if sysMetrics, err := c.system.GetMetrics(); err == nil {
		c.lastSystem = sysMetrics
		// CPU package temperatures
		if len(sysMetrics.CPUPackages) > 0 {
			for i, pkg := range sysMetrics.CPUPackages {
				inputs[models.InputTypeCPUTemp+strconv.Itoa(i)] = pkg.Temperature
			}
			// Single CPU temp field uses first package
			inputs[models.InputTypeCPUTemp] = sysMetrics.CPUPackages[0].Temperature
		} else if sysMetrics.CPUTemp != nil {
			inputs[models.InputTypeCPUTemp] = *sysMetrics.CPUTemp
		}

		// Drive temperatures
		if len(sysMetrics.Drives) > 0 {
			for i, drive := range sysMetrics.Drives {
				inputs[models.InputTypeDriveTemp+strconv.Itoa(i)] = float64(drive.Temperature)
			}
		}

		// Board temperatures (motherboard/VRM/chipset)
		for _, bt := range sysMetrics.BoardTemps {
			inputs[models.InputTypeBoardTemp+strconv.Itoa(bt.Index)] = bt.Temperature
		}

		inputs[models.InputTypeCPULoad] = sysMetrics.CPULoad
	}

	return inputs
}

// paramFloat reads a float from algorithm params with a default.
func paramFloat(p models.AlgorithmParams, key string, def float64) float64 {
	if v, ok := p[key]; ok {
		if f, ok := v.(float64); ok {
			return f
		}
	}
	return def
}

// profileInputAxis returns the low/high bounds of the profile's control input
// axis — i.e. the "temperature" domain its curve maps to speed. Load inputs are
// projected onto this axis so they can be combined with real temperatures.
func profileInputAxis(profile *models.Profile) (float64, float64) {
	p := profile.AlgorithmParams
	switch profile.Algorithm {
	case "step":
		var lo, hi float64
		found := false
		if steps, ok := p["steps"].([]any); ok {
			for _, s := range steps {
				m, ok := s.(map[string]any)
				if !ok {
					continue
				}
				t, ok := m["temp"].(float64)
				if !ok {
					continue
				}
				if !found {
					lo, hi, found = t, t, true
					continue
				}
				if t < lo {
					lo = t
				}
				if t > hi {
					hi = t
				}
			}
		}
		if !found || hi <= lo {
			return 30, 80
		}
		return lo, hi
	case "pid":
		// PID drives the aggregate (process variable) toward setpoint. Center a
		// nominal band on the setpoint so load maps sensibly around it.
		sp := paramFloat(p, "setpoint", 70)
		return sp - 15, sp + 15
	default: // linear
		lo := paramFloat(p, "min_temp", 30)
		hi := paramFloat(p, "max_temp", 80)
		if hi <= lo {
			return 30, 80
		}
		return lo, hi
	}
}

// isLoadInput reports whether an input type is a utilization (%) signal rather
// than a temperature.
func isLoadInput(t string) bool {
	return t == models.InputTypeGPULoad || t == models.InputTypeCPULoad
}

// calculateInputValue calculates the combined input value for a profile.
//
// Inputs may mix temperatures (°C) and utilization/load (%). These are
// different units, so load values are first projected onto the profile's curve
// axis (0% load → axis low, 100% → axis high) before aggregation. This makes
// mixed load+temp profiles behave sensibly: with Max, fans respond to whichever
// of "how hot" or "how loaded" demands more cooling; with weighted/avg they
// blend on a common scale instead of averaging incompatible units.
func (c *FanController) calculateInputValue(profile *models.Profile, inputs map[string]float64) float64 {
	if len(profile.Inputs) == 0 {
		return 0
	}

	axisLo, axisHi := profileInputAxis(profile)
	axisSpan := axisHi - axisLo
	if axisSpan <= 0 {
		axisLo, axisSpan = 30, 50 // 30..80 fallback
	}

	var values []float64
	var weights []float64

	for _, input := range profile.Inputs {
		key := input.InputType

		// For indexed input types, append the index
		switch input.InputType {
		case models.InputTypeGPUTemp, models.InputTypeGPULoad,
			models.InputTypeCPUTemp, models.InputTypeDriveTemp, models.InputTypeBoardTemp:
			if input.InputIndex >= 0 {
				key += strconv.Itoa(input.InputIndex)
			}
		}

		if val, ok := inputs[key]; ok {
			if isLoadInput(input.InputType) {
				// Project load% onto the curve's temperature axis.
				lv := val
				if lv < 0 {
					lv = 0
				} else if lv > 100 {
					lv = 100
				}
				val = axisLo + (lv/100.0)*axisSpan
			}
			values = append(values, val)
			weights = append(weights, input.Weight)
		}
	}

	if len(values) == 0 {
		return 0
	}

	// input_aggregation: "or"/max (default, respond to hottest), "and"/min (all
	// must be cool), "avg", or "weighted" (uses per-input weights).
	aggregation := models.AggregationMax
	if agg, ok := profile.AlgorithmParams["input_aggregation"]; ok {
		if aggStr, ok := agg.(string); ok {
			switch aggStr {
			case "and", models.AggregationMin:
				aggregation = models.AggregationMin
			case models.AggregationAvg:
				aggregation = models.AggregationAvg
			case models.AggregationWeighted:
				aggregation = models.AggregationWeighted
			}
		}
	}

	return algorithms.AggregateInputs(values, aggregation, weights)
}

// abs returns absolute value
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// getMaxTemperature returns the maximum temperature across GPU/CPU/drive inputs
// and the count of such readings found (0 = no temperature could be read). Board
// temps are deliberately excluded: nct6xxx aux/VRM sensors can read bogus-high
// or sit legitimately warm, so they never drive the emergency threshold.
// thermalCfgSnapshot returns the thermal configuration to evaluate with. If
// settings were never loaded (controller not started yet), it loads them once
// so REST/WS annotations don't silently fall back to legacy defaults.
func (c *FanController) thermalCfgSnapshot() ThermalConfig {
	c.mu.RLock()
	cfg := c.thermalCfg
	c.mu.RUnlock()
	if cfg.Mode != "" {
		return cfg
	}
	if s, err := database.GetAllSettings(); err == nil && s != nil {
		cfg = thermalConfigFrom(s)
		c.mu.Lock()
		if c.thermalCfg.Mode == "" {
			c.thermalCfg = cfg
		}
		c.mu.Unlock()
		return cfg
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return ThermalConfig{
		Mode: models.ThermalModeLegacy, LegacyWarning: c.warningTemp,
		EmergencyTemp: c.emergencyTemp, WarningEnabled: c.warningEnabled,
	}
}

// AnnotateThermal fills the per-device thermal fields (limit, headroom,
// status) on a monitoring payload with the current configuration, so REST and
// WebSocket consumers see the same judgement the control loop makes.
func (c *FanController) AnnotateThermal(gpus []models.GPUMetrics, sys *models.SystemMetrics) {
	EvaluateThermal(c.thermalCfgSnapshot(), gpus, sys)
}

// publishThermal stores the cycle's thermal state for GetState.
func (c *FanController) publishThermal(state models.ThermalState, emergency bool) {
	state.EmergencyActive = emergency
	c.mu.Lock()
	c.thermal = &state
	c.mu.Unlock()
}

// logEffectiveLimits logs each device's limit once per configuration, so the
// thresholds in force are visible without reading the code.
func (c *FanController) logEffectiveLimits(ev ThermalEvaluation) {
	if c.limitsLogged || len(ev.Devices) == 0 {
		return
	}
	c.limitsLogged = true
	for _, d := range ev.Devices {
		log.Info().Str("device", DeviceLabel(d)).Int("limit", d.Limit).Float64("temp", d.Temperature).
			Int("headroom", d.Headroom).Msg("Thermal limit in force")
	}
}

// thermalReminder is how often a device that STAYS in warning/critical is
// logged again; otherwise events are written only when its status changes.
const thermalReminder = 15 * time.Minute

// logThermalTransitions writes one event per device when its status changes
// (entering warning, entering critical, recovering) and a periodic reminder
// while it stays there — a device that runs warm for hours produces a handful
// of events, not one per cycle.
func (c *FanController) logThermalTransitions(cfg ThermalConfig, ev ThermalEvaluation) {
	if c.deviceStatus == nil {
		c.deviceStatus = make(map[string]string)
		c.deviceLastLog = make(map[string]time.Time)
	}
	now := time.Now()
	for _, d := range ev.Devices {
		key := DeviceKey(d)
		prev, seen := c.deviceStatus[key]
		changed := seen && prev != d.Status
		if !seen {
			// First sight: only report if already outside "ok".
			changed = d.Status != models.ThermalOK
		}
		reminder := d.Status != models.ThermalOK && now.Sub(c.deviceLastLog[key]) >= thermalReminder
		c.deviceStatus[key] = d.Status
		if !changed && !reminder {
			continue
		}
		c.deviceLastLog[key] = now

		var where string
		if cfg.Mode == models.ThermalModeLegacy {
			where = fmt.Sprintf("%s %.0f°C", DeviceLabel(d), d.Temperature)
		} else {
			where = fmt.Sprintf("%s %.0f°C — %d°C from its limit (%d°C)", DeviceLabel(d), d.Temperature, d.Headroom, d.Limit)
		}
		details := models.JSONMap{
			"kind": d.Kind, "index": d.Index, "device": d.Name, "temp": d.Temperature,
			"limit": d.Limit, "headroom": d.Headroom, "status": d.Status, "previous": prev,
		}
		switch d.Status {
		case models.ThermalCritical:
			log.Warn().Str("device", DeviceLabel(d)).Float64("temp", d.Temperature).Float64("emergency_temp", cfg.EmergencyTemp).Msg("Thermal emergency")
			c.logger.Error(models.CategoryTemp, fmt.Sprintf("Thermal emergency: %s reached the emergency temperature (%.0f°C) — all fans at emergency speed", where, cfg.EmergencyTemp), details)
		case models.ThermalWarning:
			if cfg.Mode == models.ThermalModeLegacy {
				c.logger.Warn(models.CategoryTemp, fmt.Sprintf("%s exceeded the warning threshold (%.0f°C)", where, cfg.LegacyWarning), details)
			} else {
				c.logger.Warn(models.CategoryTemp, "Running warm: "+where, details)
			}
		default:
			c.logger.Info(models.CategoryTemp, "Recovered: "+where, details)
		}
	}
}

func (c *FanController) getMaxTemperature(inputs map[string]float64) (float64, int) {
	maxTemp := 0.0
	count := 0
	for key, val := range inputs {
		isTemp := false
		for _, prefix := range []string{models.InputTypeGPUTemp, models.InputTypeCPUTemp, models.InputTypeDriveTemp} {
			if strings.HasPrefix(key, prefix) {
				isTemp = true
				break
			}
		}
		if isTemp {
			count++
			if val > maxTemp {
				maxTemp = val
			}
		}
	}
	return maxTemp, count
}

// setAllFans sets all fan zones to the same speed
func (c *FanController) setAllFans(ctx context.Context, percent int) {
	if err := c.ipmi.SetAllFanSpeeds(ctx, percent); err != nil {
		c.logger.LogIPMIError("set all fans", err)
	}
}

// GetState returns the current controller state
func (c *FanController) GetState() models.ControllerState {
	c.mu.RLock()
	defer c.mu.RUnlock()

	state := models.ControllerState{
		Running:    c.running.Load(),
		ManualMode: c.ipmi.IsManualMode(),
	}
	if c.thermal != nil {
		t := *c.thermal
		state.Thermal = &t
	}

	// Load all active profiles
	var profiles []models.Profile
	if err := database.DB.Where("is_active = ?", true).Find(&profiles).Error; err == nil {
		for _, p := range profiles {
			state.ActiveProfiles = append(state.ActiveProfiles, p.Name)
			state.ActiveProfileIDs = append(state.ActiveProfileIDs, p.ID)
		}
	}

	return state
}


// UpdateSettings updates controller settings
func (c *FanController) UpdateSettings(settings *models.AppSettings) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.emergencyTemp = float64(settings.EmergencyTemp)
	c.emergencySpeed = settings.EmergencySpeed
	c.warningTemp = float64(settings.WarningTemp)
	c.warningEnabled = settings.WarningEnabled
	c.interval = time.Duration(settings.ControlInterval) * time.Second
	c.thermalCfg = thermalConfigFrom(settings)
	c.limitsLogged = false
	log.Info().Str("mode", c.thermalCfg.Mode).Int("warning_margin", c.thermalCfg.WarningMargin).
		Float64("emergency_temp", c.thermalCfg.EmergencyTemp).Msg("Thermal limits updated")
}
