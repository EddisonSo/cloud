package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"eddisonso.com/edd-cloud-auth/internal/db"
	"eddisonso.com/edd-cloud/pkg/auditlog"
	"golang.org/x/crypto/bcrypt"
)

// captureHandler is a minimal slog.Handler that records every event's message
// and string-rendered attributes so tests can assert on emitted audit fields.
type captureHandler struct {
	mu      sync.Mutex
	records []map[string]string
}

func (c *captureHandler) Enabled(context.Context, slog.Level) bool { return true }
func (c *captureHandler) WithAttrs([]slog.Attr) slog.Handler       { return c }
func (c *captureHandler) WithGroup(string) slog.Handler            { return c }

func (c *captureHandler) Handle(_ context.Context, r slog.Record) error {
	m := map[string]string{"msg": r.Message, "level": r.Level.String()}
	r.Attrs(func(a slog.Attr) bool {
		m[a.Key] = a.Value.String()
		return true
	})
	c.mu.Lock()
	c.records = append(c.records, m)
	c.mu.Unlock()
	return nil
}

func (c *captureHandler) find(action string) map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, rec := range c.records {
		if rec["action"] == action {
			return rec
		}
	}
	return nil
}

// TestHandleLogin_FailureEmitsAuditEvent verifies that both credential-rejection
// branches of handleLogin emit a security audit event marked audit=true with
// action=auth.login and outcome=failure, carrying the username as the resource
// and the client IP seeded into the request context.
func TestHandleLogin_FailureEmitsAuditEvent(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("correct-horse"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	// Cases exercise the bcrypt credential-rejection branch of handleLogin, which
	// is served entirely from the in-memory user cache (no live Postgres). The
	// unknown-user branch is not table-tested here because a cache miss falls
	// through to a real DB query.
	cases := []struct {
		name         string
		username     string
		password     string
		wantResource string
	}{
		{name: "wrong password", username: "alice", password: "wrong", wantResource: "alice"},
		{name: "empty-ish wrong password", username: "alice", password: "x", wantResource: "alice"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cap := &captureHandler{}
			prev := slog.Default()
			slog.SetDefault(slog.New(cap))
			defer slog.SetDefault(prev)

			database := db.NewWithUsersForTest(&db.User{
				UserID:       "u1",
				Username:     "alice",
				PasswordHash: string(hash),
			})
			h := NewHandler(Config{
				DB:         database,
				JWTSecret:  []byte("test-secret"),
				SessionTTL: time.Hour,
			})

			body, _ := json.Marshal(loginRequest{Username: tc.username, Password: tc.password})
			r := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(body))
			r = r.WithContext(auditlog.WithClientIP(r.Context(), "203.0.113.7"))
			rec := httptest.NewRecorder()

			h.handleLogin(rec, r)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d", rec.Code)
			}

			ev := cap.find("auth.login")
			if ev == nil {
				t.Fatalf("no audit event with action=auth.login was emitted; records=%v", cap.records)
			}
			if ev["audit"] != "true" {
				t.Errorf("expected audit=true, got %q", ev["audit"])
			}
			if ev["outcome"] != "failure" {
				t.Errorf("expected outcome=failure, got %q", ev["outcome"])
			}
			if ev["resource"] != tc.wantResource {
				t.Errorf("expected resource=%q, got %q", tc.wantResource, ev["resource"])
			}
			if ev["client_ip"] != "203.0.113.7" {
				t.Errorf("expected client_ip=203.0.113.7, got %q", ev["client_ip"])
			}
		})
	}
}
