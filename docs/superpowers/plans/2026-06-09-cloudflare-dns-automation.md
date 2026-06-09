# Cloudflare DNS Automation (Per-User Tokens) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a user connect their own Cloudflare API token; adding a custom domain then auto-creates the grey-cloud CNAME in their zone and goes live in seconds (auto-verified + cert pre-issued), with graceful fallback to the manual TXT flow.

**Architecture:** Per-user CF tokens stored AES-256-GCM-encrypted in the gateway's Postgres (key from a K8s Secret). A minimal `internal/cloudflare` client is built per-request from the user's decrypted token. `createDomain` tries zone-match → upsert CNAME (`proxied:false` forced) → insert `verified` + pre-issue; any failure falls back to `pending` + TXT. Three new token endpoints on the existing JWT+CORS mux. Networking tab gets a "Cloudflare integration" card.

**Tech Stack:** Go (crypto/aes GCM, net/http, httptest), Cloudflare v4 API, PostgreSQL, K8s Secret env, React/TS.

**Spec:** `docs/superpowers/specs/2026-06-09-cloudflare-dns-automation-design.md` (rev 2)

---

## Conventions

- **Branch:** `feat/cloudflare-dns` off up-to-date main; merge at end (pushes to main deploy).
- **Commits:** ALWAYS via the `commit-organizer` agent — explicit per-path `git add` list, ONE self-contained dispatch per commit, `.claude/agents/*.md` stay untracked, branch pushes don't deploy (no actions-monitor).
- **Secrets:** `TOKEN_ENCRYPTION_KEY` (hex, 32 bytes → `openssl rand -hex 32`) in K8s Secret `gateway-token-key` (ns `core`), created with `kubectl create secret` — never committed. Manifest references it `optional: true`.
- **Go verification per task:** from `edd-gateway/`: `go build ./...`, `go vet ./<pkg>`, `go test ./<pkg>`. DB-gated tests skip without `DATABASE_URL_TEST`.

## File map

- Create: `edd-gateway/internal/secretbox/secretbox.go` (+ test) — AES-GCM seal/open
- Create: `edd-gateway/internal/cloudflare/cloudflare.go` (+ test) — minimal CF v4 client
- Modify: `edd-gateway/internal/router/router.go` — `user_cloudflare_tokens` table + CRUD; stamp `verified_at` in verified inserts (+ DB-gated test additions)
- Create: `edd-gateway/internal/api/cloudflare_token.go` — token endpoints
- Modify: `edd-gateway/internal/api/server.go` — box field, createDomain branch, delete cleanup, `dns_automated`
- Modify: `edd-gateway/main.go` — build secretbox from env
- Modify: `edd-gateway/manifests/gateway.yaml` — optional `TOKEN_ENCRYPTION_KEY`
- Create: `edd-cloud-interface/frontend/src/components/networking/CloudflareCard.tsx`
- Modify: frontend `types/domain.ts`, `hooks/useCustomDomains.ts`, `components/networking/AddDomainForm.tsx`, `pages/NetworkingPage.tsx`
- Modify: `edd-cloud-docs/docs/guides/custom-domains.md`

---

### Task 1: secretbox package (TDD)

**Files:** Create `edd-gateway/internal/secretbox/secretbox.go`, `secretbox_test.go`

- [ ] **Step 1: Failing test**

```go
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
```

- [ ] **Step 2: Run, expect FAIL** — `go test ./internal/secretbox/` (undefined symbols).

- [ ] **Step 3: Implement**

```go
// Package secretbox seals small secrets (user API tokens) with AES-256-GCM
// for at-rest storage in Postgres. The random nonce is prepended to the
// ciphertext. The key comes from a K8s Secret, hex-encoded.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
)

// Box seals and opens secrets with a fixed key.
type Box struct {
	aead cipher.AEAD
}

// New builds a Box from a 64-char hex string (32-byte key).
func New(keyHex string) (*Box, error) {
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		return nil, fmt.Errorf("secretbox: decode key: %w", err)
	}
	if len(key) != 32 {
		return nil, errors.New("secretbox: key must be 32 bytes (64 hex chars)")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("secretbox: cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secretbox: gcm: %w", err)
	}
	return &Box{aead: aead}, nil
}

// Seal encrypts plaintext; output is nonce || ciphertext.
func (b *Box) Seal(plaintext []byte) []byte {
	nonce := make([]byte, b.aead.NonceSize())
	_, _ = rand.Read(nonce)
	return b.aead.Seal(nonce, nonce, plaintext, nil)
}

// Open decrypts data produced by Seal.
func (b *Box) Open(data []byte) ([]byte, error) {
	ns := b.aead.NonceSize()
	if len(data) < ns {
		return nil, errors.New("secretbox: ciphertext too short")
	}
	pt, err := b.aead.Open(nil, data[:ns], data[ns:], nil)
	if err != nil {
		return nil, fmt.Errorf("secretbox: open: %w", err)
	}
	return pt, nil
}
```

