package services

import (
	"context"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"docker-fan-control/internal/algorithms"
	"docker-fan-control/internal/database"
	"docker-fan-control/internal/models"

	"github.com/rs/zerolog/log"
)

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
	manualOverrides map[uint]int // fan ID -> percent
	lastSpeeds      map[int]int  // zone -> percent
	zoneLastChanged map[int]time.Time // zone -> last change time
	zoneTargetSpeeds map[int]int  // zone -> target speed (for smoothing)
	emergencyTemp   float64
	emergencySpeed  int
	warningTemp    float64
	warningEnabled bool
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
		manualOverrides: make(map[uint]int),
		lastSpeeds:      make(map[int]int),
		zoneLastChanged: make(map[int]time.Time),
		zoneTargetSpeeds: make(map[int]int),
		emergencyTemp:   90,
		emergencySpeed:  100,
		warningTemp:     70,
		warningEnabled:  true,
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

	go c.controlLoop(ctx)

	c.logger.LogSystemEvent("Fan control service started", models.JSONMap{
		"interval": c.interval.String(),
	})

	return nil
}

// Stop gracefully stops the fan control loop
func (c *FanController) Stop() {
	c.StopWithSafety(true)
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

// SetManualOverride sets a manual speed override for a fan
func (c *FanController) SetManualOverride(fanID uint, percent int) {
	c.mu.Lock()
	c.manualOverrides[fanID] = percent
	c.mu.Unlock()
}

// ClearManualOverride removes a manual override for a fan
func (c *FanController) ClearManualOverride(fanID uint) {
	c.mu.Lock()
	delete(c.manualOverrides, fanID)
	c.mu.Unlock()
}

// HasManualOverride checks if a fan has a manual override
func (c *FanController) HasManualOverride(fanID uint) bool {
	c.mu.RLock()
	_, ok := c.manualOverrides[fanID]
	c.mu.RUnlock()
	return ok
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

// controlCycle performs one control cycle
func (c *FanController) controlCycle(ctx context.Context) {
	// Gather inputs
	inputs := c.gatherInputs()

	// Check for emergency conditions
	maxTemp := c.getMaxTemperature(inputs)
	if maxTemp >= c.emergencyTemp {
		log.Warn().Float64("temp", maxTemp).Float64("threshold", c.emergencyTemp).Msg("Emergency temperature threshold exceeded")
		c.setAllFans(ctx, c.emergencySpeed)
		c.logger.LogTemperatureWarning("system", maxTemp, c.emergencyTemp)
		return
	}

	// Log warnings
	if c.warningEnabled && maxTemp >= c.warningTemp {
		c.logger.LogTemperatureWarning("system", maxTemp, c.warningTemp)
	}

	// Get manual overrides
	c.mu.RLock()
	manualOverrides := make(map[uint]int)
	for k, v := range c.manualOverrides {
		manualOverrides[k] = v
	}
	c.mu.RUnlock()

	// Load ALL active profiles (supports multiple simultaneous profiles)
	var profiles []models.Profile
	if err := database.DB.Preload("Inputs").Where("is_active = ?", true).Find(&profiles).Error; err != nil {
		log.Error().Err(err).Msg("Failed to load active profiles")
		return
	}

	if len(profiles) == 0 {
		log.Debug().Msg("No active profiles")
		return // No active profiles
	}

	// Sort profiles by priority (highest first)
	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].Priority > profiles[j].Priority
	})

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

		// Get algorithm and calculate target speed
		algo := algorithms.NewAlgorithm(profile.Algorithm, profile.AlgorithmParams)
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

	// Apply zone targets with smoothing and minimum run time
	for zone, targetSpeed := range zoneTargets {
		c.mu.RLock()
		lastSpeed := c.lastSpeeds[zone]
		lastChanged := c.zoneLastChanged[zone]
		currentTarget := c.zoneTargetSpeeds[zone]
		c.mu.RUnlock()

		// Check minimum run time
		if !lastChanged.IsZero() && time.Since(lastChanged) < time.Duration(30*time.Second) {
			// Respect minimum run time
			if err := c.ipmi.SetFanSpeed(ctx, zone, lastSpeed); err != nil {
				c.logger.LogIPMIError("set fan speed", err)
			}
			continue
		}

		// Apply smoothing if enabled (default)
		finalSpeed := targetSpeed
		if currentTarget != targetSpeed {
			// Calculate intermediate speed for smooth transition
			// Simple linear interpolation based on time
			elapsed := time.Since(lastChanged)
			transitionTime := 10 * time.Second // Default transition time
			
			// Calculate percentage of transition complete
			progress := float64(elapsed) / float64(transitionTime)
			if progress > 1.0 {
				progress = 1.0
			}
			
			// Interpolate between current and target
			finalSpeed = int(float64(lastSpeed) + (float64(targetSpeed) - float64(lastSpeed)) * progress)
			
			// Ensure we don't skip over the target
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
			inputs[models.InputTypeGPUTemp+string(rune('0'+m.Index))] = float64(m.Temperature)
			inputs[models.InputTypeGPULoad+string(rune('0'+m.Index))] = float64(m.Load)
		}

	}

	// System metrics
	if sysMetrics, err := c.system.GetMetrics(); err == nil {
		// CPU package temperatures
		if len(sysMetrics.CPUPackages) > 0 {
			for i, pkg := range sysMetrics.CPUPackages {
				inputs[models.InputTypeCPUTemp+string(rune('0'+i))] = pkg.Temperature
			}
			// Single CPU temp field uses first package
			inputs[models.InputTypeCPUTemp] = sysMetrics.CPUPackages[0].Temperature
		} else if sysMetrics.CPUTemp != nil {
			inputs[models.InputTypeCPUTemp] = *sysMetrics.CPUTemp
		}

		// Drive temperatures
		if len(sysMetrics.Drives) > 0 {
			for i, drive := range sysMetrics.Drives {
				inputs[models.InputTypeDriveTemp+string(rune('0'+i))] = float64(drive.Temperature)
			}
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
			models.InputTypeCPUTemp, models.InputTypeDriveTemp:
			if input.InputIndex >= 0 {
				key += string(rune('0' + input.InputIndex))
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

	// Check for input_aggregation in algorithm params
	// "and" = use MIN (all must be cool), "or" = use MAX (respond to hottest)
	aggregation := models.AggregationMax // Default: OR logic (max)
	if agg, ok := profile.AlgorithmParams["input_aggregation"]; ok {
		if aggStr, ok := agg.(string); ok && aggStr == "and" {
			aggregation = models.AggregationMin
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

// getMaxTemperature returns the maximum temperature from inputs
func (c *FanController) getMaxTemperature(inputs map[string]float64) float64 {
	maxTemp := 0.0

	for key, val := range inputs {
		isTemp := false
		for _, prefix := range []string{models.InputTypeGPUTemp, models.InputTypeCPUTemp, models.InputTypeDriveTemp} {
			if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
				isTemp = true
				break
			}
		}
		if isTemp && val > maxTemp {
			maxTemp = val
		}
	}

	return maxTemp
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
