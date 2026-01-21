package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"docker-fan-control/internal/config"
	"docker-fan-control/internal/models"
	"docker-fan-control/internal/services"

	"github.com/go-chi/chi/v5"
)

// AuthHandler handles authentication endpoints
type AuthHandler struct {
	auth   *services.AuthService
	logger *services.EventLogger
	cfg    *config.Config
}

// NewAuthHandler creates a new auth handler
func NewAuthHandler(auth *services.AuthService, logger *services.EventLogger, cfg *config.Config) *AuthHandler {
	return &AuthHandler{auth: auth, logger: logger, cfg: cfg}
}

// Login handles POST /api/auth/login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req models.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	resp, err := h.auth.Login(r.Context(), req.Username, req.Password)
	if err != nil {
		h.logger.LogAuthEvent("login", req.Username, false, models.JSONMap{"error": err.Error()})
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	h.logger.LogAuthEvent("login", req.Username, true, nil)

	// Set cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    resp.Token,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})

	writeJSON(w, resp)
}

// Logout handles POST /api/auth/logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r.Context())
	if user != nil {
		h.logger.LogAuthEvent("logout", user.Username, true, nil)
	}

	// Clear cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})

	writeJSON(w, map[string]string{"message": "Logged out"})
}

// Refresh handles POST /api/auth/refresh
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	token, err := h.auth.RefreshToken(user)
	if err != nil {
		http.Error(w, "Failed to refresh token", http.StatusInternalServerError)
		return
	}

	// Set cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})

	writeJSON(w, map[string]string{"token": token})
}

// Me handles GET /api/auth/me
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	writeJSON(w, user.ToResponse())
}

// ChangePassword handles PUT /api/auth/password
func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	user := GetUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req models.ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.auth.ChangePassword(r.Context(), user.ID, req.CurrentPassword, req.NewPassword); err != nil {
		if err == services.ErrInvalidCredentials {
			http.Error(w, "Current password is incorrect", http.StatusBadRequest)
			return
		}
		http.Error(w, "Failed to change password", http.StatusInternalServerError)
		return
	}

	h.logger.LogAuthEvent("password_change", user.Username, true, nil)
	writeJSON(w, map[string]string{"message": "Password changed"})
}

// Status handles GET /api/auth/status
func (h *AuthHandler) Status(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"auth_enabled":  h.cfg.Auth.Enabled,
		"proxy_enabled": h.cfg.Auth.ProxyAuthEnabled,
	})
}

// ListUsers handles GET /api/users
func (h *AuthHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.auth.ListUsers(r.Context())
	if err != nil {
		http.Error(w, "Failed to list users", http.StatusInternalServerError)
		return
	}

	writeJSON(w, users)
}

// CreateUser handles POST /api/users
func (h *AuthHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req models.CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Username == "" || req.Password == "" {
		http.Error(w, "Username and password are required", http.StatusBadRequest)
		return
	}

	if req.Role == "" {
		req.Role = "user"
	}

	user, err := h.auth.CreateUser(r.Context(), req.Username, req.Password, req.Role)
	if err != nil {
		if err == services.ErrUserExists {
			http.Error(w, "User already exists", http.StatusConflict)
			return
		}
		http.Error(w, "Failed to create user", http.StatusInternalServerError)
		return
	}

	currentUser := GetUserFromContext(r.Context())
	h.logger.LogAuthEvent("user_created", req.Username, true, models.JSONMap{
		"by_user": currentUser.Username,
		"role":    req.Role,
	})

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, user.ToResponse())
}

// UpdateUser handles PUT /api/users/{id}
func (h *AuthHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	var req models.UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	user, err := h.auth.UpdateUser(r.Context(), uint(id), &req)
	if err != nil {
		if err == services.ErrUserNotFound {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to update user", http.StatusInternalServerError)
		return
	}

	currentUser := GetUserFromContext(r.Context())
	h.logger.LogAuthEvent("user_updated", user.Username, true, models.JSONMap{
		"by_user": currentUser.Username,
	})

	writeJSON(w, user.ToResponse())
}

// DeleteUser handles DELETE /api/users/{id}
func (h *AuthHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	// Prevent deleting self
	currentUser := GetUserFromContext(r.Context())
	if currentUser != nil && currentUser.ID == uint(id) {
		http.Error(w, "Cannot delete yourself", http.StatusBadRequest)
		return
	}

	// Get user for logging
	targetUser, err := h.auth.GetUserByID(r.Context(), uint(id))
	if err != nil {
		if err == services.ErrUserNotFound {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to get user", http.StatusInternalServerError)
		return
	}

	if err := h.auth.DeleteUser(r.Context(), uint(id)); err != nil {
		http.Error(w, "Failed to delete user", http.StatusInternalServerError)
		return
	}

	h.logger.LogAuthEvent("user_deleted", targetUser.Username, true, models.JSONMap{
		"by_user": currentUser.Username,
	})

	w.WriteHeader(http.StatusNoContent)
}

// writeJSON writes JSON response
func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}
