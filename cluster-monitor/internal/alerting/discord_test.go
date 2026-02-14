package alerting

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDiscord_SendAlert(t *testing.T) {
	var received discordWebhookPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	d := NewDiscordSender(server.URL)
	err := d.Send(Alert{
		Title:    "High CPU",
		Message:  "Node s0 CPU at 95%",
		Severity: SeverityCritical,
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(received.Embeds) != 1 {
		t.Fatalf("expected 1 embed, got %d", len(received.Embeds))
	}
	if received.Embeds[0].Title != "High CPU" {
		t.Fatalf("expected title 'High CPU', got '%s'", received.Embeds[0].Title)
	}
	if received.Embeds[0].Color != 0xFF0000 {
		t.Fatalf("expected red color for critical, got %d", received.Embeds[0].Color)
	}
}

func TestDiscord_SeverityColors(t *testing.T) {
	tests := []struct {
		severity Severity
		color    int
	}{
		{SeverityCritical, 0xFF0000},
		{SeverityWarning, 0xFFA500},
	}
	for _, tt := range tests {
		var received discordWebhookPayload
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewDecoder(r.Body).Decode(&received)
			w.WriteHeader(http.StatusNoContent)
		}))

		d := NewDiscordSender(server.URL)
		d.Send(Alert{Title: "test", Message: "msg", Severity: tt.severity})
		server.Close()

		if received.Embeds[0].Color != tt.color {
			t.Fatalf("severity %d: expected color %d, got %d", tt.severity, tt.color, received.Embeds[0].Color)
		}
	}
}

func TestDiscord_EmptyURL_Noop(t *testing.T) {
	d := NewDiscordSender("")
	err := d.Send(Alert{Title: "test", Message: "msg", Severity: SeverityCritical})
	if err != nil {
		t.Fatal("empty URL should be a no-op, not an error")
	}
}
