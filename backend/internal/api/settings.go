package api

import (
	"encoding/json"
	"net"
	"net/http"
	"regexp"
	"time"

	"docker-fan-control/internal/database"
	"docker-fan-control/internal/models"
	"docker-fan-control/internal/services"
)

// ipmiHostRegex validates IPMI host (hostname or IP address)
var ipmiHostRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.-]{0,253}[a-zA-Z0-9]$|^[a-zA-Z0-9]$`)

// ipmiUserRegex validates IPMI username (alphanumeric, underscores, hyphens)
var ipmiUserRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// SettingsHandler handles settings endpoints
type SettingsHandler struct {
	ipmi       *services.IPMIService
	controller *services.FanController
	auth       *services.AuthService
	logger     *services.EventLogger
}

// NewSettingsHandler creates a new settings handler
func NewSettingsHandler(ipmi *services.IPMIService, controller *services.FanController, auth *services.AuthService, logger *services.EventLogger) *SettingsHandler {
	return &SettingsHandler{ipmi: ipmi, controller: controller, auth: auth, logger: logger}
}

// Get handles GET /api/settings
func (h *SettingsHandler) Get(w http.ResponseWriter, r *http.Request) {
	settings, err := database.GetAllSettings()
	if err != nil {
		http.Error(w, "Failed to get settings", http.StatusInternalServerError)
		return
	}

	writeJSON(w, settings)
}

// Update handles PUT /api/settings
func (h *SettingsHandler) Update(w http.ResponseWriter, r *http.Request) {
	var req models.UpdateSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate enum values
	if req.IPMIMode != nil && *req.IPMIMode != "local" && *req.IPMIMode != "lan" {
		http.Error(w, "IPMI mode must be 'local' or 'lan'", http.StatusBadRequest)
		return
	}

	// Validate IPMI host (must be valid hostname or IP)
	if req.IPMIHost != nil && *req.IPMIHost != "" {
		host := *req.IPMIHost
		// Check if it's a valid IP address
		if ip := net.ParseIP(host); ip == nil {
			// Not an IP, validate as hostname
			if !ipmiHostRegex.MatchString(host) || len(host) > 255 {
				http.Error(w, "Invalid IPMI host format", http.StatusBadRequest)
				return
			}
		}
	}

	// Validate IPMI username
	if req.IPMIUser != nil && *req.IPMIUser != "" {
		if !ipmiUserRegex.MatchString(*req.IPMIUser) {
			http.Error(w, "IPMI username can only contain letters, numbers, underscores, and hyphens (max 64 chars)", http.StatusBadRequest)
			return
		}
	}

	if req.StartupMode != nil && *req.StartupMode != "resume" && *req.StartupMode != "full" && *req.StartupMode != "percent" {
		http.Error(w, "Startup mode must be 'resume', 'full', or 'percent'", http.StatusBadRequest)
		return
	}

	validFormats := map[string]bool{
		models.IPMIFormatAuto: true, models.IPMIFormatAsrockROMED8: true,
		models.IPMIFormatAsrockLegacy: true, models.IPMIFormatDell: true, models.IPMIFormatSupermicro: true,
	}
	if req.IPMICommandFormat != nil && !validFormats[*req.IPMICommandFormat] {
		http.Error(w, "Invalid IPMI command format", http.StatusBadRequest)
		return
	}

	// Validate numeric ranges
	if req.ControlInterval != nil && (*req.ControlInterval < 1 || *req.ControlInterval > 60) {
		http.Error(w, "Control interval must be between 1 and 60 seconds", http.StatusBadRequest)
		return
	}

	if req.EmergencyTemp != nil && (*req.EmergencyTemp < 30 || *req.EmergencyTemp > 120) {
		http.Error(w, "Emergency temperature must be between 30 and 120°C", http.StatusBadRequest)
		return
	}

	if req.EmergencySpeed != nil && (*req.EmergencySpeed < 0 || *req.EmergencySpeed > 100) {
		http.Error(w, "Emergency speed must be between 0 and 100%", http.StatusBadRequest)
		return
	}

	if req.StartupPercent != nil && (*req.StartupPercent < 0 || *req.StartupPercent > 100) {
		http.Error(w, "Startup percent must be between 0 and 100", http.StatusBadRequest)
		return
	}

	if req.WarningTemp != nil && (*req.WarningTemp < 30 || *req.WarningTemp > 120) {
		http.Error(w, "Warning temperature must be between 30 and 120°C", http.StatusBadRequest)
		return
	}

	// Update IPMI settings
	if req.IPMIMode != nil {
		database.SetSetting(models.SettingIPMIMode, *req.IPMIMode)
	}
	if req.IPMIHost != nil {
		database.SetSetting(models.SettingIPMIHost, *req.IPMIHost)
	}
	if req.IPMIUser != nil {
		database.SetSetting(models.SettingIPMIUser, *req.IPMIUser)
	}
	if req.IPMIPass != nil && *req.IPMIPass != "" {
		database.SetSetting(models.SettingIPMIPass, *req.IPMIPass)
	}
	if req.IPMICommandFormat != nil {
		database.SetSetting(models.SettingIPMICommandFormat, *req.IPMICommandFormat)
		// Apply the format to the IPMI service
		h.ipmi.SetCommandFormat(services.ParseIPMIFormat(*req.IPMICommandFormat))
	}
	
	// Update motherboard settings
	if req.MotherboardVendor != nil {
		database.SetSetting(models.SettingMotherboardVendor, *req.MotherboardVendor)
	}
	if req.MotherboardModel != nil {
		database.SetSetting(models.SettingMotherboardModel, *req.MotherboardModel)
	}
	if req.MotherboardDriver != nil {
		database.SetSetting(models.SettingMotherboardDriver, *req.MotherboardDriver)
		// Apply the driver selection
		h.applyMotherboardDriver(*req.MotherboardDriver)
	}
	
	// Update zone layout
	if req.ZoneLayout != nil {
		zoneLayoutBytes, err := json.Marshal(req.ZoneLayout)
		if err == nil {
			database.SetSetting(models.SettingZoneLayout, string(zoneLayoutBytes))
		}
	}

	// Update control settings
	if req.ControlInterval != nil {
		database.SetSetting(models.SettingControlInterval, *req.ControlInterval)
		h.controller.SetInterval(time.Duration(*req.ControlInterval) * time.Second)
	}
	if req.StartupMode != nil {
		database.SetSetting(models.SettingStartupMode, *req.StartupMode)
	}
	if req.StartupPercent != nil {
		database.SetSetting(models.SettingStartupPercent, *req.StartupPercent)
	}

	// Update safety settings
	if req.EmergencyTemp != nil {
		database.SetSetting(models.SettingEmergencyTemp, *req.EmergencyTemp)
	}
	if req.EmergencySpeed != nil {
		database.SetSetting(models.SettingEmergencySpeed, *req.EmergencySpeed)
	}
	if req.WarningTemp != nil {
		database.SetSetting(models.SettingWarningTemp, *req.WarningTemp)
	}
	if req.WarningEnabled != nil {
		database.SetSetting(models.SettingWarningEnabled, *req.WarningEnabled)
	}
	if req.SafetyOnShutdown != nil {
		database.SetSetting(models.SettingSafetyOnShutdown, *req.SafetyOnShutdown)
	}

	// Update IPMI service configuration
	settings, _ := database.GetAllSettings()
	ipmiPass := ""
	if val, err := database.GetSetting(models.SettingIPMIPass); err == nil {
		if pass, ok := val.(string); ok {
			ipmiPass = pass
		}
	}
	h.ipmi.UpdateConfig(settings.IPMIMode, settings.IPMIHost, settings.IPMIUser, ipmiPass)

	// Update controller settings
	h.controller.UpdateSettings(
		float64(settings.EmergencyTemp),
		float64(settings.EmergencySpeed),
		float64(settings.WarningTemp),
		settings.WarningEnabled,
		time.Duration(settings.ControlInterval)*time.Second,
	)

	h.logger.Info(models.CategorySystem, "Settings updated", nil)

	// Return updated settings
	updatedSettings, _ := database.GetAllSettings()
	writeJSON(w, updatedSettings)
}

// applyMotherboardDriver sets the IPMI driver based on motherboard selection
func (h *SettingsHandler) applyMotherboardDriver(driverName string) {
	registry := h.ipmi.GetDriverRegistry()
	
	// Find the driver by name
	driver := registry.GetDriverByVendor(driverName)
	if driver != nil {
		h.ipmi.SetDriver(driver)
	}
}

// DetectMotherboard handles POST /api/settings/detect-motherboard
func (h *SettingsHandler) DetectMotherboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	
	// Detect the best driver (drivers are registered at startup in main.go)
	err := h.ipmi.DetectAndSetDriver(ctx)
	if err != nil {
		h.logger.Error(models.CategoryIPMI, "Motherboard detection failed", models.JSONMap{"error": err.Error()})
		writeJSON(w, map[string]any{
			"success": false,
			"error":   "Motherboard detection failed",
		})
		return
	}
	
	// Get the detected driver info
	driver := h.ipmi.GetCurrentDriver()
	var driverInfo map[string]any
	if driver != nil {
		caps := driver.GetCapabilities()
		zoneLayout := driver.GetZoneLayout()
		
		// Serialize zone layout
		zones := make([]map[string]any, len(zoneLayout.Zones))
		for i, zone := range zoneLayout.Zones {
			zones[i] = map[string]any{
				"id":           zone.ID,
				"name":         zone.Name,
				"fan_indices":  zone.FanIndices,
				"description": zone.Description,
				"is_default":  zone.IsDefault,
			}
		}
		
		driverInfo = map[string]any{
			"vendor":       driver.GetVendor(),
			"model":        driver.GetModel(),
			"capabilities": map[string]any{
				"supports_manual_mode":       caps.SupportsManualMode,
				"supports_duty_cycle_reading": caps.SupportsDutyCycleReading,
				"supports_per_zone_control":   caps.SupportsPerZoneControl,
				"max_zones":                 caps.MaxZones,
				"max_fans":                  caps.MaxFans,
				"has_static_rpm_values":      caps.HasStaticRPMValues,
			},
			"zone_layout": map[string]any{
				"zones": zones,
			},
		}
		
		// Save detected motherboard info
		database.SetSetting(models.SettingMotherboardVendor, driver.GetVendor())
		database.SetSetting(models.SettingMotherboardModel, driver.GetModel())
		database.SetSetting(models.SettingMotherboardDriver, driver.GetVendor())
	}
	
	writeJSON(w, map[string]any{
		"success":      true,
		"driver":       driverInfo,
		"message":      "Motherboard detection completed",
	})
}

// GetAvailableDrivers handles GET /api/settings/drivers
func (h *SettingsHandler) GetAvailableDrivers(w http.ResponseWriter, r *http.Request) {
	registry := h.ipmi.GetDriverRegistry()
	drivers := registry.GetAllDriverInfo()
	
	driverList := make([]map[string]any, len(drivers))
	for i, driver := range drivers {
		// Serialize zone layout
		zones := make([]map[string]any, len(driver.ZoneLayout.Zones))
		for j, zone := range driver.ZoneLayout.Zones {
			zones[j] = map[string]any{
				"id":           zone.ID,
				"name":         zone.Name,
				"fan_indices":  zone.FanIndices,
				"description": zone.Description,
				"is_default":  zone.IsDefault,
			}
		}
		
		driverList[i] = map[string]any{
			"vendor":       driver.Vendor,
			"model":        driver.Model,
			"capabilities": map[string]any{
				"supports_manual_mode":       driver.Capabilities.SupportsManualMode,
				"supports_duty_cycle_reading": driver.Capabilities.SupportsDutyCycleReading,
				"supports_per_zone_control":   driver.Capabilities.SupportsPerZoneControl,
				"max_zones":                 driver.Capabilities.MaxZones,
				"max_fans":                  driver.Capabilities.MaxFans,
				"has_static_rpm_values":      driver.Capabilities.HasStaticRPMValues,
			},
			"zone_layout": map[string]any{
				"zones": zones,
			},
		}
	}
	
	writeJSON(w, driverList)
}

// TestIPMI handles POST /api/settings/test-ipmi
func (h *SettingsHandler) TestIPMI(w http.ResponseWriter, r *http.Request) {
	if err := h.ipmi.TestConnection(r.Context()); err != nil {
		h.logger.LogIPMIError("connection test", err)
		writeJSON(w, map[string]any{
			"success": false,
			"error":   "IPMI connection test failed",
		})
		return
	}

	// Get chassis status for extra info
	status, _ := h.ipmi.GetChassisStatus(r.Context())

	writeJSON(w, map[string]any{
		"success":        true,
		"message":        "IPMI connection successful",
		"chassis_status": status,
	})
}

// StartController handles POST /api/controller/start
func (h *SettingsHandler) StartController(w http.ResponseWriter, r *http.Request) {
	if h.controller.IsRunning() {
		writeJSON(w, map[string]any{
			"success": true,
			"message": "Controller already running",
		})
		return
	}

	if err := h.controller.Start(r.Context()); err != nil {
		h.logger.Error(models.CategorySystem, "Failed to start controller", models.JSONMap{"error": err.Error()})
		http.Error(w, "Failed to start controller", http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]any{
		"success": true,
		"message": "Controller started",
	})
}

// StopController handles POST /api/controller/stop
func (h *SettingsHandler) StopController(w http.ResponseWriter, r *http.Request) {
	if !h.controller.IsRunning() {
		writeJSON(w, map[string]any{
			"success": true,
			"message": "Controller already stopped",
		})
		return
	}

	h.controller.Stop()

	writeJSON(w, map[string]any{
		"success": true,
		"message": "Controller stopped",
	})
}
