package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"docker-fan-control/internal/config"
	"docker-fan-control/internal/models"
	"docker-fan-control/internal/services"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/ulule/limiter/v3"
	"github.com/ulule/limiter/v3/drivers/store/memory"
)

// ValidationError represents validation error details
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationErrors is a collection of validation errors
type ValidationErrors struct {
	Errors []ValidationError `json:"errors"`
}

func (e *ValidationErrors) Error() string {
	return fmt.Sprintf("validation failed: %d errors", len(e.Errors))
}

// MaxRequestBodySize is the maximum allowed request body size (1MB)
const MaxRequestBodySize = 1 << 20 // 1 MB

// ValidateRequest validates HTTP request
func ValidateRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Limit request body size to prevent DoS
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, MaxRequestBodySize)
		}

		// Validate query parameters
		if err := validateQueryParams(r); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Validate path parameters
		if err := validatePathParams(r); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// validateQueryParams validates query parameters
func validateQueryParams(r *http.Request) error {
	var errors []ValidationError

	queries := r.URL.Query()
	for key := range queries {
		values := queries[key]
		if len(values) == 0 {
			continue
		}

		// Validate numeric query parameters
		if isNumericParam(key) {
			for _, value := range values {
				if _, err := strconv.Atoi(value); err != nil {
					errors = append(errors, ValidationError{
						Field:   key,
						Message: "must be a valid integer",
					})
				}
			}
		}

		// Validate positive integers
		if isPositiveParam(key) {
			for _, value := range values {
				if num, err := strconv.Atoi(value); err == nil {
					if num <= 0 {
						errors = append(errors, ValidationError{
							Field:   key,
							Message: "must be a positive integer",
						})
					}
				}
			}
		}

		// Validate percentage values (0-100)
		if isPercentageParam(key) {
			for _, value := range values {
				if num, err := strconv.Atoi(value); err == nil {
					if num < 0 || num > 100 {
						errors = append(errors, ValidationError{
							Field:   key,
							Message: "must be between 0 and 100",
						})
					}
				}
			}
		}

		// Validate string length
		if isStringParam(key) {
			for _, value := range values {
				if len(value) > 255 {
					errors = append(errors, ValidationError{
						Field:   key,
						Message: "must be less than 255 characters",
					})
				}
			}
		}
	}

	if len(errors) > 0 {
		return &ValidationErrors{Errors: errors}
	}

	return nil
}

// validatePathParams validates path parameters
func validatePathParams(r *http.Request) error {
	var errors []ValidationError

	// Validate numeric path parameters
	if id := chi.URLParam(r, "id"); id != "" {
		if _, err := strconv.Atoi(id); err != nil {
			errors = append(errors, ValidationError{
				Field:   "id",
				Message: "must be a valid integer",
			})
		}
	}

	if len(errors) > 0 {
		return &ValidationErrors{Errors: errors}
	}

	return nil
}

// isNumericParam checks if parameter should be numeric
func isNumericParam(param string) bool {
	switch param {
	case "id", "limit", "offset", "page", "size":
		return true
	}
	return false
}

// isPositiveParam checks if parameter should be positive integer
func isPositiveParam(param string) bool {
	switch param {
	case "limit", "page", "size":
		return true
	}
	return false
}

// isPercentageParam checks if parameter should be percentage (0-100)
func isPercentageParam(param string) bool {
	switch param {
	case "percent", "speed":
		return true
	}
	return false
}

// isStringParam checks if parameter should be validated for length
func isStringParam(param string) bool {
	switch param {
	case "search", "query", "filter":
		return true
	}
	return false
}

// RateLimiterMiddleware creates rate limiting middleware
// RealClientIP sets r.RemoteAddr to the true client IP, trusting the
// X-Forwarded-For / X-Real-IP headers ONLY when the direct connection comes
// from a configured trusted proxy (e.g. the reverse proxy in front of this
// service). This replaces chi/middleware.RealIP, which trusts those headers
// unconditionally and lets any client spoof its IP to bypass or poison per-IP
// rate limiting. With no trusted proxies configured, the socket peer is used.
func RealClientIP(trusted []*net.IPNet) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if ipInNets(hostOnly(r.RemoteAddr), trusted) {
				if xrip := strings.TrimSpace(r.Header.Get("X-Real-IP")); net.ParseIP(xrip) != nil {
					r.RemoteAddr = net.JoinHostPort(xrip, "0")
				} else if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
					first := strings.TrimSpace(strings.Split(xff, ",")[0]) // originating client
					if net.ParseIP(first) != nil {
						r.RemoteAddr = net.JoinHostPort(first, "0")
					}
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// hostOnly strips the port from a host:port address, tolerating a bare host.
func hostOnly(remoteAddr string) string {
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		return host
	}
	return remoteAddr
}

