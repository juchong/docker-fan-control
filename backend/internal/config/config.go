package config

import (
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// Config holds all application configuration
type Config struct {
	Server   ServerConfig
	IPMI     IPMIConfig
	Auth     AuthConfig
	Data     DataConfig
	LogLevel string // zerolog level name: trace|debug|info|warn|error (LOG_LEVEL)
}

// ServerConfig holds HTTP server settings
type ServerConfig struct {
	Port         int
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	APIBasePath  string
	// TrustedProxies are CIDRs (from TRUSTED_PROXIES) whose X-Forwarded-For /
	// X-Real-IP headers are believed. Requests whose direct peer is outside
	// these ranges use the socket peer IP, so a client cannot spoof its IP to
	// bypass or poison per-IP rate limiting. Empty = trust no forwarding headers.
	TrustedProxies []*net.IPNet
}

// IPMIConfig holds IPMI connection settings
type IPMIConfig struct {
	Mode     string // "local" or "lan"
	Host     string
	User     string
	Password string
}

// AuthConfig holds authentication settings
type AuthConfig struct {
	Enabled            bool
	JWTSecret          []byte
	TokenTTL           time.Duration
	SessionTimeout     time.Duration
	ProxyAuthEnabled   bool
	ProxyAuthHeader    string
	ProxyAutoCreate    bool
	DefaultAdmin       string
	DefaultPassword    string
	ResetAdminPassword bool
}

// DataConfig holds data storage settings
type DataConfig struct {
	Path            string
	ControlInterval time.Duration
}

// Load reads configuration from environment variables
func Load() *Config {
	return &Config{
		Server: ServerConfig{
			Port:         getEnvInt("SERVER_PORT", 8080),
			ReadTimeout:  getEnvDuration("SERVER_READ_TIMEOUT", 30*time.Second),
			WriteTimeout: getEnvDuration("SERVER_WRITE_TIMEOUT", 30*time.Second),
			APIBasePath:    getEnv("API_BASE_PATH", "/api"),
			TrustedProxies: getEnvCIDRs("TRUSTED_PROXIES"),
		},
		IPMI: IPMIConfig{
			Mode:     getEnv("IPMI_MODE", "local"),
			Host:     getEnv("IPMI_HOST", ""),
			User:     getEnv("IPMI_USER", ""),
			Password: getEnv("IPMI_PASS", ""),
		},
		Auth: AuthConfig{
			Enabled:            getEnvBool("AUTH_ENABLED", true),
			JWTSecret:          []byte(getEnv("AUTH_JWT_SECRET", "change-me-in-production")),
			TokenTTL:           getEnvDuration("AUTH_TOKEN_TTL", 24*time.Hour),
			SessionTimeout:     getEnvDuration("AUTH_SESSION_TIMEOUT", 0), // 0 = disabled
			ProxyAuthEnabled:   getEnvBool("AUTH_PROXY_ENABLED", false),
			ProxyAuthHeader:    getEnv("AUTH_PROXY_HEADER", "X-Forwarded-User"),
			ProxyAutoCreate:    getEnvBool("AUTH_PROXY_AUTO_CREATE", true),
			DefaultAdmin:       getEnv("AUTH_DEFAULT_ADMIN", "admin"),
			DefaultPassword:    getEnv("AUTH_DEFAULT_PASSWORD", ""),
			ResetAdminPassword: getEnvBool("AUTH_RESET_ADMIN_PASSWORD", false),
		},
		Data: DataConfig{
			Path:            getEnv("DATA_PATH", "/app/data"),
			ControlInterval: getEnvDuration("CONTROL_INTERVAL", 5*time.Second),
		},
		LogLevel: getEnv("LOG_LEVEL", "info"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if b, err := strconv.ParseBool(value); err == nil {
			return b
		}
	}
	return defaultValue
}

// getEnvCIDRs parses a comma-separated list of CIDRs (bare IPs are accepted as
// /32 or /128). Invalid entries are logged and skipped.
func getEnvCIDRs(key string) []*net.IPNet {
	value := os.Getenv(key)
	if value == "" {
		return nil
	}
	var nets []*net.IPNet
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if !strings.Contains(part, "/") {
			if ip := net.ParseIP(part); ip != nil {
				if ip.To4() != nil {
					part += "/32"
				} else {
					part += "/128"
				}
			}
		}
		if _, ipNet, err := net.ParseCIDR(part); err == nil {
			nets = append(nets, ipNet)
		} else {
			log.Warn().Str("entry", part).Str("env", key).Msg("ignoring invalid CIDR in TRUSTED_PROXIES")
		}
	}
	return nets
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
	}
	return defaultValue
}
