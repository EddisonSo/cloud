package alerting

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Severity int

const (
	SeverityWarning  Severity = iota
	SeverityCritical
)

type Alert struct {
	Title    string
	Message  string
	Severity Severity
}

type DiscordSender struct {
	webhookURL string
	client     *http.Client
}

func NewDiscordSender(webhookURL string) *DiscordSender {
	return &DiscordSender{
		webhookURL: webhookURL,
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

type discordEmbed struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Color       int    `json:"color"`
	Timestamp   string `json:"timestamp"`
}

type discordWebhookPayload struct {
	Embeds []discordEmbed `json:"embeds"`
}

func (d *DiscordSender) Send(alert Alert) error {
	if d.webhookURL == "" {
		return nil
	}

	color := 0xFFA500 // orange/warning
	if alert.Severity == SeverityCritical {
		color = 0xFF0000 // red
	}

	payload := discordWebhookPayload{
		Embeds: []discordEmbed{{
			Title:       alert.Title,
			Description: alert.Message,
			Color:       color,
			Timestamp:   time.Now().UTC().Format(time.RFC3339),
		}},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal discord payload: %w", err)
	}

	resp, err := d.client.Post(d.webhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("discord webhook POST: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("discord webhook returned %d", resp.StatusCode)
	}
	return nil
}