// ipInNets reports whether ipStr is inside any of the given networks.
func ipInNets(ipStr string, nets []*net.IPNet) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	for _, n := range nets {
		if n != nil && n.Contains(ip) {
			return true
		}
	}
	return false
}

func RateLimiterMiddleware(authSvc *services.AuthService, authCfg *config.AuthConfig) func(next http.Handler) http.Handler {
	store := memory.NewStore()
	
	// Strict rate limit for login attempts (per IP)
	loginLimiter := limiter.New(store, limiter.Rate{
		Period: 1 * time.Minute,
		Limit:  10, // 10 login attempts per minute per IP
	})
	
	// Rate limit for sensitive operations (per user)
	sensitiveLimiter := limiter.New(store, limiter.Rate{
		Period: 1 * time.Minute,
		Limit:  100, // 100 requests per minute per user
	})
	
	// General rate limit for unauthenticated requests (per IP)
	generalLimiter := limiter.New(store, limiter.Rate{
		Period: 1 * time.Minute,
		Limit:  60, // 60 requests per minute per IP
	})
	
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip rate limiting for health checks
			if r.URL.Path == "/health" || r.URL.Path == "/api/health" {
				next.ServeHTTP(w, r)
				return
			}
			
			// Client IP for rate limiting. RemoteAddr has already been resolved
			// to the real client by RealClientIP (trusted-proxy aware), so we do
			// NOT read spoofable X-Forwarded-* headers directly here.
			clientIP := hostOnly(r.RemoteAddr)
			
			// Strict rate limiting for login endpoint
			if r.URL.Path == "/api/auth/login" && r.Method == "POST" {
				limiterCtx, err := loginLimiter.Get(r.Context(), fmt.Sprintf("login:%s", clientIP))
				if err != nil || limiterCtx.Reached {
					w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", limiterCtx.Limit))
					w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", limiterCtx.Remaining))
					w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", limiterCtx.Reset))
					http.Error(w, "Too many login attempts. Please try again later.", http.StatusTooManyRequests)
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			
			// Check if user is authenticated
			user, _ := extractUserFromRequest(r, authSvc, authCfg)
			
			if user != nil {
				// Apply rate limit for sensitive operations (authenticated users)
				if isSensitiveOperation(r) {
					context, err := sensitiveLimiter.Get(r.Context(), fmt.Sprintf("sensitive:%s", user.Username))
					if err != nil || context.Reached {
						http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
						return
					}
				}
			} else {
				// Apply general rate limit for unauthenticated requests
				context, err := generalLimiter.Get(r.Context(), fmt.Sprintf("general:%s", clientIP))
				if err != nil || context.Reached {
					http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
					return
				}
			}
			
			next.ServeHTTP(w, r)
		})
	}
}

// extractUserFromRequest extracts user from request (used for rate-limit
// identity). Proxy-header auth is honored ONLY when explicitly enabled, so a
// forged X-Forwarded-User cannot mint a rate-limit identity (or a DB user).
func extractUserFromRequest(r *http.Request, authSvc *services.AuthService, authCfg *config.AuthConfig) (*models.User, error) {
	// Check for JWT token
	token := extractBearerToken(r)
	if token != "" {
		return authSvc.ValidateToken(token)
	}

	// Check for proxy authentication (only when configured).
	if authCfg != nil && authCfg.ProxyAuthEnabled {
		if proxyUser := r.Header.Get(authCfg.ProxyAuthHeader); proxyUser != "" {
			return authSvc.GetOrCreateProxyUser(r.Context(), proxyUser, authCfg.ProxyAutoCreate)
		}
	}

	return nil, fmt.Errorf("unauthenticated")
}

// isSensitiveOperation checks if request is a sensitive operation
func isSensitiveOperation(r *http.Request) bool {
	// Prefix matches for fixed paths.
	sensitivePrefixes := []string{
		"/api/auth/login",
		"/api/auth/password",
		"/api/auth/refresh",
		"/api/users",
		"/api/fans/detect",
		"/api/controller/start",
		"/api/controller/stop",
	}
	for _, path := range sensitivePrefixes {
		if strings.HasPrefix(r.URL.Path, path) {
			return true
		}
	}

	// Per-fan mutating actions carry an {id} segment, so match by suffix
	// (the literal "/api/fans/{id}/speed" never matched a real path like
	// "/api/fans/5/speed").
	if strings.HasPrefix(r.URL.Path, "/api/fans/") &&
		(strings.HasSuffix(r.URL.Path, "/speed") || strings.HasSuffix(r.URL.Path, "/identify")) {
		return true
	}

	return false
}

