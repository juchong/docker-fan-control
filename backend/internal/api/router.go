package api

import (
	"net/http"

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
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))

	// CORS
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Forwarded-User"},
		ExposedHeaders:   []string{"Link"},
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

	r.Route("/api", func(r chi.Router) {
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

	// WebSocket for real-time updates
	r.Get("/ws", wsHandler.Handle)

	// Health check (supports both GET and HEAD for wget --spider)
	healthHandler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			w.Write([]byte("OK"))
		}
	}
	r.Get("/health", healthHandler)
	r.Head("/health", healthHandler)

	return r
}
