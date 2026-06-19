package api

import (
	"bufio"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
)

var logsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		return origin == "" || isAllowedOrigin(origin)
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
}

// HandleContainerLogs streams container stdout/stderr via WebSocket.
// Query params:
//
//	tail  - number of historical lines to send before following (default 100)
func (h *Handler) HandleContainerLogs(w http.ResponseWriter, r *http.Request) {
	containerID := r.PathValue("id")
	if containerID == "" {
		http.Error(w, "container ID required", http.StatusBadRequest)
		return
	}

	userID, _, ok := getUserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	container, err := h.db.GetContainer(containerID)
	if err != nil {
		slog.Error("failed to get container", "error", err, "container", containerID)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if container == nil || container.UserID != userID {
		http.Error(w, "container not found", http.StatusNotFound)
		return
	}

	if container.Status != "running" {
		http.Error(w, "container not running", http.StatusBadRequest)
		return
	}

	// Parse ?tail= query param (default 100)
	tailLines := int64(100)
	if v := r.URL.Query().Get("tail"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
			tailLines = n
		}
	}

	// Pod name is always "container" (hardcoded in CreatePod)
	podName := "container"

	// Upgrade to WebSocket
	ws, err := logsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("websocket upgrade failed", "error", err)
		return
	}
	defer ws.Close()

	// Configure WebSocket keepalive
	ws.SetReadDeadline(time.Now().Add(60 * time.Second))
	ws.SetPongHandler(func(string) error {
		ws.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	slog.Info("log stream started", "container", containerID, "user", userID)

	// Open the log stream from K8s (follow=true)
	stream, err := h.k8s.GetPodLogs(r.Context(), container.Namespace, podName, true, tailLines)
	if err != nil {
		slog.Error("failed to open log stream", "error", err, "container", containerID)
		ws.WriteMessage(websocket.TextMessage, []byte("error: failed to open log stream"))
		return
	}
	defer stream.Close()

	// Read lines and forward to WebSocket
	scanner := bufio.NewScanner(stream)
	for scanner.Scan() {
		line := scanner.Bytes()
		if err := ws.WriteMessage(websocket.TextMessage, line); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				slog.Debug("websocket write error in log stream", "error", err)
			}
			break
		}
		// Refresh read deadline periodically as we emit lines
		ws.SetReadDeadline(time.Now().Add(60 * time.Second))
	}

	if err := scanner.Err(); err != nil {
		slog.Debug("log stream scanner error", "error", err, "container", containerID)
	}

	slog.Info("log stream ended", "container", containerID)
}
