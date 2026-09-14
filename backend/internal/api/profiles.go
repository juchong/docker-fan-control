package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"docker-fan-control/internal/models"
	"docker-fan-control/internal/services"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
)

// normalizeProfileRequest trims surrounding whitespace on free-text fields.
// It deliberately does NOT strip/alter content: input types are lowercase enums
// (e.g. "gpu_temp") validated downstream, and GORM parameterizes all queries.
func normalizeProfileRequest(req models.CreateProfileRequest) models.CreateProfileRequest {
	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)
	for i := range req.Inputs {
		req.Inputs[i].InputType = strings.TrimSpace(req.Inputs[i].InputType)
	}
	return req
}

// ProfilesHandler handles profile endpoints
type ProfilesHandler struct {
	profiles   *services.ProfileService
	controller *services.FanController
	logger     *services.EventLogger
}

// NewProfilesHandler creates a new profiles handler
func NewProfilesHandler(profiles *services.ProfileService, controller *services.FanController, logger *services.EventLogger) *ProfilesHandler {
	return &ProfilesHandler{profiles: profiles, controller: controller, logger: logger}
}

// List handles GET /api/profiles
func (h *ProfilesHandler) List(w http.ResponseWriter, r *http.Request) {
	profiles, err := h.profiles.List(r.Context())
	if err != nil {
		http.Error(w, "Failed to list profiles", http.StatusInternalServerError)
		return
	}

	writeJSON(w, profiles)
}

// Get handles GET /api/profiles/{id}
func (h *ProfilesHandler) Get(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		http.Error(w, "Invalid profile ID", http.StatusBadRequest)
		return
	}

	profile, err := h.profiles.Get(r.Context(), uint(id))
	if err != nil {
		if err == services.ErrProfileNotFound {
			http.Error(w, "Profile not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to get profile", http.StatusInternalServerError)
		return
	}

	writeJSON(w, profile)
}

// Create handles POST /api/profiles
func (h *ProfilesHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req models.CreateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Normalize input
	req = normalizeProfileRequest(req)

	if req.Name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}

	// Validate name length
	if len(req.Name) > 64 {
		http.Error(w, "Name must be at most 64 characters", http.StatusBadRequest)
		return
	}

	// Validate description length
	if len(req.Description) > 256 {
		http.Error(w, "Description must be at most 256 characters", http.StatusBadRequest)
		return
	}

	if req.Algorithm == "" {
		req.Algorithm = "linear"
	}

	// Validate algorithm
	validAlgorithms := map[string]bool{"linear": true, "step": true, "pid": true}
	if !validAlgorithms[req.Algorithm] {
		http.Error(w, "Algorithm must be 'linear', 'step', or 'pid'", http.StatusBadRequest)
		return
	}

	if req.AlgorithmParams == nil {
		// Set default params based on algorithm
		switch req.Algorithm {
		case "linear":
			req.AlgorithmParams = models.AlgorithmParams{
				"min_temp":  30.0,
				"max_temp":  80.0,
				"min_speed": 30.0,
				"max_speed": 100.0,
			}
		case "step":
			req.AlgorithmParams = models.AlgorithmParams{
				"steps": []map[string]any{
					{"temp": 30.0, "speed": 30.0},
					{"temp": 50.0, "speed": 50.0},
					{"temp": 70.0, "speed": 75.0},
					{"temp": 80.0, "speed": 100.0},
				},
			}
		case "pid":
			req.AlgorithmParams = models.AlgorithmParams{
				"setpoint":  70.0,
				"kp":        2.0,
				"ki":        0.1,
				"kd":        1.0,
				"min_speed": 30.0,
				"max_speed": 100.0,
			}
		}
	}

	profile, err := h.profiles.Create(r.Context(), &req)
	if err != nil {
		if services.IsValidationError(err) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		log.Error().Err(err).Msg("profile create failed")
		http.Error(w, "Failed to create profile", http.StatusInternalServerError)
		return
	}

	h.logger.Info(models.CategoryProfile, "Profile created", models.JSONMap{
		"profile_id":   profile.ID,
		"profile_name": profile.Name,
		"algorithm":    profile.Algorithm,
	})

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, profile)
}

