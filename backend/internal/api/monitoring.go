package api

import (
	"net/http"

	"docker-fan-control/internal/database"
	"docker-fan-control/internal/models"
	"docker-fan-control/internal/services"
)

// MonitoringHandler handles monitoring endpoints
type MonitoringHandler struct {
	gpu        *services.GPUService
	system     *services.SystemService
	ipmi       *services.IPMIService
	controller *services.FanController
	fans       *services.FanService
}

// NewMonitoringHandler creates a new monitoring handler
func NewMonitoringHandler(gpu *services.GPUService, system *services.SystemService, ipmi *services.IPMIService, controller *services.FanController, fans *services.FanService) *MonitoringHandler {
	return &MonitoringHandler{gpu: gpu, system: system, ipmi: ipmi, controller: controller, fans: fans}
}

// GetMetrics handles GET /api/monitoring/metrics
func (h *MonitoringHandler) GetMetrics(w http.ResponseWriter, r *http.Request) {
	monitoring := models.Monitoring{
		Controller: h.controller.GetState(),
	}

	// Get motherboard information
	if vendor, err := database.GetSetting(models.SettingMotherboardVendor); err == nil {
		if v, ok := vendor.(string); ok && v != "" {
			monitoring.Controller.MotherboardVendor = v
		}
	}
	if model, err := database.GetSetting(models.SettingMotherboardModel); err == nil {
		if m, ok := model.(string); ok && m != "" {
			monitoring.Controller.MotherboardModel = m
		}
	}
	if driver, err := database.GetSetting(models.SettingMotherboardDriver); err == nil {
		if d, ok := driver.(string); ok && d != "" {
			monitoring.Controller.MotherboardDriver = d
		}
	}

	// Get current driver info
	currentDriver := h.ipmi.GetCurrentDriver()
	if currentDriver != nil {
		monitoring.Controller.DriverVendor = currentDriver.GetVendor()
		monitoring.Controller.DriverModel = currentDriver.GetModel()
		caps := currentDriver.GetCapabilities()
		monitoring.Controller.DriverCapabilities = models.DriverCapabilities{
			SupportsManualMode:       caps.SupportsManualMode,
			SupportsDutyCycleReading: caps.SupportsDutyCycleReading,
			SupportsPerZoneControl:   caps.SupportsPerZoneControl,
			MaxZones:                 caps.MaxZones,
			MaxFans:                  caps.MaxFans,
			HasStaticRPMValues:       caps.HasStaticRPMValues,
		}
	}

	// Get GPU metrics
	if gpuMetrics, err := h.gpu.GetMetrics(); err == nil {
		monitoring.GPUs = gpuMetrics
	}

	// Get system metrics
	if sysMetrics, err := h.system.GetMetrics(); err == nil {
		monitoring.System = *sysMetrics
	}

	// Get fan status
	fans, _ := h.fans.List(r.Context())
	readings, _ := h.ipmi.GetFanReadings(r.Context())

	for _, fan := range fans {
		status := models.FanStatus{
			ID:             fan.ID,
			IPMISensorID:   fan.IPMISensorID,
			Label:          fan.DisplayName(),
			IPMIZone:       fan.IPMIZone,
			Channel:        fan.Channel,
			ManualOverride: h.controller.HasManualOverride(fan.ID),
		}

		if rd, ok := readings[fan.IPMISensorID]; ok {
			status.CurrentRPM = rd.RPM
			status.CurrentDuty = rd.DutyCycle
		}
		if fan.IPMIZone != nil {
			if t, ok := h.controller.GetZoneTarget(*fan.IPMIZone); ok {
				status.TargetPercent = &t
			}
		}

		monitoring.Fans = append(monitoring.Fans, status)
	}

	writeJSON(w, monitoring)
}

// GetGPUMetrics handles GET /api/monitoring/gpus
func (h *MonitoringHandler) GetGPUMetrics(w http.ResponseWriter, r *http.Request) {
	metrics, err := h.gpu.GetMetrics()
	if err != nil {
		http.Error(w, "Failed to get GPU metrics", http.StatusInternalServerError)
		return
	}

	writeJSON(w, metrics)
}

// GetSystemMetrics handles GET /api/monitoring/system
func (h *MonitoringHandler) GetSystemMetrics(w http.ResponseWriter, r *http.Request) {
	metrics, err := h.system.GetMetrics()
	if err != nil {
		http.Error(w, "Failed to get system metrics", http.StatusInternalServerError)
		return
	}

	writeJSON(w, metrics)
}

// GetFanStatus handles GET /api/monitoring/fans
func (h *MonitoringHandler) GetFanStatus(w http.ResponseWriter, r *http.Request) {
	fans, err := h.fans.List(r.Context())
	if err != nil {
		http.Error(w, "Failed to list fans", http.StatusInternalServerError)
		return
	}

	readings, _ := h.ipmi.GetFanReadings(r.Context())

	var response []models.FanStatus
	for _, fan := range fans {
		status := models.FanStatus{
			ID:             fan.ID,
			IPMISensorID:   fan.IPMISensorID,
			Label:          fan.DisplayName(),
			IPMIZone:       fan.IPMIZone,
			Channel:        fan.Channel,
			ManualOverride: h.controller.HasManualOverride(fan.ID),
		}

		if rd, ok := readings[fan.IPMISensorID]; ok {
			status.CurrentRPM = rd.RPM
			status.CurrentDuty = rd.DutyCycle
		}
		if fan.IPMIZone != nil {
			if t, ok := h.controller.GetZoneTarget(*fan.IPMIZone); ok {
				status.TargetPercent = &t
			}
		}

		response = append(response, status)
	}

	writeJSON(w, response)
}
