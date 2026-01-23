package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds all application configuration
type Config struct {
	Server ServerConfig
	IPMI   IPMIConfig
	Auth   AuthConfig
	Data   DataConfig
}

// ServerConfig holds HTTP server settings
type ServerConfig struct {
	Port         int
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	APIBasePath  string
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
	TempUnit        string // "C" or "F"
}

// Load reads configuration from environment variables
func Load() *Config {
	return &Config{
		Server: ServerConfig{
			Port:         getEnvInt("SERVER_PORT", 8080),
			ReadTimeout:  getEnvDuration("SERVER_READ_TIMEOUT", 30*time.Second),
			WriteTimeout: getEnvDuration("SERVER_WRITE_TIMEOUT", 30*time.Second),
			APIBasePath:  getEnv("API_BASE_PATH", "/api"),
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
			TempUnit:        getEnv("TEMP_UNIT", "C"),
		},
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

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
	}
	return defaultValue
}
