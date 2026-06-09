package secretbox

import (
	"bytes"
	"strings"
	"testing"
)

const testKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" // 32 bytes hex

func TestSealOpenRoundTrip(t *testing.T) {
	box, err := New(testKey)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ct := box.Seal([]byte("cf-token-secret"))
	if bytes.Contains(ct, []byte("cf-token-secret")) {
		t.Fatal("ciphertext contains plaintext")
	}
	pt, err := box.Open(ct)
	if err != nil || string(pt) != "cf-token-secret" {
		t.Fatalf("Open: %v %q", err, pt)
	}
}

func TestOpenRejectsTamper(t *testing.T) {
	box, _ := New(testKey)
	ct := box.Seal([]byte("data"))
	ct[len(ct)-1] ^= 0xff
	if _, err := box.Open(ct); err == nil {
		t.Fatal("expected tampered ciphertext to fail")
	}
}

func TestSealUniqueNonce(t *testing.T) {
	box, _ := New(testKey)
	if bytes.Equal(box.Seal([]byte("x")), box.Seal([]byte("x"))) {
		t.Fatal("two seals of same plaintext must differ (random nonce)")
	}
}

func TestNewRejectsBadKey(t *testing.T) {
	for _, k := range []string{"", "abcd", strings.Repeat("zz", 32)} {
		if _, err := New(k); err == nil {
			t.Errorf("New(%q) should fail", k)
		}
	}
}
