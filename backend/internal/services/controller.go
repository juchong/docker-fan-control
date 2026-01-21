package services

import (
	"context"
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
	activeProfileID *uint
	manualOverrides map[uint]int // fan ID -> percent
	lastSpeeds      map[int]int  // zone -> percent
	emergencyTemp   float64
	emergencySpeed  int
	warningTemp     float64
	warningEnabled  bool
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
		if err := c.ipmi.SetAllFanSpeeds(ctx, 100); err != nil {
			log.Error().Err(err).Msg("Failed to set safe fan speed on shutdown")
			c.logger.LogIPMIError("safety shutdown fan speed", err)
		} else {
			c.logger.LogSystemEvent("Safety: fans set to 100% on shutdown", nil)
		}
	}

	// Restore automatic fan control (BMC takes over)
	if err := c.ipmi.SetManualMode(ctx, false); err != nil {
		log.Warn().Err(err).Msg("Failed to restore automatic fan control")
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

// SetActiveProfile activates or deactivates a profile
// Multiple profiles can be active simultaneously if they control different zones
func (c *FanController) SetActiveProfile(profileID *uint) {
	c.mu.Lock()
	c.activeProfileID = profileID
	c.mu.Unlock()

	// Update database - now just toggles the single profile
	if profileID != nil {
		database.SetSetting(models.SettingActiveProfileID, *profileID)
		database.DB.Model(&models.Profile{}).Where("id = ?", *profileID).Update("is_active", true)
	} else {
		database.SetSetting(models.SettingActiveProfileID, nil)
	}
}

// ActivateProfile activates a specific profile (allows multiple active)
func (c *FanController) ActivateProfile(profileID uint) {
	database.DB.Model(&models.Profile{}).Where("id = ?", profileID).Update("is_active", true)
}

// DeactivateProfile deactivates a specific profile
func (c *FanController) DeactivateProfile(profileID uint) {
	database.DB.Model(&models.Profile{}).Where("id = ?", profileID).Update("is_active", false)
}

// GetActiveProfileID returns the current active profile ID
func (c *FanController) GetActiveProfileID() *uint {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.activeProfileID
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

	// Load active profile
	if val, err := database.GetSetting(models.SettingActiveProfileID); err == nil {
		if id, ok := val.(float64); ok {
			profileID := uint(id)
			c.activeProfileID = &profileID
		}
	}
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
	if err := database.DB.Preload("Fans").Preload("Inputs").Where("is_active = ?", true).Find(&profiles).Error; err != nil {
		log.Warn().Err(err).Msg("Failed to load active profiles")
		return
	}

	if len(profiles) == 0 {
		return // No active profiles
	}

	// Collect zone targets from all profiles
	// Higher speed wins if multiple profiles target the same zone
	zoneTargets := make(map[int]int)
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
				// Use highest target if multiple profiles share a zone (safety)
				if existing, ok := zoneTargets[zone]; ok {
					if targetSpeed > existing {
						zoneTargets[zone] = targetSpeed
					}
				} else {
					zoneTargets[zone] = targetSpeed
				}
			}
			// Skip backward compatibility code if Zones is configured
			continue
		}

		// Backward compatibility: check fans for zones (only if Zones is empty)
		for _, fan := range profile.Fans {
			// Check for manual override
			if override, ok := manualOverrides[fan.ID]; ok {
				if fan.IPMIZone != nil {
					zoneTargets[*fan.IPMIZone] = override
					hasAnyZones = true
				}
				continue
			}

			if fan.IPMIZone != nil {
				hasAnyZones = true
				// Use highest target if multiple fans share a zone
				if existing, ok := zoneTargets[*fan.IPMIZone]; ok {
					if targetSpeed > existing {
						zoneTargets[*fan.IPMIZone] = targetSpeed
					}
				} else {
					zoneTargets[*fan.IPMIZone] = targetSpeed
				}
			}
		}
	}

	// If no zones assigned, set all fans to max target from all profiles
	if !hasAnyZones {
		maxTargetSpeed := 0
		for _, profile := range profiles {
			inputValue := c.calculateInputValue(&profile, inputs)
			algo := algorithms.NewAlgorithm(profile.Algorithm, profile.AlgorithmParams)
			targetSpeed := algo.Calculate(inputValue)
			if targetSpeed > maxTargetSpeed {
				maxTargetSpeed = targetSpeed
			}
		}

		c.mu.RLock()
		lastSpeed := c.lastSpeeds[-1] // Use -1 as "all fans" zone
		c.mu.RUnlock()

		if maxTargetSpeed != lastSpeed {
			log.Debug().Int("target", maxTargetSpeed).Int("last", lastSpeed).Msg("Setting all fans (no zones assigned)")
			if err := c.ipmi.SetAllFanSpeeds(ctx, maxTargetSpeed); err != nil {
				c.logger.LogIPMIError("set all fan speeds", err)
			} else {
				c.mu.Lock()
				c.lastSpeeds[-1] = maxTargetSpeed
				c.mu.Unlock()
				log.Info().Int("percent", maxTargetSpeed).Msg("Fan speed updated")
			}
		}
		return
	}

	// Apply zone targets
	for zone, speed := range zoneTargets {
		c.mu.RLock()
		lastSpeed := c.lastSpeeds[zone]
		c.mu.RUnlock()

		if speed != lastSpeed {
			if err := c.ipmi.SetFanSpeed(ctx, zone, speed); err != nil {
				c.logger.LogIPMIError("set fan speed", err)
			} else {
				c.mu.Lock()
				c.lastSpeeds[zone] = speed
				c.mu.Unlock()
				log.Info().Int("zone", zone).Int("percent", speed).Msg("Fan speed updated")
			}
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

		// Max and average GPU temps
		if len(gpuMetrics) > 0 {
			maxTemp := 0
			totalTemp := 0
			for _, m := range gpuMetrics {
				if m.Temperature > maxTemp {
					maxTemp = m.Temperature
				}
				totalTemp += m.Temperature
			}
			inputs[models.InputTypeMaxTemp] = float64(maxTemp)
			inputs[models.InputTypeAvgTemp] = float64(totalTemp) / float64(len(gpuMetrics))
		}
	}

	// System metrics
	if sysMetrics, err := c.system.GetMetrics(); err == nil {
		// CPU package temperatures
		if len(sysMetrics.CPUPackages) > 0 {
			maxCPUTemp := sysMetrics.CPUPackages[0].Temperature
			for i, pkg := range sysMetrics.CPUPackages {
				inputs[models.InputTypeCPUTemp+string(rune('0'+i))] = pkg.Temperature
				if pkg.Temperature > maxCPUTemp {
					maxCPUTemp = pkg.Temperature
				}
			}
			inputs[models.InputTypeMaxCPU] = maxCPUTemp
			// Legacy single CPU temp field (for backward compat)
			inputs[models.InputTypeCPUTemp] = maxCPUTemp
		} else if sysMetrics.CPUTemp != nil {
			// Fallback to legacy single CPU temp
			inputs[models.InputTypeCPUTemp] = *sysMetrics.CPUTemp
			inputs[models.InputTypeMaxCPU] = *sysMetrics.CPUTemp
		}

		// Drive temperatures
		if len(sysMetrics.Drives) > 0 {
			maxDriveTemp := float64(sysMetrics.Drives[0].Temperature)
			for i, drive := range sysMetrics.Drives {
				temp := float64(drive.Temperature)
				inputs[models.InputTypeDriveTemp+string(rune('0'+i))] = temp
				if temp > maxDriveTemp {
					maxDriveTemp = temp
				}
			}
			inputs[models.InputTypeMaxDrive] = maxDriveTemp
		}

		inputs[models.InputTypeCPULoad] = sysMetrics.CPULoad
	}

	return inputs
}

