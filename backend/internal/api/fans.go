package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"docker-fan-control/internal/models"
	"docker-fan-control/internal/services"

	"github.com/go-chi/chi/v5"
)

// FansHandler handles fan endpoints
type FansHandler struct {
	fans       *services.FanService
	ipmi       *services.IPMIService
	controller *services.FanController
	logger     *services.EventLogger
}

// NewFansHandler creates a new fans handler
func NewFansHandler(fans *services.FanService, ipmi *services.IPMIService, controller *services.FanController, logger *services.EventLogger) *FansHandler {
	return &FansHandler{fans: fans, ipmi: ipmi, controller: controller, logger: logger}
}

// List handles GET /api/fans
func (h *FansHandler) List(w http.ResponseWriter, r *http.Request) {
	fans, err := h.fans.List(r.Context())
	if err != nil {
		http.Error(w, "Failed to list fans", http.StatusInternalServerError)
		return
	}

	// Get current speeds
	speeds, _ := h.ipmi.GetFanSpeeds(r.Context())

	// Build response with current status
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

		// Get assigned profiles
		if profiles, err := h.fans.GetFanProfiles(r.Context(), fan.ID); err == nil {
			status.AssignedProfiles = profiles
		}

		response = append(response, status)
	}

	writeJSON(w, response)
}

// Detect handles POST /api/fans/detect
func (h *FansHandler) Detect(w http.ResponseWriter, r *http.Request) {
	detected, err := h.ipmi.DetectFans(r.Context())
	if err != nil {
		h.logger.LogIPMIError("fan detection", err)
		http.Error(w, "Failed to detect fans: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Save detected fans to database
	savedFans, err := h.fans.SaveDetectedFans(r.Context(), detected)
	if err != nil {
		http.Error(w, "Failed to save detected fans", http.StatusInternalServerError)
		return
	}

	h.logger.Info(models.CategoryFan, "Fan detection completed", models.JSONMap{
		"count": len(savedFans),
	})

	writeJSON(w, savedFans)
}

// Update handles PUT /api/fans/{id}
func (h *FansHandler) Update(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		http.Error(w, "Invalid fan ID", http.StatusBadRequest)
		return
	}

	var req models.UpdateFanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	fan, err := h.fans.Update(r.Context(), uint(id), &req)
	if err != nil {
		if err == services.ErrFanNotFound {
			http.Error(w, "Fan not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to update fan", http.StatusInternalServerError)
		return
	}

	h.logger.Info(models.CategoryFan, "Fan updated", models.JSONMap{
		"fan_id": fan.ID,
		"label":  fan.Label,
	})

	writeJSON(w, fan)
}

// Identify handles POST /api/fans/{id}/identify
func (h *FansHandler) Identify(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		http.Error(w, "Invalid fan ID", http.StatusBadRequest)
		return
	}

	fan, err := h.fans.Get(r.Context(), uint(id))
	if err != nil {
		if err == services.ErrFanNotFound {
			http.Error(w, "Fan not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to get fan", http.StatusInternalServerError)
		return
	}

	if fan.IPMIZone == nil {
		http.Error(w, "Fan has no assigned zone", http.StatusBadRequest)
		return
	}

	var req models.IdentifyFanRequest
	json.NewDecoder(r.Body).Decode(&req)

	duration := 5 * time.Second
	if req.Duration > 0 && req.Duration <= 30 {
		duration = time.Duration(req.Duration) * time.Second
	}

	h.logger.Info(models.CategoryFan, "Identifying fan", models.JSONMap{
		"fan_id":   fan.ID,
		"zone":     *fan.IPMIZone,
		"duration": duration.String(),
	})

	// Run identification in background
	go func() {
		if err := h.ipmi.IdentifyFan(r.Context(), *fan.IPMIZone, duration); err != nil {
			h.logger.LogIPMIError("fan identification", err)
		}
	}()

	writeJSON(w, map[string]any{
		"message":  "Identification started",
		"duration": duration.Seconds(),
	})
}

// SetSpeed handles POST /api/fans/{id}/speed
func (h *FansHandler) SetSpeed(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		http.Error(w, "Invalid fan ID", http.StatusBadRequest)
		return
	}

	fan, err := h.fans.Get(r.Context(), uint(id))
	if err != nil {
		if err == services.ErrFanNotFound {
			http.Error(w, "Fan not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to get fan", http.StatusInternalServerError)
		return
	}

	if fan.IPMIZone == nil {
		http.Error(w, "Fan has no assigned zone", http.StatusBadRequest)
		return
	}

	var req models.SetFanSpeedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Percent < 0 || req.Percent > 100 {
		http.Error(w, "Percent must be between 0 and 100", http.StatusBadRequest)
		return
	}

	// Set manual override
	h.controller.SetManualOverride(fan.ID, req.Percent)

	// Apply immediately
	if err := h.ipmi.SetFanSpeed(r.Context(), *fan.IPMIZone, req.Percent); err != nil {
		h.logger.LogIPMIError("set fan speed", err)
		http.Error(w, "Failed to set fan speed", http.StatusInternalServerError)
		return
	}

	h.logger.LogFanSpeedChange(fan.DisplayName(), 0, req.Percent, "manual override")

	writeJSON(w, map[string]any{
		"message": "Speed set",
		"percent": req.Percent,
	})
}
