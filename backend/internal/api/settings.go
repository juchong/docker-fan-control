package api

import (
	"encoding/json"
	"net/http"
	"time"

	"docker-fan-control/internal/database"
	"docker-fan-control/internal/models"
	"docker-fan-control/internal/services"
)

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

	// Update control settings
	if req.ControlInterval != nil {
		database.SetSetting(models.SettingControlInterval, *req.ControlInterval)
		h.controller.SetInterval(time.Duration(*req.ControlInterval) * time.Second)
	}
	if req.TempUnit != nil {
		database.SetSetting(models.SettingTempUnit, *req.TempUnit)
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

// TestIPMI handles POST /api/settings/test-ipmi
func (h *SettingsHandler) TestIPMI(w http.ResponseWriter, r *http.Request) {
	if err := h.ipmi.TestConnection(r.Context()); err != nil {
		h.logger.LogIPMIError("connection test", err)
		writeJSON(w, map[string]any{
			"success": false,
			"error":   err.Error(),
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
		http.Error(w, "Failed to start controller: "+err.Error(), http.StatusInternalServerError)
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