// Update handles PUT /api/profiles/{id}
func (h *ProfilesHandler) Update(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		http.Error(w, "Invalid profile ID", http.StatusBadRequest)
		return
	}

	var req models.UpdateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Sanitize and validate input
	if req.Name != nil {
		sanitized := strings.TrimSpace(*req.Name)
		if sanitized == "" {
			http.Error(w, "Name cannot be empty", http.StatusBadRequest)
			return
		}
		if len(sanitized) > 64 {
			http.Error(w, "Name must be at most 64 characters", http.StatusBadRequest)
			return
		}
		req.Name = &sanitized
	}
	if req.Description != nil {
		sanitized := strings.TrimSpace(*req.Description)
		if len(sanitized) > 256 {
			http.Error(w, "Description must be at most 256 characters", http.StatusBadRequest)
			return
		}
		req.Description = &sanitized
	}
	if req.Algorithm != nil {
		validAlgorithms := map[string]bool{"linear": true, "step": true, "pid": true}
		if !validAlgorithms[*req.Algorithm] {
			http.Error(w, "Algorithm must be 'linear', 'step', or 'pid'", http.StatusBadRequest)
			return
		}
	}

	profile, err := h.profiles.Update(r.Context(), uint(id), &req)
	if err != nil {
		if err == services.ErrProfileNotFound {
			http.Error(w, "Profile not found", http.StatusNotFound)
			return
		}
		if services.IsValidationError(err) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		log.Error().Err(err).Uint64("profile_id", id).Msg("profile update failed")
		http.Error(w, "Failed to update profile", http.StatusInternalServerError)
		return
	}

	h.logger.Info(models.CategoryProfile, "Profile updated", models.JSONMap{
		"profile_id":   profile.ID,
		"profile_name": profile.Name,
	})

	writeJSON(w, profile)
}

// Delete handles DELETE /api/profiles/{id}
func (h *ProfilesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		http.Error(w, "Invalid profile ID", http.StatusBadRequest)
		return
	}

	// Get profile name for logging
	profile, _ := h.profiles.Get(r.Context(), uint(id))

	// Deactivate if active
	if profile != nil && profile.IsActive {
		h.controller.DeactivateProfile(uint(id))
	}

	if err := h.profiles.Delete(r.Context(), uint(id)); err != nil {
		if err == services.ErrProfileNotFound {
			http.Error(w, "Profile not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to delete profile", http.StatusInternalServerError)
		return
	}

	if profile != nil {
		h.logger.Info(models.CategoryProfile, "Profile deleted", models.JSONMap{
			"profile_id":   id,
			"profile_name": profile.Name,
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

// Activate handles POST /api/profiles/{id}/activate
func (h *ProfilesHandler) Activate(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		http.Error(w, "Invalid profile ID", http.StatusBadRequest)
		return
	}

	profile, err := h.profiles.Get(r.Context(), uint(id))
	if err != nil {
		if err == services.ErrProfileNotFound {
			http.Error(w, "Profile not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to get profile", http.StatusInternalServerError)
		return
	}

	// Activate the profile by setting is_active flag
	h.controller.ActivateProfile(uint(id))

	h.logger.LogProfileActivated(profile.Name, profile.ID)

	writeJSON(w, map[string]any{
		"message":    "Profile activated",
		"profile_id": id,
	})
}

// Deactivate handles POST /api/profiles/{id}/deactivate
func (h *ProfilesHandler) Deactivate(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		http.Error(w, "Invalid profile ID", http.StatusBadRequest)
		return
	}

	// Deactivate the profile by clearing is_active flag
	h.controller.DeactivateProfile(uint(id))

	h.logger.Info(models.CategoryProfile, "Profile deactivated", models.JSONMap{
		"profile_id": id,
	})

	writeJSON(w, map[string]any{
		"message":    "Profile deactivated",
		"profile_id": id,
	})
}