- [ ] **Step 4: Run, expect PASS** + `go build ./...` + `go vet ./internal/secretbox/`.
- [ ] **Step 5: Commit** via commit-organizer: `feat(gateway): add AES-GCM secretbox for at-rest token encryption`.

---

### Task 2: Cloudflare client package (TDD)

**Files:** Create `edd-gateway/internal/cloudflare/cloudflare.go`, `cloudflare_test.go`

- [ ] **Step 1: Failing tests**

```go
package cloudflare

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMatchZone(t *testing.T) {
	zones := []Zone{
		{ID: "z1", Name: "eddisonso2.com"},
		{ID: "z2", Name: "sub.eddisonso2.com"},
		{ID: "z3", Name: "other.io"},
	}
	cases := []struct {
		domain string
		wantID string
		wantOK bool
	}{
		{"resume.eddisonso2.com", "z1", true},
		{"a.sub.eddisonso2.com", "z2", true}, // longest suffix wins
		{"eddisonso2.com", "z1", true},       // zone apex exact match
		{"noteddisonso2.com", "", false},     // suffix must be dot-separated
		{"example.org", "", false},
	}
	for _, c := range cases {
		id, ok := matchZone(c.domain, zones)
		if ok != c.wantOK || id != c.wantID {
			t.Errorf("matchZone(%q) = (%q,%v), want (%q,%v)", c.domain, id, ok, c.wantID, c.wantOK)
		}
	}
}

func stub(t *testing.T, handler http.HandlerFunc) *Client {
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &Client{token: "tok", baseURL: srv.URL, http: srv.Client()}
}

func envelope(result any) []byte {
	b, _ := json.Marshal(map[string]any{"success": true, "errors": []any{}, "result": result})
	return b
}

func TestListZonesAndFindZone(t *testing.T) {
	c := stub(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("missing bearer token")
		}
		w.Write(envelope([]Zone{{ID: "z1", Name: "eddisonso2.com"}}))
	})
	zs, err := c.ListZones()
	if err != nil || len(zs) != 1 || zs[0].Name != "eddisonso2.com" {
		t.Fatalf("ListZones: %v %+v", err, zs)
	}
	id, err := c.FindZone("resume.eddisonso2.com")
	if err != nil || id != "z1" {
		t.Fatalf("FindZone: %v %q", err, id)
	}
	if _, err := c.FindZone("nope.example.org"); !errors.Is(err, ErrZoneNotFound) {
		t.Fatalf("want ErrZoneNotFound, got %v", err)
	}
}

func TestUpsertCNAMECreates(t *testing.T) {
	var created dnsRecord
	c := stub(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			w.Write(envelope([]dnsRecord{}))
		case "POST":
			json.NewDecoder(r.Body).Decode(&created)
			w.Write(envelope(created))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})
	if err := c.UpsertCNAME("z1", "resume.eddisonso2.com", "ingress.cloud.eddisonso.com"); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if created.Type != "CNAME" || created.Proxied != false || created.Content != "ingress.cloud.eddisonso.com" {
		t.Errorf("bad create payload: %+v", created)
	}
}

func TestUpsertCNAMEUpdatesExisting(t *testing.T) {
	var updated dnsRecord
	var putPath string
	c := stub(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET": // existing orange-cloud record gets repaired
			w.Write(envelope([]dnsRecord{{ID: "rec9", Type: "CNAME", Name: "resume.eddisonso2.com", Content: "x", Proxied: true}}))
		case "PUT":
			putPath = r.URL.Path
			json.NewDecoder(r.Body).Decode(&updated)
			w.Write(envelope(updated))
		default:
			t.Errorf("unexpected %s", r.Method)
		}
	})
	if err := c.UpsertCNAME("z1", "resume.eddisonso2.com", "ingress.cloud.eddisonso.com"); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if updated.Proxied != false || putPath != "/zones/z1/dns_records/rec9" {
		t.Errorf("update wrong: proxied=%v path=%s", updated.Proxied, putPath)
	}
}

func TestDeleteRecordGuard(t *testing.T) {
	deleted := false
	c := stub(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET": // content does NOT match the expected target
			w.Write(envelope([]dnsRecord{{ID: "rec1", Content: "somewhere-else.example"}}))
		case "DELETE":
			deleted = true
			w.Write(envelope(nil))
		}
	})
	if err := c.DeleteRecord("z1", "resume.eddisonso2.com", "ingress.cloud.eddisonso.com"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if deleted {
		t.Error("must NOT delete a record whose content differs from expected")
	}
}

func TestAPIErrorSurfaces(t *testing.T) {
	c := stub(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"success":false,"errors":[{"code":10000,"message":"auth error"}],"result":null}`))
	})
	if _, err := c.ListZones(); err == nil {
		t.Fatal("expected error from success:false envelope")
	}
}
```

- [ ] **Step 2: Run, expect FAIL** — `go test ./internal/cloudflare/`.

- [ ] **Step 3: Implement**

```go
// Package cloudflare is a minimal Cloudflare v4 API client used to automate
// custom-domain DNS with a USER-provided token: list zones, upsert a
// grey-cloud CNAME, and clean up on delete. proxied:false is forced — a
// proxied (orange-cloud) record breaks ACME TLS-ALPN-01 and causes an
// HTTP->HTTPS redirect loop.
package cloudflare

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrZoneNotFound means no zone on the token covers the domain — the caller
// should fall back to the manual verification flow.
var ErrZoneNotFound = errors.New("cloudflare: zone not found for domain")

