package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"docker-fan-control/internal/api"
	"docker-fan-control/internal/config"
	"docker-fan-control/internal/database"
	"docker-fan-control/internal/models"
	"docker-fan-control/internal/services"
	"docker-fan-control/internal/services/drivers"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	// Setup logging
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	log.Info().Msg("Starting Fan Control Server")

	// Load configuration
	cfg := config.Load()

	// Apply the configured log level (LOG_LEVEL); default info.
	if lvl, err := zerolog.ParseLevel(cfg.LogLevel); err == nil {
		zerolog.SetGlobalLevel(lvl)
	} else {
		log.Warn().Str("log_level", cfg.LogLevel).Msg("invalid LOG_LEVEL; defaulting to info")
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	// Refuse to start with the default JWT secret when auth is enabled: a known
	// secret lets anyone forge valid tokens. Set a strong AUTH_JWT_SECRET.
	if cfg.Auth.Enabled && string(cfg.Auth.JWTSecret) == "change-me-in-production" {
		log.Fatal().Msg("AUTH_JWT_SECRET is unset or left at the default; refusing to start with auth enabled")
	}

	// Initialize database
	if err := database.Init(cfg.Data.Path); err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize database")
	}
	defer database.Close()

	// Initialize services
	logger := services.NewEventLogger()
	authSvc := services.NewAuthService(&cfg.Auth)
	ipmiSvc := services.NewIPMIService(cfg.IPMI.Mode, cfg.IPMI.Host, cfg.IPMI.User, cfg.IPMI.Password)
	gpuSvc := services.NewGPUService()
	systemSvc := services.NewSystemService()
	profileSvc := services.NewProfileService(logger, ipmiSvc.GetDriverRegistry())
	fanSvc := services.NewFanService(logger)
	controller := services.NewFanController(ipmiSvc, gpuSvc, systemSvc, logger)

	// Load saved IPMI command format setting
	if settings, err := database.GetAllSettings(); err == nil && settings.IPMICommandFormat != "" {
		format := services.ParseIPMIFormat(settings.IPMICommandFormat)
		if format != services.FormatUnknown {
			ipmiSvc.SetCommandFormat(format)
		}
	}

	// Initialize driver system - register all available drivers.
	// Hwmon (direct sysfs PWM, no BMC) is registered first and preferred; it is
	// the path for BMC-less workstation boards (Nuvoton nct6xxx via nct6775,
	// ITE it8xxx via it87 — every controllable chip is bound). The IPMI drivers
	// remain as fallbacks for BMC-equipped hosts. HWMON_CHIP optionally
	// restricts binding to a comma-separated list of hwmon name prefixes.
	ipmiSvc.RegisterDriver(drivers.NewHwmonDriver(os.Getenv("HWMON_CHIP")))
	ipmiSvc.RegisterDriver(drivers.NewASRockDriver(ipmiSvc))
	ipmiSvc.RegisterDriver(drivers.NewDellDriver(ipmiSvc))
	ipmiSvc.RegisterDriver(drivers.NewSupermicroDriver(ipmiSvc))
	ipmiSvc.RegisterDriver(drivers.NewGenericDriver(ipmiSvc))
	
	// Try to detect motherboard and set driver
	detectCtx, detectCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := ipmiSvc.DetectAndSetDriver(detectCtx); err != nil {
		log.Warn().Err(err).Msg("Failed to auto-detect motherboard driver")
	} else {
		driver := ipmiSvc.GetCurrentDriver()
		if driver != nil {
			log.Info().Str("vendor", driver.GetVendor()).Str("model", driver.GetModel()).Msg("Auto-detected motherboard")
			// Cache the detected motherboard info to database
			database.SetSetting(models.SettingMotherboardVendor, driver.GetVendor())
			database.SetSetting(models.SettingMotherboardModel, driver.GetModel())
			database.SetSetting(models.SettingMotherboardDriver, driver.GetVendor())
		}
	}
	detectCancel()

	// Log system info
	systemSvc.LogSystemInfo()
	log.Info().Int("gpu_count", gpuSvc.GetDeviceCount()).Msg("GPU monitoring initialized")

	// Ensure default admin user
	if err := authSvc.EnsureDefaultAdmin(cfg.Auth.DefaultAdmin, cfg.Auth.DefaultPassword, cfg.Auth.ResetAdminPassword); err != nil {
		log.Warn().Err(err).Msg("Failed to create default admin user")
	} else if cfg.Auth.ResetAdminPassword {
		log.Info().Str("username", cfg.Auth.DefaultAdmin).Msg("Admin password reset from AUTH_DEFAULT_PASSWORD")
	}

	// Create services container
	svc := &api.Services{
		Auth:       authSvc,
		IPMI:       ipmiSvc,
		GPU:        gpuSvc,
		System:     systemSvc,
		Controller: controller,
		Logger:     logger,
		Profiles:   profileSvc,
		Fans:       fanSvc,
	}

	// Create router
	router := api.NewRouter(svc, cfg)

	// Create server
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// Start fan controller
	controllerCtx, controllerCancel := context.WithCancel(context.Background())
	defer controllerCancel()

	if err := controller.Start(controllerCtx); err != nil {
		log.Warn().Err(err).Msg("Failed to start fan controller")
	}

	// Start server in goroutine
	go func() {
		log.Info().Int("port", cfg.Server.Port).Msg("HTTP server starting")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("HTTP server failed")
		}
	}()

	logger.LogSystemEvent("Fan control server started", map[string]any{
		"port":         cfg.Server.Port,
		"auth_enabled": cfg.Auth.Enabled,
		"ipmi_mode":    cfg.IPMI.Mode,
	})

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("Shutting down server...")

	// Stop controller with safety setting
	safetyOnShutdown := true
	if settings, err := database.GetAllSettings(); err == nil {
		safetyOnShutdown = settings.SafetyOnShutdown
	}
	controller.StopWithSafety(safetyOnShutdown)

	// Shutdown server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("Server shutdown failed")
	}

	// Cleanup
	gpuSvc.Close()

	log.Info().Msg("Server stopped")
}
