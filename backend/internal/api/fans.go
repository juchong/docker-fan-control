package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
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

	// Get current readings (RPM + duty) so current_duty is populated over REST
	// too, not only over the WebSocket feed.
	readings, _ := h.ipmi.GetFanReadings(r.Context())

	// Build response with current status
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

// Detect handles POST /api/fans/detect
func (h *FansHandler) Detect(w http.ResponseWriter, r *http.Request) {
	detected, err := h.ipmi.DetectFans(r.Context())
	if err != nil {
		h.logger.LogIPMIError("fan detection", err)
		http.Error(w, "Failed to detect fans", http.StatusInternalServerError)
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

// Update handles PUT /api/fans/{id} - set the label and/or the control zone.
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

	// Normalize and validate label (GORM parameterizes queries; no sanitizer needed)
	if req.Label != nil {
		sanitized := strings.TrimSpace(*req.Label)
		if len(sanitized) > 64 {
			http.Error(w, "Label must be at most 64 characters", http.StatusBadRequest)
			return
		}
		req.Label = &sanitized
	}

	// Validate the requested control zone against the active driver's layout.
	if req.IPMIZone != nil {
		driver := h.ipmi.GetCurrentDriver()
		if driver == nil {
			http.Error(w, "No fan driver active", http.StatusServiceUnavailable)
			return
		}
		valid := false
		for _, z := range driver.GetZoneLayout().Zones {
			if z.ID == *req.IPMIZone {
				valid = true
				break
			}
		}
		if !valid {
			http.Error(w, "Invalid zone for the current driver", http.StatusBadRequest)
			return
		}
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

	h.logger.Info(models.CategoryFan, "Fan label updated", models.JSONMap{
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

	// Run identification in the background with an INDEPENDENT context. Using
	// r.Context() here aborts identification the instant the handler returns.
	zone := *fan.IPMIZone
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), duration+5*time.Second)
		defer cancel()
		if err := h.ipmi.IdentifyFan(ctx, zone, duration); err != nil {
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

	// Validate percentage range
	if req.Percent < 0 || req.Percent > 100 {
		http.Error(w, "Percent must be between 0 and 100", http.StatusBadRequest)
		return
	}

	// Record the manual override (optionally auto-expiring), so the control loop
	// keeps this zone under manual control until it expires or is cleared.
	var dur time.Duration
	if req.DurationSeconds > 0 {
		dur = time.Duration(req.DurationSeconds) * time.Second
	}
	h.controller.SetManualOverride(fan.ID, req.Percent, dur)

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

// ClearSpeed handles DELETE /api/fans/{id}/speed - drops a manual override so the
// fan returns to automatic (profile) control on the next control cycle.
func (h *FansHandler) ClearSpeed(w http.ResponseWriter, r *http.Request) {
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

	h.controller.ClearManualOverride(fan.ID)
	h.logger.Info(models.CategoryFan, "Manual fan override cleared", models.JSONMap{"fan_id": fan.ID})

	writeJSON(w, map[string]any{"message": "Manual override cleared"})
}
