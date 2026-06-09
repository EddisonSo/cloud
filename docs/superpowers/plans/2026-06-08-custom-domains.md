# Custom Domains (Bring Your Own Hostname) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a user point their own domain (`abc.com`) at one of their containers — prove ownership via a DNS TXT record, then have edd-gateway route the hostname to the container and serve a valid auto-provisioned Let's Encrypt certificate.

**Architecture:** Everything lives in **edd-gateway** except a thin React tab. The gateway gains: a `custom_domains` table, a JWT-authenticated management API (served on loopback, exposed via an ordinary static route at `net.cloud.eddisonso.com`), a DNS-TXT verification worker, a hostname→container resolver, and on-demand ACME TLS via `certmagic` with a Postgres-backed shared cert store. The dashboard gets a **Networking** tab that calls the gateway API.

**Tech Stack:** Go 1.x (`database/sql` + `lib/pq`, `github.com/golang-jwt/jwt/v5`, `github.com/caddyserver/certmagic`), PostgreSQL, React 18 + TypeScript + Vite + Tailwind.

**Spec:** `docs/superpowers/specs/2026-06-08-custom-domains-design.md`

---

## Conventions for this plan

- **Commits:** This repo MANDATES the `commit-organizer` agent for every commit. **Never run `git commit`/`git push` directly.** Each "Commit" step below means: dispatch the `commit-organizer` agent with the stated message. After any push that changes service code, launch the `actions-monitor` agent.
- **Build order is deliberate:** the on-demand TLS layer (Phase 6) is ~70% of the risk. Phase 6 is built and proven against the **Let's Encrypt staging** endpoint before the data-plane wiring depends on it.
- **Go tests** that need a database use the env var `DATABASE_URL_TEST` (a scratch Postgres). If unset, skip with `t.Skip`. Pure-logic tests need no DB.
- **Frontend has no test runner** (confirmed: no vitest/jest). Frontend tasks are verified with `npm run type-check` and `npm run build`, plus manual smoke. Do not invent a test framework.

## File map

