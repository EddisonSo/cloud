package alerting

import (
	"encoding/json"
	"log/slog"
	"time"

	"github.com/gorilla/websocket"
)

type logEntry struct {
	Source  string `json:"source"`
	Level  int    `json:"level"`
	Message string `json:"message"`
}

// SubscribeLogService connects to the log-service WebSocket for ERROR-level logs
// and feeds them into the LogDetector. Reconnects on failure.
func SubscribeLogService(logServiceAddr string, detector *LogDetector) {
	url := "ws://" + logServiceAddr + "/ws/logs?level=ERROR"

	for {
		err := connectAndConsume(url, detector)
		if err != nil {
			slog.Error("log-service WebSocket disconnected", "error", err, "url", url)
		}
		slog.Info("reconnecting to log-service in 5s")
		time.Sleep(5 * time.Second)
	}
}

func connectAndConsume(url string, detector *LogDetector) error {
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	slog.Info("connected to log-service WebSocket", "url", url)

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return err
		}

		var entry logEntry
		if err := json.Unmarshal(msg, &entry); err != nil {
			slog.Warn("failed to parse log entry", "error", err)
			continue
		}

		if entry.Level >= 3 {
			detector.HandleLogEntry(entry.Source, entry.Message)
		}
	}
}
