package database

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"docker-fan-control/internal/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DB is the global database connection
var DB *gorm.DB

// Init initializes the database connection
func Init(dataPath string) error {
	// Ensure data directory exists
	if err := os.MkdirAll(dataPath, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	dbPath := filepath.Join(dataPath, "fan-control.db")

	var err error
	DB, err = gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	// Auto migrate schemas
	if err := DB.AutoMigrate(
		&models.User{},
		&models.Fan{},
		&models.Profile{},
		&models.ProfileInput{},
		&models.Event{},
		&models.Setting{},
	); err != nil {
		return fmt.Errorf("failed to migrate database: %w", err)
	}

	// Initialize default settings if not exists
	if err := initDefaultSettings(); err != nil {
		return fmt.Errorf("failed to initialize default settings: %w", err)
	}

	return nil
}

// initDefaultSettings creates default settings if they don't exist
func initDefaultSettings() error {
	defaults := map[string]any{
		models.SettingIPMIMode:          "local",
		models.SettingIPMICommandFormat: models.IPMIFormatAuto,
		models.SettingControlInterval:   5,
		models.SettingTempUnit:          "C",
		models.SettingStartupMode:       "resume",
		models.SettingStartupPercent:    50,
		models.SettingEmergencyTemp:     90,
		models.SettingEmergencySpeed:    100,
		models.SettingWarningTemp:       70,
		models.SettingWarningEnabled:    true,
		models.SettingSafetyOnShutdown:  true,
	}

	for key, value := range defaults {
		var existing models.Setting
		result := DB.First(&existing, "key = ?", key)
		if result.Error == gorm.ErrRecordNotFound {
			setting := models.Setting{
				Key:   key,
				Value: models.SettingVal{Data: value},
			}
			if err := DB.Create(&setting).Error; err != nil {
				return err
			}
		}
	}

	return nil
}

// GetSetting retrieves a setting value
func GetSetting(key string) (any, error) {
	var setting models.Setting
	if err := DB.First(&setting, "key = ?", key).Error; err != nil {
		return nil, err
	}
	return setting.Value.Data, nil
}

// SetSetting sets a setting value
func SetSetting(key string, value any) error {
	setting := models.Setting{
		Key:   key,
		Value: models.SettingVal{Data: value},
	}
	return DB.Save(&setting).Error
}

// GetAllSettings retrieves all settings as AppSettings
func GetAllSettings() (*models.AppSettings, error) {
	var settings []models.Setting
	if err := DB.Find(&settings).Error; err != nil {
		return nil, err
	}

	result := &models.AppSettings{
		TempUnit:          "C",
		StartupMode:       "resume",
		IPMICommandFormat: models.IPMIFormatAuto,
		EmergencyTemp:     90,
		EmergencySpeed:    100,
		WarningTemp:       70,
		WarningEnabled:    true,
		SafetyOnShutdown:  true,
	}

	for _, s := range settings {
		switch s.Key {
		case models.SettingIPMIMode:
			if v, ok := s.Value.Data.(string); ok {
				result.IPMIMode = v
			}
		case models.SettingIPMIHost:
			if v, ok := s.Value.Data.(string); ok {
				result.IPMIHost = v
			}
		case models.SettingIPMIUser:
			if v, ok := s.Value.Data.(string); ok {
				result.IPMIUser = v
			}
		case models.SettingIPMICommandFormat:
			if v, ok := s.Value.Data.(string); ok {
				result.IPMICommandFormat = v
			}
		case models.SettingControlInterval:
			if v, ok := s.Value.Data.(float64); ok {
				result.ControlInterval = int(v)
			}
		case models.SettingTempUnit:
			if v, ok := s.Value.Data.(string); ok {
				result.TempUnit = v
			}
		case models.SettingStartupMode:
			if v, ok := s.Value.Data.(string); ok {
				result.StartupMode = v
			}
		case models.SettingStartupPercent:
			if v, ok := s.Value.Data.(float64); ok {
				result.StartupPercent = int(v)
			}
		case models.SettingEmergencyTemp:
			if v, ok := s.Value.Data.(float64); ok {
				result.EmergencyTemp = int(v)
			}
		case models.SettingEmergencySpeed:
			if v, ok := s.Value.Data.(float64); ok {
				result.EmergencySpeed = int(v)
			}
		case models.SettingWarningTemp:
			if v, ok := s.Value.Data.(float64); ok {
				result.WarningTemp = int(v)
			}
		case models.SettingWarningEnabled:
			if v, ok := s.Value.Data.(bool); ok {
				result.WarningEnabled = v
			}
		case models.SettingSafetyOnShutdown:
			if v, ok := s.Value.Data.(bool); ok {
				result.SafetyOnShutdown = v
			}
		case models.SettingMotherboardVendor:
			if v, ok := s.Value.Data.(string); ok {
				result.MotherboardVendor = v
			}
		case models.SettingMotherboardModel:
			if v, ok := s.Value.Data.(string); ok {
				result.MotherboardModel = v
			}
		case models.SettingMotherboardDriver:
			if v, ok := s.Value.Data.(string); ok {
				result.MotherboardDriver = v
			}
		case models.SettingZoneLayout:
			if v, ok := s.Value.Data.(string); ok {
				var zoneLayout models.ZoneLayout
				if err := json.Unmarshal([]byte(v), &zoneLayout); err == nil {
					result.ZoneLayout = &zoneLayout
				}
			} else if v, ok := s.Value.Data.(map[string]interface{}); ok {
				// Handle if stored as JSON object directly
				jsonData, _ := json.Marshal(v)
				var zoneLayout models.ZoneLayout
				if err := json.Unmarshal(jsonData, &zoneLayout); err == nil {
					result.ZoneLayout = &zoneLayout
				}
			} else if v, ok := s.Value.Data.([]byte); ok {
				// Handle if stored as raw bytes
				var zoneLayout models.ZoneLayout
				if err := json.Unmarshal(v, &zoneLayout); err == nil {
					result.ZoneLayout = &zoneLayout
				}
			}
		}
	}

	return result, nil
}

// Close closes the database connection
func Close() error {
	sqlDB, err := DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
