package proxy

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

// requestIDHeader is the correlation header propagated through the gateway.
const requestIDHeader = "X-Request-ID"

// reqIDRe is the allowlist for inbound X-Request-ID values. Only ASCII
// alphanumerics, dots, underscores, and hyphens are accepted, with a maximum
// length of 128 characters. Anything outside this set (including CR/LF and
// other control characters) causes the inbound value to be rejected and a
// fresh UUID to be minted, preventing CRLF injection into raw response headers.
var reqIDRe = regexp.MustCompile(`^[A-Za-z0-9._\-]{1,128}$`)

// validRequestID reports whether id is safe to echo into HTTP response headers.
func validRequestID(id string) bool {
	return reqIDRe.MatchString(id)
}

// newRequestID mints a fresh correlation ID. The repo already vendors
// github.com/google/uuid (pulled in by client-go), so we reuse it rather than
// hand-rolling a crypto/rand generator.
func newRequestID() string {
	return uuid.NewString()
}

// requestID returns the inbound X-Request-ID header if the client already set
// one that passes the allowlist check, otherwise it mints a fresh one. Used on
// the parsed *http.Request path (terminated HTTPS).
func requestID(r *http.Request) string {
	if id := r.Header.Get(requestIDHeader); validRequestID(id) {
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
