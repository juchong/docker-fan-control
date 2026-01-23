package services

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"docker-fan-control/internal/database"
	"docker-fan-control/internal/models"

	"github.com/rs/zerolog/log"
)

// EventLogger handles event logging
type EventLogger struct{}

// NewEventLogger creates a new event logger
func NewEventLogger() *EventLogger {
	return &EventLogger{}
}

// Log creates a new event
func (l *EventLogger) Log(level, category, message string, details models.JSONMap) error {
	event := models.Event{
		Timestamp: time.Now(),
		Level:     level,
		Category:  category,
		Message:   message,
		Details:   details,
	}

	if err := database.DB.Create(&event).Error; err != nil {
		log.Error().Err(err).Str("message", message).Msg("Failed to log event")
		return err
	}

	// Also log to zerolog
	switch level {
	case models.LevelError:
		log.Error().Str("category", category).Any("details", details).Msg(message)
	case models.LevelWarning:
		log.Warn().Str("category", category).Any("details", details).Msg(message)
	default:
		log.Info().Str("category", category).Any("details", details).Msg(message)
	}

	return nil
}

// Info logs an info event
func (l *EventLogger) Info(category, message string, details models.JSONMap) {
	l.Log(models.LevelInfo, category, message, details)
}

// Warn logs a warning event
func (l *EventLogger) Warn(category, message string, details models.JSONMap) {
	l.Log(models.LevelWarning, category, message, details)
}

// Error logs an error event
func (l *EventLogger) Error(category, message string, details models.JSONMap) {
	l.Log(models.LevelError, category, message, details)
}

// Query retrieves events based on query parameters
func (l *EventLogger) Query(ctx context.Context, q *models.EventQuery) (*models.EventListResponse, error) {
	query := database.DB.Model(&models.Event{})

	if q.Level != "" {
		query = query.Where("level = ?", q.Level)
	}
	if q.Category != "" {
		query = query.Where("category = ?", q.Category)
	}
	if q.Search != "" {
		query = query.Where("message LIKE ?", "%"+q.Search+"%")
	}
	if q.StartTime != nil {
		query = query.Where("timestamp >= ?", q.StartTime)
	}
	if q.EndTime != nil {
		query = query.Where("timestamp <= ?", q.EndTime)
	}

	// Get total count
	var totalCount int64
	if err := query.Count(&totalCount).Error; err != nil {
		return nil, err
	}

	// Apply pagination
	limit := q.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}
	offset := q.Offset
	if offset < 0 {
		offset = 0
	}

	var events []models.Event
	if err := query.Order("timestamp DESC").Limit(limit).Offset(offset).Find(&events).Error; err != nil {
		return nil, err
	}

	return &models.EventListResponse{
		Events:     events,
		TotalCount: totalCount,
		Limit:      limit,
		Offset:     offset,
	}, nil
}

// MaxExportRows is the maximum number of rows that can be exported at once
const MaxExportRows = 10000

// ExportCSV exports events to CSV format
func (l *EventLogger) ExportCSV(ctx context.Context, q *models.EventQuery, w io.Writer) error {
	// Limit export to prevent memory exhaustion
	q.Limit = MaxExportRows
	q.Offset = 0

	query := database.DB.Model(&models.Event{})

	if q.Level != "" {
		query = query.Where("level = ?", q.Level)
	}
	if q.Category != "" {
		query = query.Where("category = ?", q.Category)
	}
	if q.Search != "" {
		query = query.Where("message LIKE ?", "%"+q.Search+"%")
	}
	if q.StartTime != nil {
		query = query.Where("timestamp >= ?", q.StartTime)
	}
	if q.EndTime != nil {
		query = query.Where("timestamp <= ?", q.EndTime)
	}

	var events []models.Event
	if err := query.Order("timestamp DESC").Find(&events).Error; err != nil {
		return err
	}

	csvWriter := csv.NewWriter(w)
	defer csvWriter.Flush()

	// Write header
	if err := csvWriter.Write([]string{"Timestamp", "Level", "Category", "Message", "Details"}); err != nil {
		return err
	}

	// Write events
	for _, event := range events {
		details := ""
		if event.Details != nil {
			if b, err := json.Marshal(event.Details); err == nil {
				details = string(b)
			}
		}

		if err := csvWriter.Write([]string{
			event.Timestamp.Format(time.RFC3339),
			event.Level,
			event.Category,
			event.Message,
			details,
		}); err != nil {
			return err
		}
	}

	return nil
}

