package api

import (
	"net/http"
	"strconv"
	"time"

	"docker-fan-control/internal/models"
	"docker-fan-control/internal/services"
)

// LogsHandler handles log endpoints
type LogsHandler struct {
	logger *services.EventLogger
}

// NewLogsHandler creates a new logs handler
func NewLogsHandler(logger *services.EventLogger) *LogsHandler {
	return &LogsHandler{logger: logger}
}

// Allowed log levels and categories for validation
var (
	allowedLevels     = map[string]bool{"info": true, "warn": true, "error": true, "debug": true}
	allowedCategories = map[string]bool{
		string(models.CategoryFan): true, string(models.CategoryProfile): true,
		string(models.CategorySystem): true, string(models.CategoryIPMI): true,
		string(models.CategoryAuth): true, string(models.CategoryTemp): true,
	}
)

// MaxLogLimit is the maximum number of log entries that can be retrieved
const MaxLogLimit = 1000

// List handles GET /api/logs
func (h *LogsHandler) List(w http.ResponseWriter, r *http.Request) {
	query := &models.EventQuery{}

	// Parse and validate query parameters
	if level := r.URL.Query().Get("level"); level != "" {
		if !allowedLevels[level] {
			http.Error(w, "Invalid log level", http.StatusBadRequest)
			return
		}
		query.Level = level
	}
	if category := r.URL.Query().Get("category"); category != "" {
		if !allowedCategories[category] {
			http.Error(w, "Invalid category", http.StatusBadRequest)
			return
		}
		query.Category = category
	}
	if search := r.URL.Query().Get("search"); search != "" {
		// Sanitize search input - limit length and remove dangerous characters
		sanitized := SanitizeInput(search)
		if len(sanitized) > 100 {
			sanitized = sanitized[:100]
		}
		query.Search = sanitized
	}
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			if limit > 0 && limit <= MaxLogLimit {
				query.Limit = limit
			}
		}
	}
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil {
			if offset >= 0 {
				query.Offset = offset
			}
		}
	}
	if startStr := r.URL.Query().Get("start_time"); startStr != "" {
		if start, err := time.Parse(time.RFC3339, startStr); err == nil {
			query.StartTime = &start
		}
	}
	if endStr := r.URL.Query().Get("end_time"); endStr != "" {
		if end, err := time.Parse(time.RFC3339, endStr); err == nil {
			query.EndTime = &end
		}
	}

	// Default limit
	if query.Limit == 0 {
		query.Limit = 50
	}

	response, err := h.logger.Query(r.Context(), query)
	if err != nil {
		http.Error(w, "Failed to query logs", http.StatusInternalServerError)
		return
	}

	writeJSON(w, response)
}

// Export handles GET /api/logs/export
func (h *LogsHandler) Export(w http.ResponseWriter, r *http.Request) {
	query := &models.EventQuery{}

	// Parse and validate query parameters
	if level := r.URL.Query().Get("level"); level != "" {
		if !allowedLevels[level] {
			http.Error(w, "Invalid log level", http.StatusBadRequest)
			return
		}
		query.Level = level
	}
	if category := r.URL.Query().Get("category"); category != "" {
		if !allowedCategories[category] {
			http.Error(w, "Invalid category", http.StatusBadRequest)
			return
		}
		query.Category = category
	}
	if search := r.URL.Query().Get("search"); search != "" {
		sanitized := SanitizeInput(search)
		if len(sanitized) > 100 {
			sanitized = sanitized[:100]
		}
		query.Search = sanitized
	}
	if startStr := r.URL.Query().Get("start_time"); startStr != "" {
		if start, err := time.Parse(time.RFC3339, startStr); err == nil {
			query.StartTime = &start
		}
	}
	if endStr := r.URL.Query().Get("end_time"); endStr != "" {
		if end, err := time.Parse(time.RFC3339, endStr); err == nil {
			query.EndTime = &end
		}
	}

	format := r.URL.Query().Get("format")
	if format == "" {
		format = "json"
	}

	switch format {
	case "csv":
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment; filename=fan-control-logs.csv")
		if err := h.logger.ExportCSV(r.Context(), query, w); err != nil {
			http.Error(w, "Failed to export logs", http.StatusInternalServerError)
		}
	case "json":
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", "attachment; filename=fan-control-logs.json")
		if err := h.logger.ExportJSON(r.Context(), query, w); err != nil {
			http.Error(w, "Failed to export logs", http.StatusInternalServerError)
		}
	default:
		http.Error(w, "Invalid format. Use 'json' or 'csv'", http.StatusBadRequest)
	}
}

// Clear handles DELETE /api/logs
func (h *LogsHandler) Clear(w http.ResponseWriter, r *http.Request) {
	if err := h.logger.Clear(r.Context()); err != nil {
		http.Error(w, "Failed to clear logs", http.StatusInternalServerError)
		return
	}

	h.logger.Info(models.CategorySystem, "Event logs cleared", nil)
	w.WriteHeader(http.StatusNoContent)
}