// Zone is a Cloudflare zone visible to the token.
type Zone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Client talks to the Cloudflare v4 API with a bearer token.
type Client struct {
	token   string
	baseURL string
	http    *http.Client
}

// New builds a client for api.cloudflare.com using the given (user) token.
func New(token string) *Client {
	return &Client{
		token:   token,
		baseURL: "https://api.cloudflare.com/client/v4",
		http:    &http.Client{Timeout: 15 * time.Second},
	}
}

type apiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type apiEnvelope struct {
	Success bool            `json:"success"`
	Errors  []apiError      `json:"errors"`
	Result  json.RawMessage `json:"result"`
}

func (c *Client) do(method, path string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("cloudflare: marshal body: %w", err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.baseURL+path, rdr)
	if err != nil {
		return fmt.Errorf("cloudflare: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("cloudflare: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	var env apiEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return fmt.Errorf("cloudflare: decode response (%s): %w", resp.Status, err)
	}
	if !env.Success {
		msg := "unknown error"
		if len(env.Errors) > 0 {
			msg = fmt.Sprintf("%d: %s", env.Errors[0].Code, env.Errors[0].Message)
		}
		return fmt.Errorf("cloudflare: API error: %s", msg)
	}
	if out != nil {
		if err := json.Unmarshal(env.Result, out); err != nil {
			return fmt.Errorf("cloudflare: decode result: %w", err)
		}
	}
	return nil
}

// ListZones returns the zones the token can see (also used to validate a
// token at save time).
func (c *Client) ListZones() ([]Zone, error) {
	var zones []Zone
	if err := c.do("GET", "/zones?per_page=50", nil, &zones); err != nil {
		return nil, err
	}
	return zones, nil
}

// FindZone returns the zone ID whose name is the longest dot-suffix match
// for domain, or ErrZoneNotFound.
func (c *Client) FindZone(domain string) (string, error) {
	zones, err := c.ListZones()
	if err != nil {
		return "", err
	}
	id, ok := matchZone(domain, zones)
	if !ok {
		return "", ErrZoneNotFound
	}
	return id, nil
}

func matchZone(domain string, zones []Zone) (string, bool) {
	bestID, bestLen := "", 0
	for _, z := range zones {
		if (domain == z.Name || strings.HasSuffix(domain, "."+z.Name)) && len(z.Name) > bestLen {
			bestID, bestLen = z.ID, len(z.Name)
		}
	}
	return bestID, bestLen > 0
}

type dnsRecord struct {
	ID      string `json:"id,omitempty"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	Proxied bool   `json:"proxied"`
	TTL     int    `json:"ttl"` // 1 = automatic
}

func (c *Client) findRecord(zoneID, name string) (*dnsRecord, error) {
	var recs []dnsRecord
	path := "/zones/" + zoneID + "/dns_records?name=" + url.QueryEscape(name)
	if err := c.do("GET", path, nil, &recs); err != nil {
		return nil, err
	}
	if len(recs) == 0 {
		return nil, nil
	}
	return &recs[0], nil
}

// UpsertCNAME creates or rewrites the record for name as an unproxied
// (grey-cloud) CNAME to target. Overwriting deliberately repairs a
// misconfigured proxied record.
func (c *Client) UpsertCNAME(zoneID, name, target string) error {
	body := dnsRecord{Type: "CNAME", Name: name, Content: target, Proxied: false, TTL: 1}
	existing, err := c.findRecord(zoneID, name)
	if err != nil {
		return err
	}
	if existing == nil {
		return c.do("POST", "/zones/"+zoneID+"/dns_records", body, nil)
	}
	return c.do("PUT", "/zones/"+zoneID+"/dns_records/"+existing.ID, body, nil)
}

// DeleteRecord removes the record for name ONLY if its content matches
// expectedContent — never destroys a record the user repurposed.
func (c *Client) DeleteRecord(zoneID, name, expectedContent string) error {
	existing, err := c.findRecord(zoneID, name)
	if err != nil {
		return err
	}
	if existing == nil || existing.Content != expectedContent {
		return nil
	}
	return c.do("DELETE", "/zones/"+zoneID+"/dns_records/"+existing.ID, nil, nil)
}
```

- [ ] **Step 4: Run, expect PASS** + build + vet.
- [ ] **Step 5: Commit** via commit-organizer: `feat(gateway): add minimal Cloudflare DNS client`.

---

### Task 3: Router — token store + verified_at stamp

**Files:** Modify `edd-gateway/internal/router/router.go`; Test additions in `edd-gateway/internal/router/customdomains_test.go`

- [ ] **Step 1: Failing DB-gated test** (append to customdomains_test.go)

```go
func TestCloudflareTokenStore(t *testing.T) {
	r := testRouter(t)
	if _, err := r.GetCloudflareToken("u_tok"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if err := r.SetCloudflareToken("u_tok", []byte{1, 2, 3}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	ct, err := r.GetCloudflareToken("u_tok")
	if err != nil || len(ct) != 3 {
		t.Fatalf("Get: %v %v", err, ct)
	}
	// upsert overwrites
	if err := r.SetCloudflareToken("u_tok", []byte{9}); err != nil {
		t.Fatalf("Set2: %v", err)
	}
	ct, _ = r.GetCloudflareToken("u_tok")
	if len(ct) != 1 || ct[0] != 9 {
		t.Fatalf("upsert failed: %v", ct)
	}
	if err := r.DeleteCloudflareToken("u_tok"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := r.GetCloudflareToken("u_tok"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}
```

- [ ] **Step 2: Run** — `go build ./...` must FAIL (undefined methods).

- [ ] **Step 3: Implement.** In `New`, after the `custom_domains` DDL block:

```go
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS user_cloudflare_tokens (
			user_id          TEXT PRIMARY KEY,
			token_ciphertext BYTEA NOT NULL,
			created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`); err != nil {
		db.Close()
		return nil, fmt.Errorf("create user_cloudflare_tokens table: %w", err)
	}
```

Append methods:

```go
// GetCloudflareToken returns the sealed Cloudflare token for a user.
func (r *Router) GetCloudflareToken(userID string) ([]byte, error) {
	var ct []byte
	err := r.db.QueryRow(`SELECT token_ciphertext FROM user_cloudflare_tokens WHERE user_id = $1`, userID).Scan(&ct)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return ct, err
}

// SetCloudflareToken upserts a user's sealed Cloudflare token.
func (r *Router) SetCloudflareToken(userID string, ciphertext []byte) error {
	_, err := r.db.Exec(`
		INSERT INTO user_cloudflare_tokens (user_id, token_ciphertext) VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET token_ciphertext = EXCLUDED.token_ciphertext, created_at = now()
	`, userID, ciphertext)
	if err != nil {
		return fmt.Errorf("set cloudflare token: %w", err)
	}
	return nil
}

// DeleteCloudflareToken removes a user's sealed Cloudflare token.
func (r *Router) DeleteCloudflareToken(userID string) error {
	_, err := r.db.Exec(`DELETE FROM user_cloudflare_tokens WHERE user_id = $1`, userID)
	if err != nil {
		return fmt.Errorf("delete cloudflare token: %w", err)
	}
	return nil
}
```

Also change the `CreateCustomDomain` INSERT so an already-verified insert stamps `verified_at`:

```go
	_, err := r.db.Exec(`
		INSERT INTO custom_domains (id, user_id, container_id, domain, target_port, verify_token, status, verified_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, CASE WHEN $7 = 'verified' THEN now() END)
	`, cd.ID, cd.UserID, cd.ContainerID, cd.Domain, cd.TargetPort, cd.VerifyToken, cd.Status)
```

(unique-violation mapping + `r.reload()` unchanged.)

- [ ] **Step 4: Verify** — `go build ./...` passes; DB-gated tests pass or skip.
- [ ] **Step 5: Commit** via commit-organizer: `feat(gateway): add user Cloudflare token store and verified_at stamping`.

---

### Task 4: API — token endpoints, createDomain automation, main.go wiring

**Files:** Create `edd-gateway/internal/api/cloudflare_token.go`; Modify `edd-gateway/internal/api/server.go`, `edd-gateway/main.go`

- [ ] **Step 1: Server struct + constructor** (server.go). Add imports `"eddisonso.com/edd-gateway/internal/cloudflare"`, `"eddisonso.com/edd-gateway/internal/secretbox"` and:

```go
// ingressTarget is the stable host custom-domain CNAMEs point at (covered by
// the DDNS-maintained *.cloud.eddisonso.com wildcard).
const ingressTarget = "ingress.cloud.eddisonso.com"
```

Server struct gains:

```go
	box *secretbox.Box // nil = Cloudflare token integration disabled
```

New signature:

```go
func New(r *router.Router, v *auth.SessionValidator, newID idGen, preIssue func(string), box *secretbox.Box) *Server {
	return &Server{router: r, validator: v, newID: newID, preIssue: preIssue, box: box}
}
```

`newCFClient` is a seam for tests:

```go
// newCFClient builds a Cloudflare client from a plaintext token; overridable in tests.
var newCFClient = func(token string) *cloudflare.Client { return cloudflare.New(token) }

// userCF returns a Cloudflare client for the user's stored token, or nil if
// the integration is disabled, no token is stored, or decryption fails.
func (s *Server) userCF(userID string) *cloudflare.Client {
	if s.box == nil {
		return nil
	}
	ct, err := s.router.GetCloudflareToken(userID)
	if err != nil {
		return nil // ErrNotFound or DB error -> manual flow
	}
	tok, err := s.box.Open(ct)
	if err != nil {
		slog.Warn("cloudflare token decryption failed (key rotated?)", "user", userID)
		return nil
	}
	return newCFClient(string(tok))
}
```

- [ ] **Step 2: Token endpoints** — create `cloudflare_token.go`:

```go
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"eddisonso.com/edd-gateway/internal/router"
)

