package api

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// captureHandler is a minimal slog.Handler that records the attributes of every
// log record so tests can assert on emitted audit events.
type captureHandler struct {
	mu      sync.Mutex
	records *[]map[string]any
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	attrs := map[string]any{"_msg": r.Message, "_level": r.Level}
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.Any()
		return true
	})
	h.mu.Lock()
	*h.records = append(*h.records, attrs)
	h.mu.Unlock()
	return nil
}

func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(string) slog.Handler      { return h }

func TestAuditDeniedOnScopeCheck(t *testing.T) {
	tests := []struct {
		name       string
		middleware func(*Handler) http.HandlerFunc
		ctx        context.Context
		wantScope  string
	}{
		{
			name: "container scope denial emits audit event",
			middleware: func(h *Handler) http.HandlerFunc {
				return h.scopeCheckContainer("delete", func(w http.ResponseWriter, r *http.Request) {
					t.Fatal("next handler should not be called on denial")
				})
			},
			// API token with no granted scopes -> requireScope returns false.
			ctx:       setAPITokenContext(context.Background(), "user-123", map[string][]string{}),
			wantScope: "compute.user-123.containers.",
		},
		{
			name: "generic resource scope denial emits audit event",
			middleware: func(h *Handler) http.HandlerFunc {
				return h.scopeCheck("containers", "create", func(w http.ResponseWriter, r *http.Request) {
					t.Fatal("next handler should not be called on denial")
				})
			},
			ctx:       setAPITokenContext(context.Background(), "user-123", map[string][]string{}),
			wantScope: "compute.user-123.containers",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var records []map[string]any
			prev := slog.Default()
			slog.SetDefault(slog.New(&captureHandler{records: &records}))
			defer slog.SetDefault(prev)

			h := &Handler{}
			req := httptest.NewRequest(http.MethodDelete, "/compute/containers/abc", nil).WithContext(tc.ctx)
			rec := httptest.NewRecorder()

			tc.middleware(h)(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Fatalf("expected status 403, got %d", rec.Code)
			}

			var audit map[string]any
			for _, r := range records {
				if r["audit"] == "true" {
					audit = r
					break
				}
			}
			if audit == nil {
				t.Fatalf("no audit event emitted; records=%v", records)
			}
			if audit["action"] != "authz.denied" {
				t.Errorf("action = %v, want authz.denied", audit["action"])
			}
			if audit["outcome"] != "denied" {
				t.Errorf("outcome = %v, want denied", audit["outcome"])
			}
			if audit["_level"] != slog.LevelWarn {
				t.Errorf("level = %v, want WARN", audit["_level"])
			}
			if res, _ := audit["resource"].(string); res == "" {
				t.Errorf("resource missing, want scope %q", tc.wantScope)
			}
			if actor, _ := audit["actor"].(string); actor != "user-123" {
				t.Errorf("actor = %v, want user-123", actor)
			}
		})
	}
}
