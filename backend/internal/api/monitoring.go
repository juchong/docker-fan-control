package api

import (
	"net/http"

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
	speeds, _ := h.ipmi.GetFanSpeeds(r.Context())

	for _, fan := range fans {
		status := models.FanStatus{
			ID:             fan.ID,
			IPMISensorID:   fan.IPMISensorID,
			Label:          fan.DisplayName(),
			IPMIZone:       fan.IPMIZone,
			ManualOverride: h.controller.HasManualOverride(fan.ID),
		}

		if rpm, ok := speeds[fan.IPMISensorID]; ok {
			status.CurrentRPM = rpm
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

	speeds, _ := h.ipmi.GetFanSpeeds(r.Context())

	var response []models.FanStatus
	for _, fan := range fans {
		status := models.FanStatus{
			ID:             fan.ID,
			IPMISensorID:   fan.IPMISensorID,
			Label:          fan.DisplayName(),
			IPMIZone:       fan.IPMIZone,
			ManualOverride: h.controller.HasManualOverride(fan.ID),
		}

		if rpm, ok := speeds[fan.IPMISensorID]; ok {
			status.CurrentRPM = rpm
		}

		if profiles, err := h.fans.GetFanProfiles(r.Context(), fan.ID); err == nil {
			status.AssignedProfiles = profiles
		}

		response = append(response, status)
	}

	writeJSON(w, response)
}
