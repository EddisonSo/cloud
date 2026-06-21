package proxy

import (
	"net/http"
	"strings"

	"github.com/google/uuid"
)

// requestIDHeader is the correlation header propagated through the gateway.
const requestIDHeader = "X-Request-ID"

// newRequestID mints a fresh correlation ID. The repo already vendors
// github.com/google/uuid (pulled in by client-go), so we reuse it rather than
// hand-rolling a crypto/rand generator.
func newRequestID() string {
	return uuid.NewString()
}

// requestID returns the inbound X-Request-ID header if the client already set
// one (so a correlation ID minted upstream is preserved end-to-end), otherwise
// it mints a fresh one. Used on the parsed *http.Request path (terminated HTTPS).
func requestID(r *http.Request) string {
	if id := r.Header.Get(requestIDHeader); id != "" {
		return id
	}
	return newRequestID()
}

// extractHeaderValue pulls a header value out of raw HTTP header text. Used on
// the raw-TCP HTTP path where the request is never parsed into an *http.Request.
// Header names are matched case-insensitively.
func extractHeaderValue(headers, name string) string {
	prefix := strings.ToLower(name) + ":"
	for _, line := range strings.Split(headers, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(line), prefix) {
			return strings.TrimSpace(line[len(prefix):])
		}
	}
	return ""
}
