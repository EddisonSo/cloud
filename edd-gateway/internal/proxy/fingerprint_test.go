package proxy

import (
	"net/http"
	"testing"
)

func TestStripFingerprintHeaders(t *testing.T) {
	h := http.Header{}
	h.Set("Server", "nginx/1.27.5")
	h.Set("X-Powered-By", "Express")
	h.Set("Content-Type", "text/html")
	stripFingerprintHeaders(h)
	if h.Get("Server") != "" || h.Get("X-Powered-By") != "" {
		t.Fatalf("fingerprint headers not stripped: %v", h)
	}
	if h.Get("Content-Type") != "text/html" {
		t.Fatalf("unrelated header removed: %v", h)
	}
}
