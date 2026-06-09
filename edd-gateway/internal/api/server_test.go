package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"eddisonso.com/edd-gateway/internal/cloudflare"
	"eddisonso.com/edd-gateway/internal/secretbox"
)

func TestAllowedPort(t *testing.T) {
	cases := []struct {
		port int
		want bool
	}{
		{80, true},
		{443, true},
		{8000, true},
		{8999, true},
		{7999, false},
		{9000, false},
		{22, false},
		{0, false},
	}
	for _, c := range cases {
		if got := allowedPort(c.port); got != c.want {
			t.Errorf("allowedPort(%d) = %v, want %v", c.port, got, c.want)
		}
	}
}

func TestPutCloudflareTokenInvalid(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"success":false,"errors":[{"code":9109,"message":"Invalid access token"}],"result":null}`))
	}))
	defer srv.Close()
	old := newCFClient
	newCFClient = func(token string) *cloudflare.Client { return cloudflare.NewForTest(token, srv.URL, srv.Client()) }
	defer func() { newCFClient = old }()

	box, _ := secretbox.New("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	s := &Server{box: box}
	req := httptest.NewRequest("PUT", "/api/cloudflare-token", strings.NewReader(`{"token":"bad"}`))
	w := httptest.NewRecorder()
	s.handleCloudflareToken(w, req, "u1")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestCloudflareTokenDisabled(t *testing.T) {
	s := &Server{} // box nil
	req := httptest.NewRequest("GET", "/api/cloudflare-token", nil)
	w := httptest.NewRecorder()
	s.handleCloudflareToken(w, req, "u1")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", w.Code)
	}
}
