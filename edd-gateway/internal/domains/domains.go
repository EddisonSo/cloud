// Package domains holds pure helpers for custom-domain handling: no DB, no I/O
// except DNS lookups (handled elsewhere). Keeping these pure makes them unit-testable.
package domains

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
)

// Normalize lowercases and trims a domain for consistent storage/lookup.
func Normalize(domain string) string {
	return strings.ToLower(strings.TrimSpace(domain))
}

// Valid reports whether domain is a syntactically valid hostname (no wildcard,
// no scheme, no path). It must contain at least one dot and only LDH labels.
func Valid(domain string) bool {
	d := Normalize(domain)
	if len(d) == 0 || len(d) > 253 || !strings.Contains(d, ".") {
		return false
	}
	labels := strings.Split(d, ".")
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 {
			return false
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for i := 0; i < len(label); i++ {
			c := label[i]
			isLetter := c >= 'a' && c <= 'z'
			isDigit := c >= '0' && c <= '9'
			if !isLetter && !isDigit && c != '-' {
				return false
			}
		}
	}
	return true
}

// GenerateToken returns a random 40-char lowercase-hex token for the
// _edd-verify TXT record. Hex chars are all valid in a TXT value.
func GenerateToken() string {
	b := make([]byte, 20)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// VerifyRecordName is the TXT record a user must create to prove ownership.
func VerifyRecordName(domain string) string {
	return "_edd-verify." + Normalize(domain)
}

// TXTMatches reports whether any record equals the expected token (trimmed).
func TXTMatches(records []string, token string) bool {
	for _, r := range records {
		if strings.TrimSpace(r) == token {
			return true
		}
	}
	return false
}
