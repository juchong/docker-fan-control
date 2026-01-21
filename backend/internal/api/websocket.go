package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"docker-fan-control/internal/models"
	"docker-fan-control/internal/services"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins
	},
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
	h.sendMetrics(conn)

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
			h.handleMessage(conn, message)
		}
	}()

	// Write pump - send periodic updates
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			h.sendMetrics(conn)
		}
	}
}

// handleMessage handles incoming WebSocket messages
func (h *WebSocketHandler) handleMessage(conn *websocket.Conn, message []byte) {
	var msg struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}

	if err := json.Unmarshal(message, &msg); err != nil {
		return
	}

	switch msg.Type {
	case "ping":
		h.sendMessage(conn, "pong", nil)
	case "subscribe":
		// Could implement topic-based subscriptions
		h.sendMessage(conn, "subscribed", nil)
	}
}

// sendMetrics sends current metrics to a client
func (h *WebSocketHandler) sendMetrics(conn *websocket.Conn) {
	data := h.gatherMetrics()
	h.sendMessage(conn, "metrics", data)
}

// gatherMetrics collects all monitoring data
func (h *WebSocketHandler) gatherMetrics() *models.Monitoring {
	monitoring := &models.Monitoring{
		Controller: h.controller.GetState(),
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
			ManualOverride: h.controller.HasManualOverride(fan.ID),
		}

		if reading, ok := readings[fan.IPMISensorID]; ok {
			status.CurrentRPM = reading.RPM
			status.CurrentDuty = reading.DutyCycle
		}

		monitoring.Fans = append(monitoring.Fans, status)
	}

	return monitoring
}

// sendMessage sends a typed message to a client
func (h *WebSocketHandler) sendMessage(conn *websocket.Conn, msgType string, data any) {
	msg := struct {
		Type      string `json:"type"`
		Timestamp string `json:"timestamp"`
		Data      any    `json:"data"`
	}{
		Type:      msgType,
		Timestamp: time.Now().Format(time.RFC3339),
		Data:      data,
	}

	if err := conn.WriteJSON(msg); err != nil {
		log.Debug().Err(err).Msg("WebSocket write error")
	}
}

// Broadcast sends a message to all connected clients
func (h *WebSocketHandler) Broadcast(msgType string, data any) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for conn := range h.clients {
		h.sendMessage(conn, msgType, data)
	}
}

// ClientCount returns the number of connected clients
func (h *WebSocketHandler) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