// handleCloudflareToken serves PUT/GET/DELETE /api/cloudflare-token.
func (s *Server) handleCloudflareToken(w http.ResponseWriter, r *http.Request, userID string) {
	if s.box == nil {
		http.Error(w, "cloudflare integration not configured", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodPut:
		var req struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" {
			http.Error(w, "token is required", http.StatusBadRequest)
			return
		}
		// Validate the token by listing zones with it before storing.
		zones, err := newCFClient(req.Token).ListZones()
		if err != nil {
			http.Error(w, "token invalid or lacks zone access", http.StatusBadRequest)
			return
		}
		if err := s.router.SetCloudflareToken(userID, s.box.Seal([]byte(req.Token))); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		slog.Info("cloudflare token connected", "user", userID, "zones", len(zones))
		writeJSON(w, http.StatusOK, map[string]any{"configured": true, "zones": zoneNames(zones)})
	case http.MethodGet:
		cf := s.userCF(userID)
		if cf == nil {
			writeJSON(w, http.StatusOK, map[string]any{"configured": false})
			return
		}
		zones, err := cf.ListZones()
		if err != nil {
			// Token stored but no longer working (revoked?). Report configured
			// with no zones so the UI can suggest reconnecting.
			writeJSON(w, http.StatusOK, map[string]any{"configured": true, "zones": []string{}})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"configured": true, "zones": zoneNames(zones)})
	case http.MethodDelete:
		if err := s.router.DeleteCloudflareToken(userID); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func zoneNames(zs []cloudflareZoneList) []string { return nil } // replaced below
```

NOTE: implement `zoneNames` properly (the stub above is illustrative of placement only — final code is):

```go
func zoneNames(zs []cloudflare.Zone) []string {
	names := make([]string, 0, len(zs))
	for _, z := range zs {
		names = append(names, z.Name)
	}
	return names
}
```

with import `"eddisonso.com/edd-gateway/internal/cloudflare"` (and drop the `router` import if unused).

Register in `Handler()` (server.go):

```go
	mux.HandleFunc("/api/cloudflare-token", s.auth(s.handleCloudflareToken))
```

- [ ] **Step 3: createDomain automation branch** (server.go). After the ownership check, before building `cd`:

```go
	status := "pending"
	dnsAutomated := false
	if cf := s.userCF(userID); cf != nil {
		zoneID, err := cf.FindZone(d)
		switch {
		case err == nil:
			if err := cf.UpsertCNAME(zoneID, d, ingressTarget); err == nil {
				status = "verified"
				dnsAutomated = true
			} else {
				slog.Warn("cloudflare CNAME upsert failed; falling back to manual verification", "domain", d, "error", err)
			}
		case !errors.Is(err, cloudflare.ErrZoneNotFound):
			slog.Warn("cloudflare zone lookup failed; falling back to manual verification", "domain", d, "error", err)
		}
	}
```

Change the `cd` literal to `Status: status`. After successful insert:

```go
	if dnsAutomated && s.preIssue != nil {
		s.preIssue(d)
	}
	slog.Info("custom domain created", "domain", d, "user", userID, "container", req.ContainerID, "dns_automated", dnsAutomated)
	resp := toResponse(cd)
	resp.DNSAutomated = dnsAutomated
	writeJSON(w, http.StatusCreated, resp)
```

Add to `domainResponse`:

```go
	DNSAutomated bool `json:"dns_automated,omitempty"`
```

- [ ] **Step 4: Delete cleanup** (server.go, DELETE branch, after `DeleteCustomDomain` succeeds, before the 204):

```go
		if cf := s.userCF(userID); cf != nil {
			if zoneID, err := cf.FindZone(cd.Domain); err == nil {
				if err := cf.DeleteRecord(zoneID, cd.Domain, ingressTarget); err != nil {
					slog.Warn("cloudflare record cleanup failed", "domain", cd.Domain, "error", err)
				}
			}
		}
```

- [ ] **Step 5: main.go.** Where `apiSrv` is built:

```go
	var box *secretbox.Box
	if keyHex := os.Getenv("TOKEN_ENCRYPTION_KEY"); keyHex != "" {
		b, err := secretbox.New(keyHex)
		if err != nil {
			slog.Error("invalid TOKEN_ENCRYPTION_KEY", "error", err)
			os.Exit(1)
		}
		box = b
		slog.Info("Cloudflare token integration enabled")
	}
	apiSrv := api.New(r, validator, newDomainID, preIssue, box)
```

Add import `"eddisonso.com/edd-gateway/internal/secretbox"`.

- [ ] **Step 6: Unit test the endpoint validation seam** (append to `internal/api/server_test.go`): stub `newCFClient` to point at an `httptest` CF stub and test PUT with invalid token → 400 (the stub returns `success:false`), and that the manual path is taken when `box == nil`. Keep tests DB-free: construct `Server{box: ...}` directly only where no router call is reached (the PUT validation path calls `ListZones` BEFORE any DB access, so an invalid-token 400 needs no DB).

```go
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
```

This requires a small exported test constructor in the cloudflare package (add to cloudflare.go):

```go
// NewForTest builds a client against a stub server. Test use only.
func NewForTest(token, baseURL string, h *http.Client) *Client {
	return &Client{token: token, baseURL: baseURL, http: h}
}
```

Imports for the test: `net/http`, `net/http/httptest`, `strings`, plus the cloudflare and secretbox packages.

- [ ] **Step 7: Verify** — `go build ./...`, `go build -o /tmp/gw .`, `go vet ./internal/api/`, `go test ./internal/api/ ./internal/cloudflare/ ./internal/secretbox/` all pass.
- [ ] **Step 8: Commit** via commit-organizer: `feat(gateway): per-user Cloudflare tokens with automatic DNS on domain add` (stage `internal/api/cloudflare_token.go`, `internal/api/server.go`, `internal/api/server_test.go`, `internal/cloudflare/cloudflare.go`, `main.go`).

---

### Task 5: Manifest env

**Files:** Modify `edd-gateway/manifests/gateway.yaml`

- [ ] **Step 1:** Add after `ACME_STAGING` (12-space indent like siblings):

```yaml
            - name: TOKEN_ENCRYPTION_KEY
              valueFrom:
                secretKeyRef:
                  name: gateway-token-key
                  key: TOKEN_ENCRYPTION_KEY
                  optional: true
```

- [ ] **Step 2:** YAML parse check: `python3 -c "import yaml; list(yaml.safe_load_all(open('edd-gateway/manifests/gateway.yaml')))"`.
- [ ] **Step 3: Commit** via commit-organizer: `chore(gateway): add optional TOKEN_ENCRYPTION_KEY for Cloudflare token storage`.

---

### Task 6: Frontend — Cloudflare card + automation feedback

**Files:**
- Create: `edd-cloud-interface/frontend/src/components/networking/CloudflareCard.tsx`
- Modify: `frontend/src/types/domain.ts`, `frontend/src/hooks/useCustomDomains.ts`, `frontend/src/components/networking/AddDomainForm.tsx`, `frontend/src/pages/NetworkingPage.tsx`

- [ ] **Step 1: Types** (types/domain.ts):

```ts
// add to CustomDomain
  dns_automated?: boolean;

export interface CloudflareStatus {
  configured: boolean;
  zones?: string[];
}
```

- [ ] **Step 2: Hook additions** (useCustomDomains.ts) — add inside the hook:

```ts
  const [cloudflare, setCloudflare] = useState<CloudflareStatus | null>(null);

  const loadCloudflare = useCallback(async () => {
    const res = await fetch(`${base()}/api/cloudflare-token`, { headers: getAuthHeaders() });
    if (res.ok) setCloudflare(await res.json());
  }, []);

  const saveCloudflareToken = useCallback(async (token: string) => {
    const res = await fetch(`${base()}/api/cloudflare-token`, {
      method: "PUT",
      headers: { "Content-Type": "application/json", ...getAuthHeaders() },
      body: JSON.stringify({ token }),
    });
    if (!res.ok) throw new Error((await res.text()) || "Failed to save token");
    const status = (await res.json()) as CloudflareStatus;
    setCloudflare(status);
    return status;
  }, []);

  const deleteCloudflareToken = useCallback(async () => {
    const res = await fetch(`${base()}/api/cloudflare-token`, { method: "DELETE", headers: getAuthHeaders() });
    if (!res.ok) throw new Error("Failed to disconnect");
    setCloudflare({ configured: false });
  }, []);
```

Export them in the hook's return object and import `CloudflareStatus` from `@/types`.

- [ ] **Step 3: CloudflareCard component:**

```tsx
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import type { CloudflareStatus } from "@/types";

export function CloudflareCard({
  status, onSave, onDisconnect,
}: {
  status: CloudflareStatus | null;
  onSave: (token: string) => Promise<CloudflareStatus>;
  onDisconnect: () => Promise<void>;
}) {
  const [token, setToken] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");

  const save = async (e: React.FormEvent) => {
    e.preventDefault();
    setErr("");
    setBusy(true);
    try {
      await onSave(token);
      setToken("");
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="bg-card border border-border rounded-lg p-5 mb-6">
      <h2 className="text-sm font-semibold mb-1">Cloudflare integration</h2>
      <p className="text-xs text-muted-foreground mb-4">
        Connect a Cloudflare API token (scoped to Zone:Read + DNS:Edit on your zone)
        and DNS records are created automatically when you add a domain.
      </p>
      {status?.configured ? (
        <div className="flex items-center justify-between gap-3">
          <p className="text-sm">
            Connected{status.zones && status.zones.length > 0 ? ` — zones: ${status.zones.join(", ")}` : " (token no longer lists zones — consider reconnecting)"}
          </p>
          <Button size="sm" variant="outline" onClick={() => onDisconnect()}>Disconnect</Button>
        </div>
      ) : (
        <form onSubmit={save} className="flex flex-col sm:flex-row gap-2 max-w-xl">
          <Input
            type="password"
            placeholder="Cloudflare API token"
            value={token}
            onChange={(e) => setToken(e.target.value)}
            required
          />
          <Button type="submit" disabled={busy}>{busy ? "Verifying…" : "Connect"}</Button>
        </form>
      )}
      {err && <p className="text-sm text-destructive mt-2">{err}</p>}
    </div>
  );
}
```

- [ ] **Step 4: AddDomainForm feedback** — change the prop type and add a notice:

```tsx
import type { CreateCustomDomainData, CustomDomain } from "@/types";

export function AddDomainForm({ onCreate }: { onCreate: (d: CreateCustomDomainData) => Promise<CustomDomain> }) {
```

Add `const [notice, setNotice] = useState("");` and in submit's try block:

```tsx
      const created = await onCreate({ container_id: containerId, domain, target_port: Number(port) });
      setDomain("");
      setNotice(created?.dns_automated
        ? "DNS configured automatically — your domain is going live."
        : "Domain added — follow the DNS setup instructions below to verify it.");
```

Render after the error: `{notice && <p className="text-sm text-muted-foreground">{notice}</p>}`

- [ ] **Step 5: NetworkingPage** — pull the new hook values, load CF status on mount alongside domains, and render `<CloudflareCard status={cloudflare} onSave={saveCloudflareToken} onDisconnect={deleteCloudflareToken} />` between the add-domain card and the domain list. Import from `@/components/networking/CloudflareCard`.

- [ ] **Step 6: Verify** — `npm run type-check && npm run build` in `frontend/`.
- [ ] **Step 7: Commit** via commit-organizer: `feat(frontend): Cloudflare integration card and automatic-DNS feedback`.

---

### Task 7: Docs

**Files:** Modify `edd-cloud-docs/docs/guides/custom-domains.md`

- [ ] **Step 1:** Add a "Connect Cloudflare (optional)" section: create a CF API token scoped Zone:Read + DNS:Edit on your zone → paste in Networking → Cloudflare integration → adding a domain then configures DNS automatically and skips manual verification. Add the manual-path warning: the traffic record **must be DNS-only — do not proxy** (Cloudflare: grey cloud). Proxied records prevent certificate issuance and cause a redirect loop. Troubleshooting bullet: "redirect loop or no certificate → your DNS record is proxied; switch it to DNS-only."
- [ ] **Step 2:** `npm run build` in `edd-cloud-docs/` passes.
- [ ] **Step 3: Commit** via commit-organizer: `docs: Cloudflare auto-DNS and the no-proxy requirement for custom domains`.

---

### Task 8: Merge, secret, live E2E

- [ ] **Step 1:** Generate + create the encryption key secret (controller can do this — it's a generated key, not a user credential):
`kubectl create secret generic gateway-token-key -n core --from-literal=TOKEN_ENCRYPTION_KEY=$(openssl rand -hex 32)`
- [ ] **Step 2:** Merge `feat/cloudflare-dns` → main (`--no-ff`) via commit-organizer; launch actions-monitor; after deploy succeeds, **bump the gateway manifest image tag to the tag CI built** in a follow-up commit (stale-tag rule).
- [ ] **Step 3:** E2E (user): create a CF token scoped to `eddisonso2.com`, paste it in the Networking tab (expect "Connected — zones: eddisonso2.com"), delete + re-add `resume.eddisonso2.com` → expect `dns_automated`, grey-cloud record in CF (repairing the proxied one), Verified immediately, prod cert, HTTPS 200.
- [ ] **Step 4:** Update `memory/project_custom_domains.md` with the outcome.

---

## Self-review (applied)

- **Spec coverage:** secretbox/key handling (T1, T5, T8), CF client + ListZones validation (T2), token store (T3), token endpoints incl. 503-when-disabled + validate-on-save (T4), auto-verify + verified_at + pre-issue + `dns_automated` (T3, T4), delete cleanup guard (T2, T4), fallback on every failure (T4 `userCF` nil paths + switch), UI card + feedback (T6), docs (T7), E2E (T8). No gaps.
- **Type consistency:** `secretbox.Box{Seal,Open}`, `cloudflare.Client{ListZones,FindZone,UpsertCNAME,DeleteRecord}`, `Zone`, `ErrZoneNotFound`, `NewForTest`, router `Get/Set/DeleteCloudflareToken`, `api.New(..., box)`, `ingressTarget` — consistent across tasks.
- **Placeholder scan:** the one intentionally-marked illustrative stub in T4 Step 2 is immediately followed by the final code. No TBDs.
