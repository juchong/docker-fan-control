package api

import (
	"context"
	"net/http"
	"strings"

	"docker-fan-control/internal/config"
	"docker-fan-control/internal/models"
	"docker-fan-control/internal/services"
)

type contextKey string

const (
	UserContextKey contextKey = "user"
)

// GetUserFromContext returns the user from request context
func GetUserFromContext(ctx context.Context) *models.User {
	user, ok := ctx.Value(UserContextKey).(*models.User)
	if !ok {
		return nil
	}
	return user
}

// AuthMiddleware checks for valid authentication
func AuthMiddleware(authSvc *services.AuthService, cfg *config.AuthConfig) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var user *models.User
			var err error

			// Method 1: Check proxy auth header (if enabled)
			if cfg.ProxyAuthEnabled {
				if username := r.Header.Get(cfg.ProxyAuthHeader); username != "" {
					user, err = authSvc.GetOrCreateProxyUser(r.Context(), username)
					if err == nil {
						ctx := context.WithValue(r.Context(), UserContextKey, user)
						next.ServeHTTP(w, r.WithContext(ctx))
						return
					}
				}
			}

			// Method 2: Check JWT token from Authorization header
			if token := extractBearerToken(r); token != "" {
				user, err = authSvc.ValidateToken(token)
				if err == nil {
					ctx := context.WithValue(r.Context(), UserContextKey, user)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}

			// Method 3: Check JWT token from cookie
			if cookie, err := r.Cookie("auth_token"); err == nil {
				user, err = authSvc.ValidateToken(cookie.Value)
				if err == nil {
					ctx := context.WithValue(r.Context(), UserContextKey, user)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}

			http.Error(w, "Unauthorized", http.StatusUnauthorized)
		})
	}
}

// RequireRole middleware ensures user has required role
func RequireRole(role string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := GetUserFromContext(r.Context())
			if user == nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			if role == "admin" && !user.IsAdmin() {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// extractBearerToken extracts token from Authorization header
func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}

	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return ""
	}

	return parts[1]
}
