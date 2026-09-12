package services

import (
	"context"
	"encoding/json"
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
			_ = c.ipmi.SetAllFanSpeeds(ctx, safeFloor)
		}
	}
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

// ClearManualOverride removes a manual override for a fan, returning it to
// automatic (profile) control on the next control cycle.
func (c *FanController) ClearManualOverride(fanID uint) {
	c.mu.Lock()
	delete(c.manualOverrides, fanID)
	c.mu.Unlock()
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

	maxTemp, tempCount := c.getMaxTemperature(inputs)
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

	// Emergency: a genuine over-temperature forces all zones to emergency speed.
	if tempCount > 0 && maxTemp >= c.emergencyTemp {
		c.noTempCycles = 0
		c.setAllFans(ctx, c.emergencySpeed)
		if time.Since(c.lastWarnLogged) > 30*time.Second {
			log.Warn().Float64("temp", maxTemp).Float64("threshold", c.emergencyTemp).Msg("Emergency temperature threshold exceeded")
			c.logger.LogTemperatureWarning("system", maxTemp, c.emergencyTemp)
			c.lastWarnLogged = time.Now()
		}
		return
	}

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

	// Warning log (throttled to avoid flooding the event table every cycle).
	if c.warningEnabled && tempCount > 0 && maxTemp >= c.warningTemp {
		if time.Since(c.lastWarnLogged) > 60*time.Second {
			c.logger.LogTemperatureWarning("system", maxTemp, c.warningTemp)
			c.lastWarnLogged = time.Now()
		}
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
	for zone, targetSpeed := range zoneTargets {
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

	// GPU metrics
	if gpuMetrics, err := c.gpu.GetMetrics(); err == nil {
		for _, m := range gpuMetrics {
			idx := strconv.Itoa(m.Index)
			inputs[models.InputTypeGPUTemp+idx] = float64(m.Temperature)
			inputs[models.InputTypeGPULoad+idx] = float64(m.Load)
		}

	}

	// System metrics
	if sysMetrics, err := c.system.GetMetrics(); err == nil {
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

// calculateInputValue calculates the combined input value for a profile
func (c *FanController) calculateInputValue(profile *models.Profile, inputs map[string]float64) float64 {
	if len(profile.Inputs) == 0 {
		return 0
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
func (c *FanController) UpdateSettings(emergencyTemp, emergencySpeed, warningTemp float64, warningEnabled bool, interval time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.emergencyTemp = emergencyTemp
	c.emergencySpeed = int(emergencySpeed)
	c.warningTemp = warningTemp
	c.warningEnabled = warningEnabled
	c.interval = interval
}