**edd-gateway (new files):**
- `internal/domains/domains.go` — pure helpers: normalize, validate, token gen, TXT match
- `internal/domains/domains_test.go`
- `internal/auth/session.go` — JWT validation (mirror of compute's `SessionValidator`)
- `internal/auth/session_test.go`
- `internal/api/server.go` — management API mux + handlers
- `internal/certstore/postgres.go` — `certmagic.Storage` over Postgres (data + locks)
- `internal/tlsmgr/tlsmgr.go` — certmagic config init, DecisionFunc, pre-issue helper

**edd-gateway (modified):**
- `internal/router/router.go` — `custom_domains` table, `CustomDomain` struct, CRUD methods, in-memory map, `ResolveCustomDomain`
- `internal/proxy/server.go` — keep wildcard cert; `EnableOnDemandTLS`; combined `GetCertificate`
- `internal/proxy/tls.go` — custom-domain branch + `handleCustomDomainTLSTermination`
- `main.go` — start API server, start verification worker, init TLS manager
- `manifests/gateway.yaml` (or `edd-gateway/manifests/gateway.yaml`) — `JWT_SECRET`, `ACME_*` env
- `edd-gateway/manifests/gateway-routes.yaml` — route `net.cloud.eddisonso.com` → loopback API

**edd-cloud-interface frontend (new):**
- `frontend/src/hooks/useCustomDomains.ts`
- `frontend/src/pages/NetworkingPage.tsx`
- `frontend/src/components/networking/AddDomainForm.tsx`
- `frontend/src/components/networking/DomainList.tsx`

**edd-cloud-interface frontend (modified):**
- `frontend/src/types/domain.ts`, `frontend/src/hooks/index.ts`, `frontend/src/pages/index.ts`,
  `frontend/src/App.tsx`, `frontend/src/lib/constants.ts`, `frontend/src/lib/api.ts`

---

# Phase 0 — Dependencies

### Task 0.1: Add certmagic to the gateway module

**Files:**
- Modify: `edd-gateway/go.mod`, `edd-gateway/go.sum`

- [ ] **Step 1: Add the dependencies**

Run (from `edd-gateway/`):
```bash
go get github.com/caddyserver/certmagic@latest
go get github.com/golang-jwt/jwt/v5@v5.3.0
go mod tidy
```
Expected: `go.mod` now lists `github.com/caddyserver/certmagic` and `github.com/golang-jwt/jwt/v5 v5.3.0`.

- [ ] **Step 2: Pin and record the certmagic version**

Run: `go list -m github.com/caddyserver/certmagic`
Record the exact version (e.g. `v0.21.4`). **Note the `OnDemandConfig.DecisionFunc` signature for that version** — recent versions use `func(ctx context.Context, name string) error`; older use `func(name string) error`. Task 6.2 must match this signature.

- [ ] **Step 3: Verify it builds**

Run: `go build ./...`
Expected: success, no errors.

- [ ] **Step 4: Commit**

Commit via the `commit-organizer` agent. Message: `chore(gateway): add certmagic and jwt/v5 dependencies`

---

# Phase 1 — Data model

### Task 1.1: Create the `custom_domains` table and `CustomDomain` type

**Files:**
- Modify: `edd-gateway/internal/router/router.go`

- [ ] **Step 1: Add the `CustomDomain` struct**

After the `Container` struct (router.go:137), add:
```go
// CustomDomain holds a user-claimed domain mapped to a container port.
type CustomDomain struct {
	ID          string
	UserID      string
	ContainerID string
	Domain      string // lowercased, e.g. "abc.com"
	TargetPort  int
	VerifyToken string
	Status      string // pending | verified | active | failed
	CreatedAt   time.Time
	VerifiedAt  sql.NullTime
}
```

- [ ] **Step 2: Add the table DDL in `New`**

In `New` (router.go), right after the `static_routes` `CREATE TABLE IF NOT EXISTS` block (router.go:178), add:
```go
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS custom_domains (
			id           TEXT PRIMARY KEY,
			user_id      TEXT NOT NULL,
			container_id TEXT NOT NULL,
			domain       TEXT NOT NULL UNIQUE,
			target_port  INTEGER NOT NULL,
			verify_token TEXT NOT NULL,
			status       TEXT NOT NULL DEFAULT 'pending',
			created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
			verified_at  TIMESTAMPTZ
		)
	`); err != nil {
		db.Close()
		return nil, fmt.Errorf("create custom_domains table: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_custom_domains_status ON custom_domains(status)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("create custom_domains status index: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_custom_domains_user ON custom_domains(user_id)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("create custom_domains user index: %w", err)
	}
```

- [ ] **Step 3: Add the in-memory map field**

In the `Router` struct (router.go:141-150) add a field after `routes`:
```go
	customDomains map[string]*CustomDomain // lowercased domain -> mapping (verified/active only)
```
And initialize it in `New` where the struct is built (router.go:181-187):
```go
		customDomains: make(map[string]*CustomDomain),
```

- [ ] **Step 4: Verify it builds**

Run (from `edd-gateway/`): `go build ./...`
Expected: success.

- [ ] **Step 5: Commit**

Commit via `commit-organizer`. Message: `feat(gateway): add custom_domains table and CustomDomain type`

---

# Phase 2 — Pure domain logic (TDD)

### Task 2.1: Domain normalize + validate

**Files:**
- Create: `edd-gateway/internal/domains/domains.go`
- Test: `edd-gateway/internal/domains/domains_test.go`

- [ ] **Step 1: Write the failing test**

Create `domains_test.go`:
```go
package domains

import "testing"

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"ABC.com":        "abc.com",
		"  Foo.Bar.io  ": "foo.bar.io",
		"WWW.Example.COM": "www.example.com",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValid(t *testing.T) {
	valid := []string{"abc.com", "www.abc.com", "a-b.example.io", "x.y.z.co.uk"}
	for _, d := range valid {
		if !Valid(d) {
			t.Errorf("Valid(%q) = false, want true", d)
		}
	}
	invalid := []string{"", "nodot", "-abc.com", "abc-.com", "ab..com", "abc.com/path", "*.abc.com", "a b.com"}
	for _, d := range invalid {
		if Valid(d) {
			t.Errorf("Valid(%q) = true, want false", d)
		}
	}
}
```

- [ ] **Step 2: Run, verify it fails**

Run (from `edd-gateway/`): `go test ./internal/domains/`
Expected: FAIL — `undefined: Normalize` / `undefined: Valid`.

- [ ] **Step 3: Implement**

Create `domains.go`:
```go
// Package domains holds pure helpers for custom-domain handling: no DB, no I/O
// except DNS lookups (see VerifyTXT). Keeping these pure makes them unit-testable.
package domains

import (
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
```

- [ ] **Step 4: Run, verify it passes**

Run: `go test ./internal/domains/`
Expected: PASS.

- [ ] **Step 5: Commit**

Commit via `commit-organizer`. Message: `feat(gateway): add domain normalize/validate helpers`

---

### Task 2.2: Verify-token generation

**Files:**
- Modify: `edd-gateway/internal/domains/domains.go`, `edd-gateway/internal/domains/domains_test.go`

- [ ] **Step 1: Write the failing test**

Append to `domains_test.go`:
```go
func TestGenerateToken(t *testing.T) {
	a := GenerateToken()
	b := GenerateToken()
	if len(a) < 32 {
		t.Errorf("token too short: %q (len %d)", a, len(a))
	}
	if a == b {
		t.Errorf("tokens not unique: %q == %q", a, b)
	}
	for _, c := range a {
		ok := (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
		if !ok {
			t.Errorf("token has non-[a-z0-9] char: %q", c)
		}
	}
}
```

- [ ] **Step 2: Run, verify it fails**

Run: `go test ./internal/domains/ -run TestGenerateToken`
Expected: FAIL — `undefined: GenerateToken`.

- [ ] **Step 3: Implement**

Add to `domains.go`:
```go
import (
	"crypto/rand"      // add to the import block
	"encoding/hex"     // add to the import block
	"strings"
)

// GenerateToken returns a random 40-char lowercase-hex token for the
// _edd-verify TXT record. Hex chars are all valid in a TXT value.
func GenerateToken() string {
	b := make([]byte, 20)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
```

- [ ] **Step 4: Run, verify it passes**

Run: `go test ./internal/domains/`
Expected: PASS.

- [ ] **Step 5: Commit**

Commit via `commit-organizer`. Message: `feat(gateway): add verify-token generator`

---

### Task 2.3: TXT-match + verification record name

**Files:**
- Modify: `edd-gateway/internal/domains/domains.go`, `edd-gateway/internal/domains/domains_test.go`

- [ ] **Step 1: Write the failing test**

Append to `domains_test.go`:
```go
func TestVerifyRecordName(t *testing.T) {
	if got := VerifyRecordName("abc.com"); got != "_edd-verify.abc.com" {
		t.Errorf("VerifyRecordName = %q", got)
	}
}

func TestTXTMatches(t *testing.T) {
	token := "deadbeef"
	if !TXTMatches([]string{"other", "deadbeef", "x"}, token) {
		t.Error("expected match")
	}
	if TXTMatches([]string{"other", "x"}, token) {
		t.Error("expected no match")
	}
	// DNS libraries sometimes return values with surrounding whitespace.
	if !TXTMatches([]string{"  deadbeef  "}, token) {
		t.Error("expected trimmed match")
	}
}
```

- [ ] **Step 2: Run, verify it fails**

Run: `go test ./internal/domains/ -run 'TestVerifyRecordName|TestTXTMatches'`
Expected: FAIL — undefined functions.

- [ ] **Step 3: Implement**

Add to `domains.go`:
```go
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
```

- [ ] **Step 4: Run, verify it passes**

Run: `go test ./internal/domains/`
Expected: PASS (all domains tests).

- [ ] **Step 5: Commit**

Commit via `commit-organizer`. Message: `feat(gateway): add TXT verification record name and matcher`

---

# Phase 3 — Custom-domain DB layer & resolver

### Task 3.1: CRUD methods on Router

**Files:**
- Modify: `edd-gateway/internal/router/router.go`
- Test: `edd-gateway/internal/router/customdomains_test.go`

- [ ] **Step 1: Write the failing test (DB integration, gated)**

Create `customdomains_test.go`:
```go
package router

import (
	"os"
	"testing"
)

func testRouter(t *testing.T) *Router {
	dsn := os.Getenv("DATABASE_URL_TEST")
	if dsn == "" {
		t.Skip("DATABASE_URL_TEST not set")
	}
	r, err := New(dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { r.Close() })
	_, _ = r.db.Exec(`DELETE FROM custom_domains WHERE domain LIKE '%.test.invalid'`)
	return r
}

func TestCustomDomainCRUD(t *testing.T) {
	r := testRouter(t)
	cd := &CustomDomain{
		ID: "cd_test1", UserID: "u1", ContainerID: "c1",
		Domain: "a.test.invalid", TargetPort: 8000, VerifyToken: "tok", Status: "pending",
	}
	if err := r.CreateCustomDomain(cd); err != nil {
		t.Fatalf("CreateCustomDomain: %v", err)
	}
	got, err := r.GetCustomDomain("cd_test1")
	if err != nil || got.Domain != "a.test.invalid" {
		t.Fatalf("GetCustomDomain: %v %+v", err, got)
	}
	list, err := r.ListCustomDomainsByUser("u1")
	if err != nil || len(list) != 1 {
		t.Fatalf("ListCustomDomainsByUser: %v len=%d", err, len(list))
	}
	if err := r.SetCustomDomainStatus("cd_test1", "verified", true); err != nil {
		t.Fatalf("SetCustomDomainStatus: %v", err)
	}
	pend, err := r.ListPendingDomains()
	if err != nil {
		t.Fatalf("ListPendingDomains: %v", err)
	}
	for _, p := range pend {
		if p.ID == "cd_test1" {
			t.Fatal("expected cd_test1 to no longer be pending")
		}
	}
	if err := r.DeleteCustomDomain("cd_test1", "u1"); err != nil {
		t.Fatalf("DeleteCustomDomain: %v", err)
	}
}
```

- [ ] **Step 2: Run, verify it fails**

Run: `DATABASE_URL_TEST=postgres://... go test ./internal/router/ -run TestCustomDomainCRUD`
Expected: FAIL — undefined methods (or Skip if no DB; if skipped, still proceed — the build must fail to compile because methods are undefined, so run `go build ./...` and expect failure).

- [ ] **Step 3: Implement the CRUD methods**

Append to `router.go`:
```go
// CreateCustomDomain inserts a new pending custom domain. The UNIQUE(domain)
// constraint surfaces as an error the API maps to "domain already in use".
func (r *Router) CreateCustomDomain(cd *CustomDomain) error {
	_, err := r.db.Exec(`
		INSERT INTO custom_domains (id, user_id, container_id, domain, target_port, verify_token, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, cd.ID, cd.UserID, cd.ContainerID, cd.Domain, cd.TargetPort, cd.VerifyToken, cd.Status)
	if err != nil {
		return fmt.Errorf("insert custom domain: %w", err)
	}
	return r.reload()
}

func scanCustomDomain(s interface{ Scan(...any) error }) (*CustomDomain, error) {
	var cd CustomDomain
	if err := s.Scan(&cd.ID, &cd.UserID, &cd.ContainerID, &cd.Domain, &cd.TargetPort,
		&cd.VerifyToken, &cd.Status, &cd.CreatedAt, &cd.VerifiedAt); err != nil {
		return nil, err
	}
	return &cd, nil
}

const customDomainCols = `id, user_id, container_id, domain, target_port, verify_token, status, created_at, verified_at`

// GetCustomDomain returns one domain by id.
func (r *Router) GetCustomDomain(id string) (*CustomDomain, error) {
	row := r.db.QueryRow(`SELECT `+customDomainCols+` FROM custom_domains WHERE id = $1`, id)
	cd, err := scanCustomDomain(row)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return cd, err
}

// ListCustomDomainsByUser returns all domains owned by a user.
func (r *Router) ListCustomDomainsByUser(userID string) ([]*CustomDomain, error) {
	rows, err := r.db.Query(`SELECT `+customDomainCols+` FROM custom_domains WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*CustomDomain
	for rows.Next() {
		cd, err := scanCustomDomain(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, cd)
	}
	return out, rows.Err()
}

// ListPendingDomains returns domains awaiting DNS verification.
func (r *Router) ListPendingDomains() ([]*CustomDomain, error) {
	rows, err := r.db.Query(`SELECT `+customDomainCols+` FROM custom_domains WHERE status = 'pending'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*CustomDomain
	for rows.Next() {
		cd, err := scanCustomDomain(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, cd)
	}
	return out, rows.Err()
}

// SetCustomDomainStatus updates a domain's status, optionally stamping verified_at.
func (r *Router) SetCustomDomainStatus(id, status string, setVerifiedAt bool) error {
	var err error
	if setVerifiedAt {
		_, err = r.db.Exec(`UPDATE custom_domains SET status = $1, verified_at = now() WHERE id = $2`, status, id)
	} else {
		_, err = r.db.Exec(`UPDATE custom_domains SET status = $1 WHERE id = $2`, status, id)
	}
	if err != nil {
		return fmt.Errorf("update custom domain status: %w", err)
	}
	return r.reload()
}

// DeleteCustomDomain removes a domain, scoped to its owner.
func (r *Router) DeleteCustomDomain(id, userID string) error {
	res, err := r.db.Exec(`DELETE FROM custom_domains WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return fmt.Errorf("delete custom domain: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return r.reload()
}

// ContainerOwner returns the user_id that owns a container, for ownership checks.
func (r *Router) ContainerOwner(containerID string) (string, error) {
	var userID string
	err := r.db.QueryRow(`SELECT user_id FROM containers WHERE id = $1`, containerID).Scan(&userID)
	if err == sql.ErrNoRows {
		return "", ErrNotFound
	}
	return userID, err
}
```

- [ ] **Step 4: Run, verify it passes**

Run: `go build ./...` (expect success), then `DATABASE_URL_TEST=postgres://... go test ./internal/router/ -run TestCustomDomainCRUD` (expect PASS, or Skip if no DB).

- [ ] **Step 5: Commit**

Commit via `commit-organizer`. Message: `feat(gateway): add custom-domain CRUD methods`

---

### Task 3.2: Load custom domains into the in-memory map and resolve them

**Files:**
- Modify: `edd-gateway/internal/router/router.go`
- Test: `edd-gateway/internal/router/customdomains_test.go`

- [ ] **Step 1: Write the failing test**

Append to `customdomains_test.go`:
```go
func TestResolveCustomDomain(t *testing.T) {
	r := testRouter(t)
	// Seed a running container row the resolver can join to.
	_, _ = r.db.Exec(`INSERT INTO containers (id, namespace, external_ip, status, user_id)
		VALUES ('cdc1','compute-u1-cdc1','10.0.0.9','running','u1')
		ON CONFLICT (id) DO UPDATE SET status='running', external_ip='10.0.0.9'`)
	cd := &CustomDomain{ID: "cd_res1", UserID: "u1", ContainerID: "cdc1",
		Domain: "live.test.invalid", TargetPort: 8000, VerifyToken: "t", Status: "verified"}
	if err := r.CreateCustomDomain(cd); err != nil {
		t.Fatalf("create: %v", err)
	}
	c, port, err := r.ResolveCustomDomain("live.test.invalid")
	if err != nil {
		t.Fatalf("ResolveCustomDomain: %v", err)
	}
	if c.Namespace != "compute-u1-cdc1" || port != 8000 {
		t.Fatalf("got ns=%s port=%d", c.Namespace, port)
	}
	if _, _, err := r.ResolveCustomDomain("nope.test.invalid"); err == nil {
		t.Fatal("expected error for unknown domain")
	}
	_ = r.DeleteCustomDomain("cd_res1", "u1")
}

func TestCustomDomainAllowed(t *testing.T) {
	r := testRouter(t)
	cd := &CustomDomain{ID: "cd_a1", UserID: "u1", ContainerID: "c1",
		Domain: "allow.test.invalid", TargetPort: 8000, VerifyToken: "t", Status: "verified"}
	_ = r.CreateCustomDomain(cd)
	if !r.CustomDomainAllowed("allow.test.invalid") {
		t.Error("expected allowed")
	}
	if r.CustomDomainAllowed("missing.test.invalid") {
		t.Error("expected not allowed")
	}
	_ = r.DeleteCustomDomain("cd_a1", "u1")
}
```

- [ ] **Step 2: Run, verify it fails**

Run: `go build ./...`
Expected: FAIL — `ResolveCustomDomain`/`CustomDomainAllowed` undefined.

- [ ] **Step 3: Populate the map in `reload` and add resolvers**

In `reload()` (router.go), after the static-routes load block and before the atomic swap (router.go:276), add:
```go
	// Load verified/active custom domains
	customDomains := make(map[string]*CustomDomain)
	cdRows, err := r.db.Query(`SELECT ` + customDomainCols + `
		FROM custom_domains WHERE status IN ('verified','active')`)
	if err != nil {
		return fmt.Errorf("query custom domains: %w", err)
	}
	defer cdRows.Close()
	for cdRows.Next() {
		cd, err := scanCustomDomain(cdRows)
		if err != nil {
			return fmt.Errorf("scan custom domain: %w", err)
		}
		customDomains[cd.Domain] = cd
	}
```
Then inside the atomic swap (router.go:277-280), add:
```go
	r.customDomains = customDomains
```

Append the resolver methods to `router.go`:
```go
// ResolveCustomDomain maps a verified custom hostname to its container + target port.
func (r *Router) ResolveCustomDomain(host string) (*Container, int, error) {
	host = strings.ToLower(host)
	r.mu.RLock()
	cd, ok := r.customDomains[host]
	r.mu.RUnlock()
	if !ok {
		return nil, 0, ErrNoRoute
	}
	c, err := r.Resolve(cd.ContainerID)
	if err != nil {
		return nil, 0, err
	}
	return c, cd.TargetPort, nil
}

// CustomDomainAllowed reports whether a hostname is a verified/active custom
// domain. Used as the on-demand TLS issuance allowlist (abuse gate).
func (r *Router) CustomDomainAllowed(host string) bool {
	host = strings.ToLower(host)
	r.mu.RLock()
	_, ok := r.customDomains[host]
	r.mu.RUnlock()
	return ok
}
```

- [ ] **Step 4: Run, verify it passes**

Run: `go build ./...` (success), then the gated tests (PASS or Skip).

- [ ] **Step 5: Commit**

Commit via `commit-organizer`. Message: `feat(gateway): resolve and allowlist verified custom domains`

---

# Phase 4 — JWT validation in the gateway

### Task 4.1: Mirror the compute `SessionValidator`

**Files:**
- Create: `edd-gateway/internal/auth/session.go`
- Test: `edd-gateway/internal/auth/session_test.go`

- [ ] **Step 1: Write the failing test**

Create `session_test.go`:
```go
package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func signToken(t *testing.T, secret, userID string) string {
	claims := JWTClaims{
		UserID:   userID,
		Username: "alice",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := tok.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return s
}

func TestValidateSession(t *testing.T) {
	v := &SessionValidator{jwtSecret: []byte("topsecret")}
	good := signToken(t, "topsecret", "u123")
	claims, err := v.ValidateSession(good)
	if err != nil || claims.UserID != "u123" {
		t.Fatalf("good token: %v %+v", err, claims)
	}
	// ecloud_ prefix must be stripped
	if _, err := v.ValidateSession("ecloud_" + good); err != nil {
		t.Errorf("prefixed token: %v", err)
	}
	// wrong secret rejected
	bad := signToken(t, "othersecret", "u123")
	if _, err := v.ValidateSession(bad); err == nil {
		t.Error("expected rejection of wrong-secret token")
	}
}
```

- [ ] **Step 2: Run, verify it fails**

Run: `go test ./internal/auth/`
Expected: FAIL — undefined `SessionValidator`/`JWTClaims`.

- [ ] **Step 3: Implement (mirror of compute service)**

Create `session.go`:
```go
// Package auth validates the HS256 JWTs issued by edd-cloud-auth. It mirrors
// the compute service's SessionValidator so all services accept the same tokens.
package auth

import (
	"crypto/rand"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// JWTClaims matches edd-cloud-auth's session claims (internal/api/handler.go).
type JWTClaims struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	UserID      string `json:"user_id"`
	jwt.RegisteredClaims
}

// SessionValidator validates tokens against the shared JWT_SECRET.
type SessionValidator struct {
	jwtSecret []byte
}

// NewSessionValidator reads JWT_SECRET from the environment (same K8s secret as auth).
func NewSessionValidator() *SessionValidator {
	return &SessionValidator{jwtSecret: getJWTSecret()}
}

func getJWTSecret() []byte {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			slog.Error("failed to generate fallback JWT secret", "error", err)
			os.Exit(1)
		}
		slog.Warn("JWT_SECRET not set, using random secret (tokens will not validate)")
		return b
	}
	return []byte(secret)
}

// ValidateSession parses and verifies a token, returning its claims.
func (v *SessionValidator) ValidateSession(tokenString string) (*JWTClaims, error) {
	tokenString = strings.TrimPrefix(tokenString, "ecloud_")
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return v.jwtSecret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	return claims, nil
}

// ExtractToken pulls the bearer token from an Authorization header or `token` cookie.
func ExtractToken(authHeader, cookie string) string {
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}
	return cookie
}
```

- [ ] **Step 4: Run, verify it passes**

Run: `go test ./internal/auth/`
Expected: PASS.

- [ ] **Step 5: Commit**

Commit via `commit-organizer`. Message: `feat(gateway): add JWT session validator mirroring auth service`

---

# Phase 5 — Management API

### Task 5.1: API server with authenticated handlers

**Files:**
- Create: `edd-gateway/internal/api/server.go`

- [ ] **Step 1: Write the handler (no unit test — exercised via manual smoke in Task 5.3)**

Create `server.go`:
```go
// Package api serves the authenticated custom-domains management API. It runs on
// loopback; the gateway exposes it via a static route at net.cloud.eddisonso.com.
package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"eddisonso.com/edd-gateway/internal/auth"
	"eddisonso.com/edd-gateway/internal/domains"
	"eddisonso.com/edd-gateway/internal/router"
)

// idGen returns a unique id for a new domain row.
type idGen func() string

// Server is the management API.
type Server struct {
	router    *router.Router
	validator *auth.SessionValidator
	newID     idGen
	// preIssue, if set, kicks off cert issuance when a domain becomes verified.
	preIssue func(domain string)
}

// New builds the API server.
func New(r *router.Router, v *auth.SessionValidator, newID idGen, preIssue func(string)) *Server {
	return &Server{router: r, validator: v, newID: newID, preIssue: preIssue}
}

// Handler returns the HTTP mux for the API.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/domains", s.auth(s.handleDomains))
	mux.HandleFunc("/api/domains/", s.auth(s.handleDomainByID))
	return mux
}

// auth wraps a handler, injecting the validated user id via the request context header.
func (s *Server) auth(next func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var cookie string
		if c, err := r.Cookie("token"); err == nil {
			cookie = c.Value
		}
		tok := auth.ExtractToken(r.Header.Get("Authorization"), cookie)
		if tok == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		claims, err := s.validator.ValidateSession(tok)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r, claims.UserID)
	}
}

type domainResponse struct {
	ID          string `json:"id"`
	Domain      string `json:"domain"`
	ContainerID string `json:"container_id"`
	TargetPort  int    `json:"target_port"`
	Status      string `json:"status"`
	VerifyName  string `json:"verify_name"`
	VerifyToken string `json:"verify_token"`
}

func toResponse(cd *router.CustomDomain) domainResponse {
	return domainResponse{
		ID: cd.ID, Domain: cd.Domain, ContainerID: cd.ContainerID,
		TargetPort: cd.TargetPort, Status: cd.Status,
		VerifyName: domains.VerifyRecordName(cd.Domain), VerifyToken: cd.VerifyToken,
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// handleDomains: GET list, POST create.
func (s *Server) handleDomains(w http.ResponseWriter, r *http.Request, userID string) {
	switch r.Method {
	case http.MethodGet:
		list, err := s.router.ListCustomDomainsByUser(userID)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		out := make([]domainResponse, 0, len(list))
		for _, cd := range list {
			out = append(out, toResponse(cd))
		}
		writeJSON(w, http.StatusOK, map[string]any{"domains": out})
	case http.MethodPost:
		s.createDomain(w, r, userID)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

type createRequest struct {
	ContainerID string `json:"container_id"`
	Domain      string `json:"domain"`
	TargetPort  int    `json:"target_port"`
}

// allowedPort mirrors the compute ingress rules: 80, 443, or 8000-8999.
func allowedPort(p int) bool {
	return p == 80 || p == 443 || (p >= 8000 && p <= 8999)
}

func (s *Server) createDomain(w http.ResponseWriter, r *http.Request, userID string) {
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	d := domains.Normalize(req.Domain)
	if !domains.Valid(d) {
		http.Error(w, "invalid domain", http.StatusBadRequest)
		return
	}
	if !allowedPort(req.TargetPort) {
		http.Error(w, "port must be 80, 443, or 8000-8999", http.StatusBadRequest)
		return
	}
	owner, err := s.router.ContainerOwner(req.ContainerID)
	if err != nil {
		http.Error(w, "container not found", http.StatusNotFound)
		return
	}
	if owner != userID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	cd := &router.CustomDomain{
		ID: s.newID(), UserID: userID, ContainerID: req.ContainerID,
		Domain: d, TargetPort: req.TargetPort,
		VerifyToken: domains.GenerateToken(), Status: "pending",
	}
	if err := s.router.CreateCustomDomain(cd); err != nil {
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
			http.Error(w, "domain already in use", http.StatusConflict)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	slog.Info("custom domain created", "domain", d, "user", userID, "container", req.ContainerID)
	writeJSON(w, http.StatusCreated, toResponse(cd))
}

// handleDomainByID: DELETE /api/domains/{id}, POST /api/domains/{id}/verify.
func (s *Server) handleDomainByID(w http.ResponseWriter, r *http.Request, userID string) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/domains/")
	if rest == "" {
		http.NotFound(w, r)
		return
	}
	if strings.HasSuffix(rest, "/verify") && r.Method == http.MethodPost {
		id := strings.TrimSuffix(rest, "/verify")
		s.verifyNow(w, r, userID, id)
		return
	}
	if r.Method == http.MethodDelete {
		id := rest
		cd, err := s.router.GetCustomDomain(id)
		if err != nil || cd.UserID != userID {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err := s.router.DeleteCustomDomain(id, userID); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

// verifyNow runs an immediate DNS TXT check for one domain.
func (s *Server) verifyNow(w http.ResponseWriter, r *http.Request, userID, id string) {
	cd, err := s.router.GetCustomDomain(id)
	if err != nil || cd.UserID != userID {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	records, _ := lookupTXT(domains.VerifyRecordName(cd.Domain))
	if domains.TXTMatches(records, cd.VerifyToken) {
		if err := s.router.SetCustomDomainStatus(cd.ID, "verified", true); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if s.preIssue != nil {
			s.preIssue(cd.Domain)
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "verified"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "pending",
		"detail": fmt.Sprintf("TXT %s not found or does not match", domains.VerifyRecordName(cd.Domain)),
	})
}
```

- [ ] **Step 2: Add the DNS lookup seam**

Create `edd-gateway/internal/api/dns.go`:
```go
package api

import "net"

// lookupTXT is a package var so tests can stub DNS.
var lookupTXT = net.LookupTXT
```

- [ ] **Step 3: Verify it builds**

Run: `go build ./...`
Expected: success.

- [ ] **Step 4: Commit**

Commit via `commit-organizer`. Message: `feat(gateway): add authenticated custom-domains management API`

---

### Task 5.2: Wire the API server and a loopback route in main.go

**Files:**
- Modify: `edd-gateway/main.go`

- [ ] **Step 1: Add an id generator helper**

In `main.go`, add an import for `crypto/rand` and `encoding/hex`, and a helper:
```go
func newDomainID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "cd_" + hex.EncodeToString(b)
}
```

- [ ] **Step 2: Start the API server on loopback**

In `main()`, after the proxy server is created and before "Mark as ready" (main.go:137), add. (The `tlsMgr.PreIssue` reference is added in Phase 6; for now pass `nil` and replace in Task 6.4.)
```go
	// Management API on loopback; exposed via static route net.cloud.eddisonso.com.
	validator := auth.NewSessionValidator()
	apiSrv := api.New(r, validator, newDomainID, nil)
	go func() {
		slog.Info("management API listening", "addr", "127.0.0.1:9092")
		if err := http.ListenAndServe("127.0.0.1:9092", apiSrv.Handler()); err != nil {
			slog.Error("management API failed", "error", err)
		}
	}()
```
Add imports: `"eddisonso.com/edd-gateway/internal/api"` and `"eddisonso.com/edd-gateway/internal/auth"`.

- [ ] **Step 3: Register the loopback static route at startup**

Still in `main()`, after routes.yaml is loaded (main.go:91), add:
```go
	// Route the management API host to the loopback API server.
	if err := r.RegisterRoute("net.cloud.eddisonso.com", "/", "127.0.0.1:9092", false); err != nil {
		slog.Error("failed to register management API route", "error", err)
	}
```

- [ ] **Step 4: Verify it builds**

Run: `go build ./...`
Expected: success.

- [ ] **Step 5: Commit**

Commit via `commit-organizer`. Message: `feat(gateway): serve management API and route net.cloud.eddisonso.com`

---

### Task 5.3: Add `JWT_SECRET` env + route to manifests; smoke test

**Files:**
- Modify: `edd-gateway/manifests/gateway.yaml` (the gateway Deployment)
- Modify: `edd-gateway/manifests/gateway-routes.yaml`

- [ ] **Step 1: Add JWT_SECRET to the gateway Deployment**

In the gateway container `env:` block, add (referencing the existing auth secret):
```yaml
            - name: JWT_SECRET
              valueFrom:
                secretKeyRef:
                  name: edd-cloud-auth
                  key: JWT_SECRET
```
Note: the `edd-cloud-auth` Secret must exist in the gateway's namespace. If the gateway runs in a different namespace than auth, create the secret there too (`kubectl get secret edd-cloud-auth -n <auth-ns> -o yaml | sed 's/namespace: .*/namespace: <gateway-ns>/' | kubectl apply -f -`). Confirm the namespace before deploy.

- [ ] **Step 2: Document the route in gateway-routes.yaml**

Add under `routes:` (this mirrors the programmatic registration so the ConfigMap stays the source of truth):
```yaml
    - host: net.cloud.eddisonso.com
      path: /
      target: 127.0.0.1:9092
      strip_prefix: false
```

- [ ] **Step 3: Smoke test locally (manual)**

Build and run the gateway locally with `JWT_SECRET=test DATABASE_URL=...`, then:
```bash
# Mint a token with the same secret, or copy one from a logged-in browser session.
curl -s -H "Authorization: Bearer $TOKEN" http://127.0.0.1:9092/api/domains
```
Expected: `{"domains":[]}` (200) with a valid token; `unauthorized` (401) without.

- [ ] **Step 4: Commit**

Commit via `commit-organizer`. Message: `chore(gateway): add JWT_SECRET env and net.cloud route to manifests`. After push, launch `actions-monitor`.

---

# Phase 6 — On-demand TLS (highest risk — build and prove against LE staging)

### Task 6.1: Postgres-backed `certmagic.Storage` (data + distributed lock)

**Files:**
- Create: `edd-gateway/internal/certstore/postgres.go`
- Test: `edd-gateway/internal/certstore/postgres_test.go`

- [ ] **Step 1: Write the failing test (gated on DB)**

Create `postgres_test.go`:
```go
package certstore

import (
	"context"
	"os"
	"testing"

	"github.com/caddyserver/certmagic"
)

func testStore(t *testing.T) *PostgresStorage {
	dsn := os.Getenv("DATABASE_URL_TEST")
	if dsn == "" {
		t.Skip("DATABASE_URL_TEST not set")
	}
	st, err := New(dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestStoreLoadDeleteExists(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	key := "test/key1"
	if err := st.Store(ctx, key, []byte("hello")); err != nil {
		t.Fatalf("Store: %v", err)
	}
	if !st.Exists(ctx, key) {
		t.Fatal("Exists should be true")
	}
	v, err := st.Load(ctx, key)
	if err != nil || string(v) != "hello" {
		t.Fatalf("Load: %v %q", err, v)
	}
	if err := st.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if st.Exists(ctx, key) {
		t.Fatal("Exists should be false after delete")
	}
	if _, err := st.Load(ctx, key); err != certmagic.ErrNotExist(err) && err == nil {
		t.Fatal("Load after delete should error")
	}
}

func TestLockUnlock(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	if err := st.Lock(ctx, "issue:abc.com"); err != nil {
		t.Fatalf("Lock: %v", err)
	}
	if err := st.Unlock(ctx, "issue:abc.com"); err != nil {
		t.Fatalf("Unlock: %v", err)
	}
}

func TestList(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	_ = st.Store(ctx, "p/a", []byte("1"))
	_ = st.Store(ctx, "p/b", []byte("2"))
	keys, err := st.List(ctx, "p/", false)
	if err != nil || len(keys) < 2 {
		t.Fatalf("List: %v %v", err, keys)
	}
	_ = st.Delete(ctx, "p/a")
	_ = st.Delete(ctx, "p/b")
}
```

- [ ] **Step 2: Run, verify it fails**

Run: `go build ./...`
Expected: FAIL — package `certstore` does not exist.

- [ ] **Step 3: Implement the storage**

Create `postgres.go`:
```go
// Package certstore implements certmagic.Storage over PostgreSQL so all gateway
// replicas share one certificate cache and coordinate issuance via a lock table.
// Without shared storage, each replica would issue its own cert and exhaust the
// Let's Encrypt rate limit.
package certstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/caddyserver/certmagic"
	_ "github.com/lib/pq"
)

// PostgresStorage is a certmagic.Storage backed by Postgres.
type PostgresStorage struct {
	db *sql.DB
}

// lockTTL bounds how long a stale lock survives a crashed holder.
const lockTTL = 2 * time.Minute

// New opens the DB and ensures the storage + lock tables exist.
func New(dsn string) (*PostgresStorage, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS certmagic_data (
			key      TEXT PRIMARY KEY,
			value    BYTEA NOT NULL,
			modified TIMESTAMPTZ NOT NULL DEFAULT now()
		)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("create certmagic_data: %w", err)
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS certmagic_locks (
			name    TEXT PRIMARY KEY,
			expires TIMESTAMPTZ NOT NULL
		)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("create certmagic_locks: %w", err)
	}
	return &PostgresStorage{db: db}, nil
}

func (s *PostgresStorage) Close() error { return s.db.Close() }

func (s *PostgresStorage) Store(ctx context.Context, key string, value []byte) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO certmagic_data (key, value, modified) VALUES ($1, $2, now())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, modified = now()
	`, key, value)
	return err
}

func (s *PostgresStorage) Load(ctx context.Context, key string) ([]byte, error) {
	var v []byte
	err := s.db.QueryRowContext(ctx, `SELECT value FROM certmagic_data WHERE key = $1`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return nil, fs.ErrNotExist
	}
	return v, err
}

func (s *PostgresStorage) Delete(ctx context.Context, key string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM certmagic_data WHERE key = $1`, key)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fs.ErrNotExist
	}
	return nil
}

func (s *PostgresStorage) Exists(ctx context.Context, key string) bool {
	var one int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM certmagic_data WHERE key = $1`, key).Scan(&one)
	return err == nil
}

func (s *PostgresStorage) List(ctx context.Context, prefix string, recursive bool) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key FROM certmagic_data WHERE key LIKE $1`, prefix+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		if !recursive {
			// Only return direct children: trim anything past the next "/".
			rest := strings.TrimPrefix(k, prefix)
			if i := strings.IndexByte(rest, '/'); i >= 0 {
				k = prefix + rest[:i]
			}
		}
		keys = append(keys, k)
	}
	return dedupe(keys), rows.Err()
}

func (s *PostgresStorage) Stat(ctx context.Context, key string) (certmagic.KeyInfo, error) {
	var modified time.Time
	var size int64
	err := s.db.QueryRowContext(ctx,
		`SELECT modified, octet_length(value) FROM certmagic_data WHERE key = $1`, key).Scan(&modified, &size)
	if err == sql.ErrNoRows {
		return certmagic.KeyInfo{}, fs.ErrNotExist
	}
	if err != nil {
		return certmagic.KeyInfo{}, err
	}
	return certmagic.KeyInfo{Key: key, Modified: modified, Size: size, IsTerminal: true}, nil
}

// Lock blocks until it acquires the named lock or ctx is cancelled. Stale locks
// (holder crashed) are stolen after lockTTL.
func (s *PostgresStorage) Lock(ctx context.Context, name string) error {
	for {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO certmagic_locks (name, expires) VALUES ($1, $2)
			ON CONFLICT (name) DO UPDATE SET expires = EXCLUDED.expires
			WHERE certmagic_locks.expires < now()
		`, name, time.Now().Add(lockTTL))
		if err != nil {
			return err
		}
		// Did we hold it? Re-read expiry we just (maybe) wrote.
		var expires time.Time
		if err := s.db.QueryRowContext(ctx,
			`SELECT expires FROM certmagic_locks WHERE name = $1`, name).Scan(&expires); err != nil {
			return err
		}
		if time.Until(expires) > lockTTL-5*time.Second {
			return nil // we acquired/refreshed it
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

func (s *PostgresStorage) Unlock(ctx context.Context, name string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM certmagic_locks WHERE name = $1`, name)
	return err
}

func dedupe(in []string) []string {
	seen := make(map[string]bool, len(in))
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
```

Add `"io/fs"` to the import block (used for `fs.ErrNotExist`).

- [ ] **Step 4: Verify the interface is satisfied**

Add to the bottom of `postgres.go`:
```go
// Compile-time check that PostgresStorage implements certmagic.Storage.
var _ certmagic.Storage = (*PostgresStorage)(nil)
```
Run: `go build ./...`
Expected: success. If it fails with "does not implement", the method signatures don't match the pinned certmagic version — adjust to the exact interface from `go doc github.com/caddyserver/certmagic.Storage`.

- [ ] **Step 5: Run tests**

Run: `DATABASE_URL_TEST=postgres://... go test ./internal/certstore/`
Expected: PASS (or Skip without DB).

- [ ] **Step 6: Commit**

Commit via `commit-organizer`. Message: `feat(gateway): add Postgres-backed certmagic storage with distributed lock`

---

### Task 6.2: TLS manager — certmagic config, allowlist DecisionFunc, pre-issue

**Files:**
- Create: `edd-gateway/internal/tlsmgr/tlsmgr.go`

- [ ] **Step 1: Implement the manager**

Create `tlsmgr.go`. **Match `OnDemandConfig.DecisionFunc` to the signature recorded in Task 0.1.** The code below uses the recent `func(ctx, name) error` form:
```go
// Package tlsmgr wires certmagic for on-demand Let's Encrypt issuance, gated by
// the router's verified-domain allowlist, with certs persisted in Postgres.
package tlsmgr

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"os"

	"github.com/caddyserver/certmagic"

	"eddisonso.com/edd-gateway/internal/certstore"
	"eddisonso.com/edd-gateway/internal/router"
)

// Manager owns the certmagic config.
type Manager struct {
	magic *certmagic.Config
}

// New builds the certmagic config. `r` provides the issuance allowlist.
func New(dsn string, r *router.Router) (*Manager, error) {
	store, err := certstore.New(dsn)
	if err != nil {
		return nil, fmt.Errorf("cert store: %w", err)
	}

	cache := certmagic.NewCache(certmagic.CacheOptions{
		GetConfigForCert: func(certmagic.Certificate) (*certmagic.Config, error) {
			return certmagic.New(nil, certmagic.Config{Storage: store}), nil
		},
	})

	cfg := certmagic.New(cache, certmagic.Config{
		Storage: store,
		OnDemand: &certmagic.OnDemandConfig{
			DecisionFunc: func(ctx context.Context, name string) error {
				if r.CustomDomainAllowed(name) {
					return nil
				}
				return fmt.Errorf("domain %q not allowed", name)
			},
		},
	})

	ca := certmagic.LetsEncryptProductionCA
	if os.Getenv("ACME_STAGING") == "true" {
		ca = certmagic.LetsEncryptStagingCA
	}
	issuer := certmagic.NewACMEIssuer(cfg, certmagic.ACMEIssuer{
		CA:     ca,
		Email:  os.Getenv("ACME_EMAIL"),
		Agreed: true,
	})
	cfg.Issuers = []certmagic.Issuer{issuer}

	slog.Info("TLS manager initialized", "ca", ca)
	return &Manager{magic: cfg}, nil
}

// GetCertificate returns certmagic's on-demand certificate callback target.
func (m *Manager) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	return m.magic.GetCertificate(hello)
}

// PreIssue obtains a cert for a freshly-verified domain so the first visit is fast.
func (m *Manager) PreIssue(domain string) {
	go func() {
		ctx := context.Background()
		if err := m.magic.ManageAsync(ctx, []string{domain}); err != nil {
			slog.Warn("pre-issue failed", "domain", domain, "error", err)
		}
	}()
}
```
Note: `certmagic.NewACMEIssuer` / `ManageAsync` / `NewCache` names must match the pinned version — verify with `go doc github.com/caddyserver/certmagic` and adjust if the API differs.

- [ ] **Step 2: Verify it builds**

Run: `go build ./...`
Expected: success. Fix any API name mismatches against the pinned certmagic version now.

- [ ] **Step 3: Commit**

Commit via `commit-organizer`. Message: `feat(gateway): add certmagic TLS manager with allowlist and pre-issue`

---

### Task 6.3: Hook on-demand certs and the custom-domain route into the TLS path

**Files:**
- Modify: `edd-gateway/internal/proxy/server.go`, `edd-gateway/internal/proxy/tls.go`

- [ ] **Step 1: Keep the wildcard cert and add an on-demand hook in server.go**

In `Server` struct (server.go:16-23) add:
```go
	wildcardCert *tls.Certificate
	onDemand     func(*tls.ClientHelloInfo) (*tls.Certificate, error)
```
Change `LoadTLSCert` (server.go:34-48) to retain the cert and install a combined `GetCertificate`:
```go
func (s *Server) LoadTLSCert(certFile, keyFile string) error {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return fmt.Errorf("load TLS cert: %w", err)
	}
	s.wildcardCert = &cert
	s.tlsConfig = &tls.Config{
		MinVersion: tls.VersionTLS12,
		// acme-tls/1 first so TLS-ALPN-01 challenges are negotiated; http/1.1 for normal traffic.
		NextProtos:     []string{"acme-tls/1", "http/1.1"},
		GetCertificate: s.getCertificate,
	}
	slog.Info("loaded TLS certificate", "cert", certFile)
	return nil
}

// EnableOnDemandTLS installs certmagic's on-demand certificate callback for
// non-wildcard (custom) domains.
func (s *Server) EnableOnDemandTLS(onDemand func(*tls.ClientHelloInfo) (*tls.Certificate, error)) {
	s.onDemand = onDemand
}

// getCertificate serves the wildcard cert for platform domains and delegates
// custom domains to certmagic (which also answers acme-tls/1 challenges).
func (s *Server) getCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	name := strings.ToLower(hello.ServerName)
	if s.onDemand != nil && !isPlatformDomain(name) {
		return s.onDemand(hello)
	}
	return s.wildcardCert, nil
}

// isPlatformDomain reports whether a name is covered by the wildcard cert.
func isPlatformDomain(name string) bool {
	return name == "eddisonso.com" ||
		strings.HasSuffix(name, ".eddisonso.com")
}
```
Add `"strings"` to server.go imports.

- [ ] **Step 2: Add the custom-domain branch in handleTLS**

In `handleTLS` (tls.go), inside the `if s.tlsConfig != nil {` block, after the static-route check (tls.go:88-93) and before the passthrough fallback, add:
```go
		// Custom (bring-your-own) domains: terminate TLS (on-demand cert) and route to container.
		if s.router.CustomDomainAllowed(sni) {
			s.handleCustomDomainTLSTermination(conn, header, payload, sni, clientAddr)
			return
		}
```

- [ ] **Step 3: Add the termination handler**

Append to `tls.go`:
```go
// handleCustomDomainTLSTermination terminates TLS for a verified custom domain
// (cert from certmagic) and proxies to the mapped container/port. acme-tls/1
// challenge handshakes are answered by certmagic and then closed.
func (s *Server) handleCustomDomainTLSTermination(rawConn net.Conn, header, payload []byte, sni, clientAddr string) {
	replayConn := &replayConn{Conn: rawConn, replay: append(header, payload...)}
	tlsConn := tls.Server(replayConn, s.tlsConfig)
	if err := tlsConn.Handshake(); err != nil {
		slog.Warn("TLS handshake failed for custom domain", "sni", sni, "error", err, "client", clientAddr)
		rawConn.Close()
		return
	}
	// If this was an ACME TLS-ALPN-01 challenge, certmagic already answered it.
	if tlsConn.ConnectionState().NegotiatedProtocol == "acme-tls/1" {
		slog.Debug("answered acme-tls/1 challenge", "sni", sni)
		tlsConn.Close()
		return
	}

	container, targetPort, err := s.router.ResolveCustomDomain(sni)
	if err != nil {
		slog.Warn("custom domain not resolvable after handshake", "sni", sni, "error", err)
		tlsConn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\nContent-Type: text/plain\r\nConnection: close\r\n\r\nNo route for domain\r\n"))
		tlsConn.Close()
		return
	}
	backendAddr := fmt.Sprintf("lb.%s.svc.cluster.local:%d", container.Namespace, targetPort)
	slog.Info("custom domain TLS terminated, routing to backend", "sni", sni, "target", backendAddr)
	backend, err := net.DialTimeout("tcp", backendAddr, 5*time.Second)
	if err != nil {
		slog.Error("failed to connect to custom domain backend", "sni", sni, "addr", backendAddr, "error", err)
		tlsConn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\nContent-Type: text/plain\r\nConnection: close\r\n\r\nBackend connection failed\r\n"))
		tlsConn.Close()
		return
	}
	proxy(tlsConn, backend, nil)
}
```

- [ ] **Step 4: Verify it builds**

Run: `go build ./...`
Expected: success.

- [ ] **Step 5: Commit**

Commit via `commit-organizer`. Message: `feat(gateway): terminate TLS and route verified custom domains`

---

### Task 6.4: Initialize the TLS manager and wire pre-issue in main.go

**Files:**
- Modify: `edd-gateway/main.go`

- [ ] **Step 1: Construct the manager and enable on-demand TLS**

In `main()`, after `LoadTLSCert` succeeds (main.go:97-103), add:
```go
		tlsMgr, err := tlsmgr.New(dbConnStr, r)
		if err != nil {
			slog.Error("failed to init TLS manager", "error", err)
			os.Exit(1)
		}
		srv.EnableOnDemandTLS(tlsMgr.GetCertificate)
```
Add import `"eddisonso.com/edd-gateway/internal/tlsmgr"`. Declare `tlsMgr` in a scope visible to the API server construction (move the `var tlsMgr *tlsmgr.Manager` above if needed).

- [ ] **Step 2: Pass pre-issue into the API server**

Change the `api.New` call from Task 5.2 to pass the pre-issue hook:
```go
	var preIssue func(string)
	if tlsMgr != nil {
		preIssue = tlsMgr.PreIssue
	}
	apiSrv := api.New(r, validator, newDomainID, preIssue)
```

- [ ] **Step 3: Verify it builds**

Run: `go build ./...`
Expected: success.

- [ ] **Step 4: Prove it against Let's Encrypt staging (manual integration)**

Deploy to a test environment (or run locally on a publicly reachable host) with:
```
ACME_STAGING=true
ACME_EMAIL=you@example.com
JWT_SECRET=<real secret>
DATABASE_URL=<real db>
```
Then: claim a throwaway domain via the API, add the `_edd-verify` TXT, point an A record at the host, hit `https://<domain>`. Verify in logs: TXT verified → `ManageAsync` → TLS-ALPN-01 challenge answered → cert in `certmagic_data`. Confirm the browser gets the (staging) cert and the request proxies to the container.

- [ ] **Step 5: Commit**

Commit via `commit-organizer`. Message: `feat(gateway): initialize on-demand TLS manager and pre-issue on verify`. After push, launch `actions-monitor`.

---

# Phase 7 — DNS verification worker

### Task 7.1: Background poller that flips pending → verified

**Files:**
- Create: `edd-gateway/internal/api/worker.go`
- Modify: `edd-gateway/main.go`

- [ ] **Step 1: Implement the worker**

Create `worker.go`:
```go
package api

import (
	"context"
	"log/slog"
	"time"

	"eddisonso.com/edd-gateway/internal/domains"
	"eddisonso.com/edd-gateway/internal/router"
)

// VerifyWorker polls pending custom domains and verifies their DNS TXT records.
type VerifyWorker struct {
	router   *router.Router
	preIssue func(string)
	interval time.Duration
}

// NewVerifyWorker builds the worker. interval of 0 defaults to 30s.
func NewVerifyWorker(r *router.Router, preIssue func(string), interval time.Duration) *VerifyWorker {
	if interval == 0 {
		interval = 30 * time.Second
	}
	return &VerifyWorker{router: r, preIssue: preIssue, interval: interval}
}

// Run polls until ctx is cancelled.
func (w *VerifyWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.tick()
		}
	}
}

func (w *VerifyWorker) tick() {
	pending, err := w.router.ListPendingDomains()
	if err != nil {
		slog.Error("verify worker: list pending failed", "error", err)
		return
	}
	for _, cd := range pending {
		// Expire stale claims after 7 days.
		if time.Since(cd.CreatedAt) > 7*24*time.Hour {
			if err := w.router.SetCustomDomainStatus(cd.ID, "failed", false); err != nil {
				slog.Error("verify worker: mark failed", "domain", cd.Domain, "error", err)
			}
			continue
		}
		records, _ := lookupTXT(domains.VerifyRecordName(cd.Domain))
		if domains.TXTMatches(records, cd.VerifyToken) {
			if err := w.router.SetCustomDomainStatus(cd.ID, "verified", true); err != nil {
				slog.Error("verify worker: mark verified", "domain", cd.Domain, "error", err)
				continue
			}
			slog.Info("custom domain verified", "domain", cd.Domain)
			if w.preIssue != nil {
				w.preIssue(cd.Domain)
			}
		}
	}
}
```

- [ ] **Step 2: Start the worker in main.go**

In `main()`, after the API server goroutine (Task 5.2), add:
```go
	worker := api.NewVerifyWorker(r, preIssue, 0)
	workerCtx, cancelWorker := context.WithCancel(context.Background())
	defer cancelWorker()
	go worker.Run(workerCtx)
```
Add import `"context"` if not present.

- [ ] **Step 3: Verify it builds**

Run: `go build ./...`
Expected: success.

- [ ] **Step 4: Commit**

Commit via `commit-organizer`. Message: `feat(gateway): add DNS TXT verification worker with 7-day expiry`. After push, launch `actions-monitor`.

---

# Phase 8 — Frontend Networking tab

> No test runner exists. Verify each task with `npm run type-check` (from `edd-cloud-interface/frontend/`) and a manual smoke once deployed.

### Task 8.1: Types + service host mapping

**Files:**
- Modify: `frontend/src/types/domain.ts`, `frontend/src/lib/api.ts`

- [ ] **Step 1: Add types**

In `frontend/src/types/domain.ts` add:
```ts
export interface CustomDomain {
  id: string;
  domain: string;
  container_id: string;
  target_port: number;
  status: "pending" | "verified" | "active" | "failed";
  verify_name: string;
  verify_token: string;
}

export interface CreateCustomDomainData {
  container_id: string;
  domain: string;
  target_port: number;
}
```

- [ ] **Step 2: Map the `networking` service host**

In `frontend/src/lib/api.ts`, find `resolveServiceHost` and add a case so `buildServiceBase("networking")` resolves to `net.cloud.eddisonso.com`, following the existing per-service pattern in that function (mirror how `compute` → `compute.cloud.eddisonso.com` is done).

- [ ] **Step 3: Type-check**

Run (from `frontend/`): `npm run type-check`
Expected: no errors.

- [ ] **Step 4: Commit**

Commit via `commit-organizer`. Message: `feat(frontend): add custom domain types and networking service host`

---

### Task 8.2: Data hook

**Files:**
- Create: `frontend/src/hooks/useCustomDomains.ts`
- Modify: `frontend/src/hooks/index.ts`

- [ ] **Step 1: Implement the hook**

Create `useCustomDomains.ts` (mirrors the existing `useContainers` shape):
```ts
import { useState, useCallback, useEffect, useRef } from "react";
import { buildServiceBase, getAuthHeaders } from "@/lib/api";
import type { CustomDomain, CreateCustomDomainData } from "@/types";

export function useCustomDomains(user: string | null) {
  const [domains, setDomains] = useState<CustomDomain[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const abortRef = useRef<AbortController | null>(null);

  useEffect(() => () => abortRef.current?.abort(), []);

  const base = () => buildServiceBase("networking");

  const loadDomains = useCallback(async (): Promise<CustomDomain[]> => {
    abortRef.current?.abort();
    abortRef.current = new AbortController();
    try {
      setLoading(true);
      setError("");
      const res = await fetch(`${base()}/api/domains`, {
        headers: getAuthHeaders(),
        signal: abortRef.current.signal,
      });
      if (!res.ok) {
        if (res.status === 401) { setError("Sign in to manage domains"); return []; }
        throw new Error("Failed to load domains");
      }
      const payload = await res.json();
      const list: CustomDomain[] = payload.domains || [];
      setDomains(list);
      return list;
    } catch (err) {
      if ((err as Error).name === "AbortError") return [];
      setError((err as Error).message);
      return [];
    } finally {
      setLoading(false);
    }
  }, []);

  const createDomain = useCallback(async (data: CreateCustomDomainData) => {
    const res = await fetch(`${base()}/api/domains`, {
      method: "POST",
      headers: { "Content-Type": "application/json", ...getAuthHeaders() },
      body: JSON.stringify(data),
    });
    if (!res.ok) throw new Error((await res.text()) || "Failed to create domain");
    await loadDomains();
    return res.json();
  }, [loadDomains]);

  const verifyDomain = useCallback(async (id: string) => {
    const res = await fetch(`${base()}/api/domains/${id}/verify`, {
      method: "POST",
      headers: getAuthHeaders(),
    });
    if (!res.ok) throw new Error("Failed to verify domain");
    await loadDomains();
    return res.json();
  }, [loadDomains]);

  const deleteDomain = useCallback(async (id: string) => {
    const res = await fetch(`${base()}/api/domains/${id}`, {
      method: "DELETE",
      headers: getAuthHeaders(),
    });
    if (!res.ok) throw new Error("Failed to delete domain");
    await loadDomains();
  }, [loadDomains]);

  return { domains, loading, error, setError, loadDomains, createDomain, verifyDomain, deleteDomain };
}
```

- [ ] **Step 2: Export it**

In `frontend/src/hooks/index.ts` add:
```ts
export { useCustomDomains } from "./useCustomDomains";
```

- [ ] **Step 3: Type-check**

Run: `npm run type-check`
Expected: no errors.

- [ ] **Step 4: Commit**

Commit via `commit-organizer`. Message: `feat(frontend): add useCustomDomains hook`

---

### Task 8.3: Networking page + sub-components

**Files:**
- Create: `frontend/src/pages/NetworkingPage.tsx`, `frontend/src/components/networking/AddDomainForm.tsx`, `frontend/src/components/networking/DomainList.tsx`
- Modify: `frontend/src/pages/index.ts`

- [ ] **Step 1: DomainList component**

Create `DomainList.tsx`:
```tsx
import type { CustomDomain } from "@/types";
import { Button } from "@/components/ui/button";

const STATUS_LABEL: Record<CustomDomain["status"], string> = {
  pending: "Pending verification",
  verified: "Verified",
  active: "Live",
  failed: "Failed",
};

export function DomainList({
  domains, onVerify, onDelete,
}: {
  domains: CustomDomain[];
  onVerify: (id: string) => void;
  onDelete: (id: string) => void;
}) {
  if (domains.length === 0) {
    return <p className="text-sm text-muted-foreground">No custom domains yet.</p>;
  }
  return (
    <div className="flex flex-col gap-3">
      {domains.map((d) => (
        <div key={d.id} className="border border-border rounded-lg p-4 flex flex-col gap-2">
          <div className="flex items-center justify-between">
            <span className="font-medium">{d.domain}</span>
            <span className="text-xs px-2 py-1 rounded bg-accent">{STATUS_LABEL[d.status]}</span>
          </div>
          <div className="text-xs text-muted-foreground">
            → container {d.container_id} : port {d.target_port}
          </div>
          {d.status === "pending" && (
            <div className="text-xs bg-card border border-border rounded p-2 font-mono">
              <div>Add this DNS TXT record, then click Verify:</div>
              <div className="mt-1">{d.verify_name} TXT {d.verify_token}</div>
              <div className="mt-1">Then point traffic: CNAME {d.domain} → ingress.cloud.eddisonso.com (subdomain) or A → ingress IP (apex)</div>
            </div>
          )}
          <div className="flex gap-2">
            {d.status === "pending" && (
              <Button size="sm" onClick={() => onVerify(d.id)}>Verify now</Button>
            )}
            <Button size="sm" variant="destructive" onClick={() => onDelete(d.id)}>Delete</Button>
          </div>
        </div>
      ))}
    </div>
  );
}
```

- [ ] **Step 2: AddDomainForm component**

Create `AddDomainForm.tsx`:
```tsx
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import type { CreateCustomDomainData } from "@/types";

export function AddDomainForm({ onCreate }: { onCreate: (d: CreateCustomDomainData) => Promise<void> }) {
  const [containerId, setContainerId] = useState("");
  const [domain, setDomain] = useState("");
  const [port, setPort] = useState("8000");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setErr("");
    setBusy(true);
    try {
      await onCreate({ container_id: containerId, domain, target_port: Number(port) });
      setDomain("");
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <form onSubmit={submit} className="flex flex-col gap-3 max-w-lg">
      <Input placeholder="Container ID" value={containerId} onChange={(e) => setContainerId(e.target.value)} required />
      <Input placeholder="abc.com" value={domain} onChange={(e) => setDomain(e.target.value)} required />
      <Input placeholder="Target port" value={port} onChange={(e) => setPort(e.target.value)} required />
      {err && <p className="text-sm text-destructive">{err}</p>}
      <Button type="submit" disabled={busy}>{busy ? "Adding…" : "Add domain"}</Button>
    </form>
  );
}
```

- [ ] **Step 3: NetworkingPage**

Create `NetworkingPage.tsx`:
```tsx
import { useEffect } from "react";
import { PageHeader } from "@/components/ui/page-header";
import { useCustomDomains } from "@/hooks";
import { useAuth } from "@/contexts/AuthContext";
import { AddDomainForm } from "@/components/networking/AddDomainForm";
import { DomainList } from "@/components/networking/DomainList";

export function NetworkingPage() {
  const { user } = useAuth();
  const { domains, error, loadDomains, createDomain, verifyDomain, deleteDomain } = useCustomDomains(user);

  useEffect(() => { if (user) loadDomains(); }, [user, loadDomains]);

  return (
    <div>
      <PageHeader title="Networking" description="Point your own domains at your containers." />
      {error && (
        <div className="bg-destructive/10 border border-destructive/20 rounded-lg px-4 py-3 mb-4">
          <p className="text-sm text-destructive">{error}</p>
        </div>
      )}
      <div className="bg-card border border-border rounded-lg p-5 mb-6">
        <h2 className="text-sm font-semibold mb-4">Add a domain</h2>
        <AddDomainForm onCreate={createDomain} />
      </div>
      <div className="bg-card border border-border rounded-lg p-5">
        <h2 className="text-sm font-semibold mb-4">Your domains</h2>
        <DomainList domains={domains} onVerify={verifyDomain} onDelete={deleteDomain} />
      </div>
    </div>
  );
}
```

- [ ] **Step 4: Export the page**

In `frontend/src/pages/index.ts` add:
```ts
export { NetworkingPage } from "./NetworkingPage";
```

- [ ] **Step 5: Type-check**

Run: `npm run type-check`
Expected: no errors. (If `Input`'s prop names differ, adjust to the real component API in `components/ui/input.tsx`.)

- [ ] **Step 6: Commit**

Commit via `commit-organizer`. Message: `feat(frontend): add Networking page with add/list/verify/delete`

---

### Task 8.4: Register route + nav

**Files:**
- Modify: `frontend/src/App.tsx`, `frontend/src/lib/constants.ts`

- [ ] **Step 1: Add the route**

In `frontend/src/App.tsx`, import `NetworkingPage` and add inside the routes:
```tsx
<Route path="/networking" element={<NetworkingPage />} />
```

- [ ] **Step 2: Add the nav item**

In `frontend/src/lib/constants.ts`, import an icon (e.g. `Globe` from `lucide-react`) and add to `NAV_ITEMS`:
```tsx
{ id: "networking", label: "Networking", icon: Globe, path: "/networking" },
```

- [ ] **Step 3: Type-check + build**

Run: `npm run type-check && npm run build`
Expected: both succeed.

- [ ] **Step 4: Commit**

Commit via `commit-organizer`. Message: `feat(frontend): register Networking tab route and nav item`. After push, launch `actions-monitor`.

---

# Phase 9 — Deploy config & docs

### Task 9.1: ACME env on the gateway Deployment

**Files:**
- Modify: `edd-gateway/manifests/gateway.yaml`

- [ ] **Step 1: Add ACME env vars**

In the gateway container `env:` block add:
```yaml
            - name: ACME_EMAIL
              value: "admin@eddisonso.com"
            - name: ACME_STAGING
              value: "true"   # flip to "false" only after staging issuance is verified end-to-end
```

- [ ] **Step 2: Confirm public reachability**

Verify the gateway already accepts inbound 443 from the internet (it does — it serves `*.cloud.eddisonso.com`). TLS-ALPN-01 needs nothing more. No port 80 ACME wiring is required.

- [ ] **Step 3: Commit**

Commit via `commit-organizer`. Message: `chore(gateway): add ACME email and staging env`. After push, launch `actions-monitor`.

- [ ] **Step 4: Promote to production CA**

After the staging end-to-end test (Task 6.4 / a real domain) issues and serves correctly, set `ACME_STAGING` to `"false"` and re-deploy via CI. **Do not skip staging** — production Let's Encrypt rate limits are unforgiving.

---

### Task 9.2: User docs

**Files:**
- Create: a custom-domains guide page under `edd-cloud-docs/`

- [ ] **Step 1: Write the guide**

Add a docs page covering: add a domain in the Networking tab → create the `_edd-verify` TXT record → point a CNAME (subdomain) or A record (apex) at the ingress → click Verify → HTTPS goes live automatically. Document the allowed ports (80, 443, 8000–8999) and that one domain maps to one container port. Follow the existing docs page structure/frontmatter in `edd-cloud-docs/`.

- [ ] **Step 2: Commit**

Commit via `commit-organizer`. Message: `docs: add custom domains (bring your own hostname) guide`. After push, launch `actions-monitor`.

---

## Self-review notes (already applied)

- **Spec coverage:** DNS-both (docs Task 9.2 + UI hint), TXT verification (Phase 2, 5, 7), container+port target (Task 3, 5), on-demand ACME (Phase 6), gateway-owned management (Phase 5), Networking tab (Phase 8), shared cert storage + lock (Task 6.1), pre-issue-on-verify (Task 6.2/6.4), abuse gate via verified allowlist (Task 3.2 `CustomDomainAllowed` → Task 6.2 DecisionFunc). All covered.
- **Type consistency:** `CustomDomain`, `ResolveCustomDomain`, `CustomDomainAllowed`, `SetCustomDomainStatus`, `ListPendingDomains`, `PreIssue`, `EnableOnDemandTLS`, `getCertificate` names are used identically across tasks.
- **Known version risk:** certmagic API names (`NewACMEIssuer`, `ManageAsync`, `NewCache`, `OnDemandConfig.DecisionFunc` signature, `Storage` interface) must be checked against the version pinned in Task 0.1 and adjusted — flagged at each use.
- **Repo rules honored:** every commit goes through `commit-organizer`; `actions-monitor` launched after service-code pushes; no direct `kubectl`/`git` mutations.
