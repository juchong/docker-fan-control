package api

import (
	"net/http"
	"net/url"
	"os"
	"strings"

	"docker-fan-control/internal/config"
	"docker-fan-control/internal/services"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

// Services holds all service instances
type Services struct {
	Auth       *services.AuthService
	IPMI       *services.IPMIService
	GPU        *services.GPUService
	System     *services.SystemService
	Controller *services.FanController
	Logger     *services.EventLogger
	Profiles   *services.ProfileService
	Fans       *services.FanService
}

// NewRouter creates a new HTTP router
func NewRouter(svc *Services, cfg *config.Config) *chi.Mux {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID)
	// Trusted-proxy-aware client IP (replaces chi middleware.RealIP, which trusts
	// spoofable X-Forwarded-* from any peer). Configure TRUSTED_PROXIES.
	r.Use(RealClientIP(cfg.Server.TrustedProxies))
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))

	// Input validation
	r.Use(ValidateRequest)

	// Add auth service to context
	r.Use(AddAuthServiceToContext(svc.Auth))

	// Rate limiting
	r.Use(RateLimiterMiddleware(svc.Auth, &cfg.Auth))

	// Session timeout
	r.Use(SessionTimeoutMiddleware(svc.Auth, cfg.Auth.SessionTimeout))

	// Optional explicit allow-list (comma-separated absolute origins).
	var allowedOrigins []string
	if v := os.Getenv("ALLOWED_ORIGINS"); v != "" {
		for _, o := range strings.Split(v, ",") {
			if o = strings.TrimSpace(o); o != "" {
				allowedOrigins = append(allowedOrigins, o)
			}
		}
	}

	// CORS - exact-host matching (the previous strings.Contains check accepted
	// e.g. https://host.evil.com because it contains "://host"). Same-origin and
	// localhost are always allowed so the live deployment keeps working.
	r.Use(cors.Handler(cors.Options{
		AllowOriginFunc: func(req *http.Request, origin string) bool {
			// Non-browser clients send no Origin header.
			if origin == "" {
				return true
			}
			u, err := url.Parse(origin)
			if err != nil || u.Host == "" {
				return false
			}
			// Same-origin (exact host[:port] match against the request Host).
			host := req.Host
			if host == "" {
				host = req.Header.Get("Host")
			}
			if u.Host == host {
				return true
			}
			// Localhost for development.
			if hn := u.Hostname(); hn == "localhost" || hn == "127.0.0.1" {
				return true
			}
			// Explicit allow-list.
			for _, allowed := range allowedOrigins {
				if origin == allowed {
					return true
				}
			}
			return false
		},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Forwarded-User"},
		ExposedHeaders:   []string{"Link", "X-RateLimit-Limit", "X-RateLimit-Remaining", "X-RateLimit-Reset"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Create handlers
	authHandler := NewAuthHandler(svc.Auth, svc.Logger, cfg)
	fansHandler := NewFansHandler(svc.Fans, svc.IPMI, svc.Controller, svc.Logger)
	profilesHandler := NewProfilesHandler(svc.Profiles, svc.Controller, svc.Logger)
	monitoringHandler := NewMonitoringHandler(svc.GPU, svc.System, svc.IPMI, svc.Controller, svc.Fans)
	logsHandler := NewLogsHandler(svc.Logger)
	settingsHandler := NewSettingsHandler(svc.IPMI, svc.Controller, svc.Auth, svc.Logger)
	wsHandler := NewWebSocketHandler(svc.GPU, svc.System, svc.IPMI, svc.Controller, svc.Fans)

	apiBasePath := cfg.Server.APIBasePath
	r.Route(apiBasePath, func(r chi.Router) {
		// Public routes (no auth required)
		r.Post("/auth/login", authHandler.Login)
		r.Get("/auth/status", authHandler.Status)

		// Protected routes
		r.Group(func(r chi.Router) {
			if cfg.Auth.Enabled {
				r.Use(AuthMiddleware(svc.Auth, &cfg.Auth))
			}

			// Auth management
			r.Post("/auth/logout", authHandler.Logout)
			r.Post("/auth/refresh", authHandler.Refresh)
			r.Get("/auth/me", authHandler.Me)
			r.Put("/auth/password", authHandler.ChangePassword)

			// Users (admin only)
			r.Route("/users", func(r chi.Router) {
				r.Use(RequireRole("admin"))
				r.Get("/", authHandler.ListUsers)
				r.Post("/", authHandler.CreateUser)
				r.Put("/{id}", authHandler.UpdateUser)
				r.Delete("/{id}", authHandler.DeleteUser)
			})

			// Fans
			r.Get("/fans", fansHandler.List)
			r.Post("/fans/detect", fansHandler.Detect)
			r.Put("/fans/{id}", fansHandler.Update)
			r.Post("/fans/{id}/identify", fansHandler.Identify)
			r.Post("/fans/{id}/speed", fansHandler.SetSpeed)

			// Profiles
			r.Get("/profiles", profilesHandler.List)
			r.Post("/profiles", profilesHandler.Create)
			r.Get("/profiles/{id}", profilesHandler.Get)
			r.Put("/profiles/{id}", profilesHandler.Update)
			r.Delete("/profiles/{id}", profilesHandler.Delete)
			r.Post("/profiles/{id}/activate", profilesHandler.Activate)
			r.Post("/profiles/{id}/deactivate", profilesHandler.Deactivate)

			// Monitoring
			r.Get("/monitoring/metrics", monitoringHandler.GetMetrics)
			r.Get("/monitoring/gpus", monitoringHandler.GetGPUMetrics)
			r.Get("/monitoring/system", monitoringHandler.GetSystemMetrics)
			r.Get("/monitoring/fans", monitoringHandler.GetFanStatus)

			// Logs
			r.Get("/logs", logsHandler.List)
			r.Get("/logs/export", logsHandler.Export)
			r.Delete("/logs", logsHandler.Clear)

			// Settings
			r.Get("/settings", settingsHandler.Get)
			r.Put("/settings", settingsHandler.Update)
			r.Post("/settings/test-ipmi", settingsHandler.TestIPMI)
			r.Post("/settings/detect-motherboard", settingsHandler.DetectMotherboard)
			r.Get("/settings/drivers", settingsHandler.GetAvailableDrivers)

			// Controller
			r.Post("/controller/start", settingsHandler.StartController)
			r.Post("/controller/stop", settingsHandler.StopController)
		})
	})

	// WebSocket for real-time updates (requires authentication if enabled)
	r.Group(func(r chi.Router) {
		if cfg.Auth.Enabled {
			r.Use(AuthMiddleware(svc.Auth, &cfg.Auth))
		}
		r.Get(apiBasePath+"/ws", wsHandler.Handle)
	})

	// Health check (supports both GET and HEAD for wget --spider)
	// Available at both /health (for backwards compatibility) and /api/health
	healthHandler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			w.Write([]byte("OK"))
		}
	}
	r.Get("/health", healthHandler)
	r.Head("/health", healthHandler)
	r.Get(apiBasePath+"/health", healthHandler)
	r.Head(apiBasePath+"/health", healthHandler)

	return r
}