// calculateInputValue calculates the aggregated input value for a profile
func (c *FanController) calculateInputValue(profile *models.Profile, inputs map[string]float64) float64 {
	if len(profile.Inputs) == 0 {
		// Default to max GPU temp if no inputs specified
		if maxTemp, ok := inputs[models.InputTypeMaxTemp]; ok {
			return maxTemp
		}
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
			// Only append index if this is an indexed type (not aggregate like max_temp)
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

// getMaxTemperature returns the maximum temperature from inputs
func (c *FanController) getMaxTemperature(inputs map[string]float64) float64 {
	maxTemp := 0.0

	// Check aggregate temps first
	aggregateKeys := []string{
		models.InputTypeMaxTemp,
		models.InputTypeMaxCPU,
		models.InputTypeMaxDrive,
	}
	for _, key := range aggregateKeys {
		if val, ok := inputs[key]; ok && val > maxTemp {
			maxTemp = val
		}
	}

	// Also check individual temps in case aggregates aren't set
	for key, val := range inputs {
		isTemp := false
		// Check if this is an individual temperature input
		for _, prefix := range []string{models.InputTypeGPUTemp, models.InputTypeCPUTemp, models.InputTypeDriveTemp} {
			if len(key) > len(prefix) && key[:len(prefix)] == prefix {
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

		// For backward compatibility, set single profile fields if there's exactly one
		if len(profiles) == 1 {
			state.ActiveProfileID = &profiles[0].ID
			state.ActiveProfile = profiles[0].Name
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