// ExportJSON exports events to JSON format
func (l *EventLogger) ExportJSON(ctx context.Context, q *models.EventQuery, w io.Writer) error {
	// Limit export to prevent memory exhaustion
	q.Limit = MaxExportRows
	q.Offset = 0

	query := database.DB.Model(&models.Event{})

	if q.Level != "" {
		query = query.Where("level = ?", q.Level)
	}
	if q.Category != "" {
		query = query.Where("category = ?", q.Category)
	}
	if q.Search != "" {
		query = query.Where("message LIKE ?", "%"+q.Search+"%")
	}
	if q.StartTime != nil {
		query = query.Where("timestamp >= ?", q.StartTime)
	}
	if q.EndTime != nil {
		query = query.Where("timestamp <= ?", q.EndTime)
	}

	var events []models.Event
	if err := query.Order("timestamp DESC").Find(&events).Error; err != nil {
		return err
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(events)
}

// Clear deletes all events
func (l *EventLogger) Clear(ctx context.Context) error {
	return database.DB.Exec("DELETE FROM events").Error
}

// ClearOld deletes events older than specified duration
func (l *EventLogger) ClearOld(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)
	result := database.DB.Where("timestamp < ?", cutoff).Delete(&models.Event{})
	return result.RowsAffected, result.Error
}

// GetRecent returns the most recent events
func (l *EventLogger) GetRecent(ctx context.Context, limit int) ([]models.Event, error) {
	var events []models.Event
	err := database.DB.Order("timestamp DESC").Limit(limit).Find(&events).Error
	return events, err
}

// LogFanSpeedChange logs a fan speed change
func (l *EventLogger) LogFanSpeedChange(fanLabel string, oldSpeed, newSpeed int, reason string) {
	l.Info(models.CategoryFan, fmt.Sprintf("Fan '%s' speed changed from %d%% to %d%%", fanLabel, oldSpeed, newSpeed), models.JSONMap{
		"fan":       fanLabel,
		"old_speed": oldSpeed,
		"new_speed": newSpeed,
		"reason":    reason,
	})
}

// LogProfileActivated logs a profile activation
func (l *EventLogger) LogProfileActivated(profileName string, profileID uint) {
	l.Info(models.CategoryProfile, fmt.Sprintf("Profile '%s' activated", profileName), models.JSONMap{
		"profile_id":   profileID,
		"profile_name": profileName,
	})
}

// LogTemperatureWarning logs a temperature warning
func (l *EventLogger) LogTemperatureWarning(source string, temp float64, threshold float64) {
	l.Warn(models.CategoryTemp, fmt.Sprintf("%s temperature %.1f°C exceeded threshold %.1f°C", source, temp, threshold), models.JSONMap{
		"source":    source,
		"temp":      temp,
		"threshold": threshold,
	})
}

// LogIPMIError logs an IPMI error
func (l *EventLogger) LogIPMIError(operation string, err error) {
	l.Error(models.CategoryIPMI, fmt.Sprintf("IPMI %s failed: %v", operation, err), models.JSONMap{
		"operation": operation,
		"error":     err.Error(),
	})
}

// LogSystemEvent logs a system event
func (l *EventLogger) LogSystemEvent(message string, details models.JSONMap) {
	l.Info(models.CategorySystem, message, details)
}

// LogAuthEvent logs an authentication event
func (l *EventLogger) LogAuthEvent(action, username string, success bool, details models.JSONMap) {
	level := models.LevelInfo
	if !success {
		level = models.LevelWarning
	}
	if details == nil {
		details = models.JSONMap{}
	}
	details["username"] = username
	details["success"] = success
	l.Log(level, models.CategoryAuth, fmt.Sprintf("Auth %s: %s", action, username), details)
}
