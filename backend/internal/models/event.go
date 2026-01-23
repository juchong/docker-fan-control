package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

// Event represents a logged event
type Event struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Timestamp time.Time `json:"timestamp" gorm:"not null;index"`
	Level     string    `json:"level" gorm:"not null;index"` // "info", "warning", "error"
	Category  string    `json:"category" gorm:"not null;index"` // "fan", "profile", "ipmi", "system", "temp", "auth"
	Message   string    `json:"message" gorm:"not null"`
	Details   JSONMap   `json:"details,omitempty" gorm:"type:json"`
}

// Event levels
const (
	LevelInfo    = "info"
	LevelWarning = "warning"
	LevelError   = "error"
)

// Event categories
const (
	CategoryFan     = "fan"
	CategoryProfile = "profile"
	CategoryIPMI    = "ipmi"
	CategorySystem  = "system"
	CategoryTemp    = "temp"
	CategoryAuth    = "auth"
)

// JSONMap is a generic JSON map type for GORM
type JSONMap map[string]any

// Scan implements sql.Scanner for JSONMap
func (j *JSONMap) Scan(value any) error {
	if value == nil {
		*j = make(JSONMap)
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return errors.New("type assertion to []byte or string failed")
	}
	return json.Unmarshal(bytes, j)
}

// Value implements driver.Valuer for JSONMap
func (j JSONMap) Value() (driver.Value, error) {
	if j == nil {
		return "{}", nil
	}
	return json.Marshal(j)
}

// EventQuery represents query parameters for events
type EventQuery struct {
	Level     string     `json:"level,omitempty"`
	Category  string     `json:"category,omitempty"`
	Search    string     `json:"search,omitempty"`
	StartTime *time.Time `json:"start_time,omitempty"`
	EndTime   *time.Time `json:"end_time,omitempty"`
	Limit     int        `json:"limit,omitempty"`
	Offset    int        `json:"offset,omitempty"`
}

// EventListResponse is the API response for listing events
type EventListResponse struct {
	Events     []Event `json:"events"`
	TotalCount int64   `json:"total_count"`
	Limit      int     `json:"limit"`
	Offset     int     `json:"offset"`
}