// extractBearerToken extracts token from Authorization header
func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return ""
	}
	
	return parts[1]
}

// SessionTimeoutMiddleware adds session timeout warning headers when token is about to expire
func SessionTimeoutMiddleware(authSvc *services.AuthService, sessionTimeout time.Duration) func(next http.Handler) http.Handler {
	if sessionTimeout <= 0 {
		// Session timeout disabled
		return func(next http.Handler) http.Handler {
			return next
		}
	}
	
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check for JWT token - if not present, just pass through
			// The actual auth middleware will handle authentication requirements
			token := extractBearerToken(r)
			if token == "" {
				next.ServeHTTP(w, r)
				return
			}
			
			// Validate token and check expiration
			claims, err := parseTokenClaims(token, authSvc)
			if err != nil || claims == nil {
				// Invalid token - pass through, let auth middleware handle it
				next.ServeHTTP(w, r)
				return
			}
			
			// Check if token is about to expire
			expirationTime := claims.ExpiresAt.Time
			remainingTime := expirationTime.Sub(time.Now())
			
			// If token is about to expire, add warning header
			if remainingTime < sessionTimeout {
				w.Header().Set("X-Session-Timeout", fmt.Sprintf("%d", int(remainingTime.Seconds())))
				w.Header().Set("X-Session-Timeout-Threshold", fmt.Sprintf("%d", int(sessionTimeout.Seconds())))
			}
			
			next.ServeHTTP(w, r)
		})
	}
}

// parseTokenClaims parses JWT token claims
func parseTokenClaims(tokenStr string, authSvc *services.AuthService) (*services.JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &services.JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		return authSvc.GetJWTSecret(), nil
	})
	
	if err != nil {
		return nil, err
	}
	
	claims, ok := token.Claims.(*services.JWTClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	
	return claims, nil
}

// GetUserFromContext extracts user from context
func GetUserFromContext(ctx context.Context) *models.User {
	if val := ctx.Value(contextUserKey{}); val != nil {
		if user, ok := val.(*models.User); ok {
			return user
		}
	}
	return nil
}

// contextUserKey is a key for user in context
type contextUserKey struct{}

// AuthMiddleware checks for valid authentication
// WSTokenFromQuery lets a WebSocket client authenticate with a ?token= query
// parameter, since browsers cannot set an Authorization header on a WebSocket.
// When no Authorization header is present it copies the query token into one so
// AuthMiddleware validates it normally. Scoped to the WS route only, so tokens
// in URLs stay off the general API.
func WSTokenFromQuery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			if tok := r.URL.Query().Get("token"); tok != "" {
				r.Header.Set("Authorization", "Bearer "+tok)
			}
		}
		next.ServeHTTP(w, r)
	})
}

func AuthMiddleware(authSvc *services.AuthService, cfg *config.AuthConfig) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var user *models.User
			var err error

			// Method 1: Check proxy auth header (if enabled)
			if cfg.ProxyAuthEnabled {
				if username := r.Header.Get(cfg.ProxyAuthHeader); username != "" {
					user, err = authSvc.GetOrCreateProxyUser(r.Context(), username, cfg.ProxyAutoCreate)
					if err == nil {
						ctx := context.WithValue(r.Context(), contextUserKey{}, user)
						next.ServeHTTP(w, r.WithContext(ctx))
						return
					}
				}
			}

			// Method 2: Check JWT token from Authorization header
			if token := extractBearerToken(r); token != "" {
				user, err = authSvc.ValidateToken(token)
				if err == nil {
					ctx := context.WithValue(r.Context(), contextUserKey{}, user)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}

			// Method 3: Check JWT token from cookie
			if cookie, err := r.Cookie("auth_token"); err == nil {
				user, err = authSvc.ValidateToken(cookie.Value)
				if err == nil {
					ctx := context.WithValue(r.Context(), contextUserKey{}, user)
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

// AddAuthServiceToContext adds auth service to context
func AddAuthServiceToContext(authSvc *services.AuthService) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), contextAuthServiceKey{}, authSvc)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// contextAuthServiceKey is a key for auth service in context
type contextAuthServiceKey struct{}

// GetAuthServiceFromContext gets auth service from context
func GetAuthServiceFromContext(ctx context.Context) *services.AuthService {
	if val, ok := ctx.Value(contextAuthServiceKey{}).(*services.AuthService); ok {
		return val
	}
	return nil
}

