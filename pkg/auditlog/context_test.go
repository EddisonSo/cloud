package auditlog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestContextGettersDefaults(t *testing.T) {
	ctx := context.Background()
	if actorFrom(ctx) != "anonymous" {
		t.Fatal("actor default")
	}
	if clientIPFrom(ctx) != "" || requestIDFrom(ctx) != "" {
		t.Fatal("empty defaults")
	}
	ctx = WithActor(WithRequestID(ctx, "rid"), "bob")
	if actorFrom(ctx) != "bob" || requestIDFrom(ctx) != "rid" {
		t.Fatal("set/get")
	}
}

func TestHTTPMiddleware_PopulatesContext(t *testing.T) {
	var gotRID, gotIP string
	h := HTTPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRID = requestIDFrom(r.Context())
		gotIP = clientIPFrom(r.Context())
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Request-ID", "abc123")
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if gotRID != "abc123" {
		t.Fatalf("rid=%q", gotRID)
	}
	if gotIP != "1.2.3.4" {
		t.Fatalf("ip=%q", gotIP)
	}
}
