package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for now
	},
}

// WSMessage represents a WebSocket message
type WSMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// ContainerStatusUpdate represents a container status change
type ContainerStatusUpdate struct {
	ContainerID string  `json:"container_id"`
	Status      string  `json:"status"`
	ExternalIP  *string `json:"external_ip,omitempty"`
}

// WSHub manages WebSocket connections per user
type WSHub struct {
	mu    sync.RWMutex
	conns map[int64]map[*websocket.Conn]bool // userID -> connections
}

// Global hub instance
var hub = &WSHub{
	conns: make(map[int64]map[*websocket.Conn]bool),
}

// GetHub returns the global WebSocket hub
func GetHub() *WSHub {
	return hub
}

// Register adds a connection for a user
func (h *WSHub) Register(userID int64, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.conns[userID] == nil {
		h.conns[userID] = make(map[*websocket.Conn]bool)
	}
	h.conns[userID][conn] = true
	slog.Debug("WebSocket registered", "user", userID)
}

// Unregister removes a connection
func (h *WSHub) Unregister(userID int64, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.conns[userID] != nil {
		delete(h.conns[userID], conn)
		if len(h.conns[userID]) == 0 {
			delete(h.conns, userID)
		}
	}
	slog.Debug("WebSocket unregistered", "user", userID)
}

// BroadcastToUser sends a message to all connections for a user
func (h *WSHub) BroadcastToUser(userID int64, msg WSMessage) {
	h.mu.RLock()
	conns := h.conns[userID]
	h.mu.RUnlock()

	if conns == nil {
		return
	}

	data, err := json.Marshal(msg)
	if err != nil {
		slog.Error("failed to marshal ws message", "error", err)
		return
	}

	for conn := range conns {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			slog.Debug("failed to write ws message", "error", err)
			conn.Close()
			h.Unregister(userID, conn)
		}
	}
}

// SendContainerStatus broadcasts a container status update to a user
func (h *WSHub) SendContainerStatus(userID int64, containerID string, status string, externalIP *string) {
	h.BroadcastToUser(userID, WSMessage{
		Type: "container_status",
		Data: ContainerStatusUpdate{
			ContainerID: containerID,
			Status:      status,
			ExternalIP:  externalIP,
		},
	})
}

// HandleWebSocket handles WebSocket connections
func (h *Handler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	internalID, userID, _, ok := getUserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("websocket upgrade failed", "error", err)
		return
	}

	hub := GetHub()
	hub.Register(internalID, conn)

	// Send initial container list
	containers, err := h.db.ListContainersByUser(userID)
	if err == nil {
		resp := make([]containerResponse, 0, len(containers))
		for _, c := range containers {
			resp = append(resp, containerToResponse(c))
		}
		msg := WSMessage{Type: "containers", Data: resp}
		if data, err := json.Marshal(msg); err == nil {
			conn.WriteMessage(websocket.TextMessage, data)
		}
	}

	// Setup ping/pong for connection keepalive
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	// Start ping ticker
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	done := make(chan struct{})

	// Read messages (to detect disconnection)
	go func() {
		defer close(done)
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	}()

	// Ping loop
	for {
		select {
		case <-done:
			hub.Unregister(internalID, conn)
			conn.Close()
			return
		case <-ticker.C:
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				hub.Unregister(internalID, conn)
				conn.Close()
				return
			}
		}
	}
}
