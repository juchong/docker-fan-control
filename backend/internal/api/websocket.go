package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"docker-fan-control/internal/database"
	"docker-fan-control/internal/models"
	"docker-fan-control/internal/services"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Exact-host origin check to prevent cross-site WebSocket hijacking. The
		// previous strings.Contains(origin, "://"+host) accepted host.evil.com.
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true // non-browser clients send no Origin
		}
		u, err := url.Parse(origin)
		if err != nil || u.Host == "" {
			return false
		}
		host := r.Host
		if host == "" {
			host = r.Header.Get("Host")
		}
		if u.Host == host {
			return true // same-origin
		}
		if hn := u.Hostname(); hn == "localhost" || hn == "127.0.0.1" {
			return true
		}
		if allowed := os.Getenv("ALLOWED_ORIGINS"); allowed != "" {
			for _, o := range strings.Split(allowed, ",") {
				if strings.TrimSpace(o) == origin {
					return true
				}
			}
		}
		log.Warn().Str("origin", origin).Str("host", host).Msg("WebSocket origin validation failed")
		return false
	},
}

// wsConn serializes writes to a single WebSocket connection. Gorilla forbids
// concurrent writes, and the read goroutine's replies (pong/subscribe) would
// otherwise race the periodic write pump and panic.
type wsConn struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (c *wsConn) writeJSON(v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteJSON(v)
}

// WebSocketHandler handles WebSocket connections
type WebSocketHandler struct {
	gpu        *services.GPUService
	system     *services.SystemService
	ipmi       *services.IPMIService
	controller *services.FanController
	fans       *services.FanService
	clients    map[*websocket.Conn]bool
	mu         sync.RWMutex
}

// NewWebSocketHandler creates a new WebSocket handler
func NewWebSocketHandler(gpu *services.GPUService, system *services.SystemService, ipmi *services.IPMIService, controller *services.FanController, fans *services.FanService) *WebSocketHandler {
	return &WebSocketHandler{
		gpu:        gpu,
		system:     system,
		ipmi:       ipmi,
		controller: controller,
		fans:       fans,
		clients:    make(map[*websocket.Conn]bool),
	}
}

// Handle handles WebSocket upgrade and connection
func (h *WebSocketHandler) Handle(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error().Err(err).Msg("WebSocket upgrade failed")
		return
	}
	defer conn.Close()
	wc := &wsConn{conn: conn}

	// Register client
	h.mu.Lock()
	h.clients[conn] = true
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.clients, conn)
		h.mu.Unlock()
	}()

	log.Debug().Str("remote", r.RemoteAddr).Msg("WebSocket client connected")

	// Send initial data
	h.sendMetrics(wc)

	// Start periodic updates
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// Handle incoming messages and send updates
	done := make(chan struct{})

	// Read pump - handle incoming messages
	go func() {
		defer close(done)
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Debug().Err(err).Msg("WebSocket read error")
				}
				return
			}

			// Handle incoming commands
			h.handleMessage(wc, message)
		}
	}()

	// Write pump - send periodic updates
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			h.sendMetrics(wc)
		}
	}
}

// handleMessage handles incoming WebSocket messages
func (h *WebSocketHandler) handleMessage(wc *wsConn, message []byte) {
	var msg struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}

	if err := json.Unmarshal(message, &msg); err != nil {
		return
	}

	switch msg.Type {
	case "ping":
		h.sendMessage(wc, "pong", nil)
	case "subscribe":
		// Could implement topic-based subscriptions
		h.sendMessage(wc, "subscribed", nil)
	}
}

// sendMetrics sends current metrics to a client
func (h *WebSocketHandler) sendMetrics(wc *wsConn) {
	data := h.gatherMetrics()
	h.sendMessage(wc, "metrics", data)
}

// gatherMetrics collects all monitoring data
func (h *WebSocketHandler) gatherMetrics() *models.Monitoring {
	monitoring := &models.Monitoring{
		Controller: h.controller.GetState(),
	}

	// Get motherboard information
	if vendor, err := database.GetSetting(models.SettingMotherboardVendor); err == nil {
		if v, ok := vendor.(string); ok && v != "" {
			monitoring.Controller.MotherboardVendor = v
		}
	}
	if model, err := database.GetSetting(models.SettingMotherboardModel); err == nil {
		if m, ok := model.(string); ok && m != "" {
			monitoring.Controller.MotherboardModel = m
		}
	}
	if driver, err := database.GetSetting(models.SettingMotherboardDriver); err == nil {
		if d, ok := driver.(string); ok && d != "" {
			monitoring.Controller.MotherboardDriver = d
		}
	}

	// Get current driver info
	currentDriver := h.ipmi.GetCurrentDriver()
	if currentDriver != nil {
		monitoring.Controller.DriverVendor = currentDriver.GetVendor()
		monitoring.Controller.DriverModel = currentDriver.GetModel()
		caps := currentDriver.GetCapabilities()
		monitoring.Controller.DriverCapabilities = models.DriverCapabilities{
			SupportsManualMode:       caps.SupportsManualMode,
			SupportsDutyCycleReading: caps.SupportsDutyCycleReading,
			SupportsPerZoneControl:   caps.SupportsPerZoneControl,
			FirmwareFallback:         caps.PerZoneFirmwareFallback,
			MaxZones:                 caps.MaxZones,
			MaxFans:                  caps.MaxFans,
			HasStaticRPMValues:       caps.HasStaticRPMValues,
		}
		if hr, ok := currentDriver.(services.HealthReporter); ok {
			monitoring.Controller.DriverWarnings = hr.DriverWarnings()
		}
	}

	// Get GPU metrics
	if gpuMetrics, err := h.gpu.GetMetrics(); err == nil {
		monitoring.GPUs = gpuMetrics
	}

	// Get system metrics
	if sysMetrics, err := h.system.GetMetrics(); err == nil {
		monitoring.System = *sysMetrics
	}

	// Get fan status
	ctx := context.Background()
	fans, _ := h.fans.List(ctx)
	readings, _ := h.ipmi.GetFanReadings(ctx)

	for _, fan := range fans {
		status := models.FanStatus{
			ID:             fan.ID,
			IPMISensorID:   fan.IPMISensorID,
			Label:          fan.DisplayName(),
			IPMIZone:       fan.IPMIZone,
			Channel:        fan.Channel,
			Chip:           fan.Chip,
			ManualOverride: h.controller.HasManualOverride(fan.ID),
		}

		if reading, ok := readings[fan.IPMISensorID]; ok {
			status.CurrentRPM = reading.RPM
			status.CurrentDuty = reading.DutyCycle
			status.ControlMode = reading.ControlMode
		}
		if fan.IPMIZone != nil {
			if t, ok := h.controller.GetZoneTarget(*fan.IPMIZone); ok {
				status.TargetPercent = &t
			}
		}

		monitoring.Fans = append(monitoring.Fans, status)
	}

	return monitoring
}

// sendMessage sends a typed message to a client (serialized per connection).
func (h *WebSocketHandler) sendMessage(wc *wsConn, msgType string, data any) {
	msg := struct {
		Type      string `json:"type"`
		Timestamp string `json:"timestamp"`
		Data      any    `json:"data"`
	}{
		Type:      msgType,
		Timestamp: time.Now().Format(time.RFC3339),
		Data:      data,
	}

	if err := wc.writeJSON(msg); err != nil {
		log.Debug().Err(err).Msg("WebSocket write error")
	}
}

// ClientCount returns the number of connected clients
func (h *WebSocketHandler) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
