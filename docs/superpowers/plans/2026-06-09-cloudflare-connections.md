# Cloudflare Multi-Connection + Domains Sub-Tab Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a user keep multiple Cloudflare connections (one token per zone) simultaneously — connect `eddisonso.com` AND `eddisonso2.com`, then map hostnames under both with no disconnect dance — and move the Networking UI under a Domains sub-tab.

**Architecture:** Replace the one-row-per-user `user_cloudflare_tokens` table with `cloudflare_connections` (N per user, zones snapshot stored at connect). `createDomain`/delete pick the connection by longest stored-zone suffix match, then do the live FindZone/Upsert with that token only. `/api/cloudflare-token` is replaced by `/api/cloudflare-connections` (POST/GET/DELETE/{id}, POST {id}/refresh). Frontend: connections-list card + nav sub-tab `/networking/domains`.

**Tech Stack:** Go (lib/pq `pq.Array` for TEXT[]), React/TS, existing secretbox + cloudflare packages (unchanged).

**Spec:** `docs/superpowers/specs/2026-06-09-cloudflare-dns-automation-design.md` (rev 3)

---

## Conventions

Branch `feat/cloudflare-connections` off main. Commits via commit-organizer (explicit per-path `git add`, one dispatch, `.claude/agents/*.md` untracked, branch pushes don't deploy). Go verification from `edd-gateway/`: build, vet, test (DB tests gated on `DATABASE_URL_TEST`); frontend: `npm run type-check && npm run build`.

## File map

- Modify: `edd-gateway/internal/router/router.go` — new table + startup migration + connection CRUD (replaces token CRUD)
- Modify: `edd-gateway/internal/router/customdomains_test.go` — replace token-store test with connections test
- Modify: `edd-gateway/internal/api/cloudflare_token.go` → rename content to connections handlers (file may be renamed `cloudflare_connections.go`)
- Modify: `edd-gateway/internal/api/server.go` — route registration, `userCF` → `cfForDomain(userID, domain)`
- Modify: `edd-gateway/internal/api/server_test.go` — update endpoint tests
- Frontend modify: `types/domain.ts`, `hooks/useCustomDomains.ts`, `components/networking/CloudflareCard.tsx`, `pages/NetworkingPage.tsx`, `App.tsx`, `lib/constants.ts`
- Docs modify: `edd-cloud-docs/docs/guides/custom-domains.md`

---

### Task 1: Router — connections table, migration, CRUD

**Files:** `edd-gateway/internal/router/router.go`, `customdomains_test.go`

- [ ] **Step 1: Failing DB-gated test** — REPLACE `TestCloudflareTokenStore` in customdomains_test.go with:

```go
func TestCloudflareConnections(t *testing.T) {
	r := testRouter(t)
	_, _ = r.db.Exec(`DELETE FROM cloudflare_connections WHERE user_id = 'u_conn'`)
	id1, err := r.AddCloudflareConnection("u_conn", []byte{1}, []string{"eddisonso.com"})
	if err != nil || id1 == "" {
		t.Fatalf("Add1: %v %q", err, id1)
	}
	id2, err := r.AddCloudflareConnection("u_conn", []byte{2}, []string{"eddisonso2.com", "other.io"})
	if err != nil {
		t.Fatalf("Add2: %v", err)
	}
	conns, err := r.ListCloudflareConnections("u_conn")
	if err != nil || len(conns) != 2 {
		t.Fatalf("List: %v len=%d", err, len(conns))
	}
	var c2 *CloudflareConnection
	for i := range conns {
		if conns[i].ID == id2 {
			c2 = &conns[i]
		}
	}
	if c2 == nil || len(c2.Zones) != 2 || c2.Ciphertext[0] != 2 {
		t.Fatalf("conn2 wrong: %+v", c2)
	}
	if err := r.UpdateCloudflareConnectionZones(id2, "u_conn", []string{"eddisonso2.com"}); err != nil {
		t.Fatalf("UpdateZones: %v", err)
	}
	// ownership: wrong user cannot delete
	if err := r.DeleteCloudflareConnection(id1, "someone_else"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound for wrong user, got %v", err)
	}
	if err := r.DeleteCloudflareConnection(id1, "u_conn"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	conns, _ = r.ListCloudflareConnections("u_conn")
	if len(conns) != 1 {
		t.Fatalf("expected 1 after delete, got %d", len(conns))
	}
	_ = r.DeleteCloudflareConnection(id2, "u_conn")
}
```

- [ ] **Step 2:** Confirm red (test compile fails: undefined type/methods).

- [ ] **Step 3: Implement.** In `New()`, REPLACE the `user_cloudflare_tokens` DDL block with:

```go
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS cloudflare_connections (
			id               TEXT PRIMARY KEY,
			user_id          TEXT NOT NULL,
			token_ciphertext BYTEA NOT NULL,
			zones            TEXT[] NOT NULL DEFAULT '{}',
			created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`); err != nil {
		db.Close()
		return nil, fmt.Errorf("create cloudflare_connections table: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_cloudflare_connections_user ON cloudflare_connections(user_id)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("create cloudflare_connections user index: %w", err)
	}
	// One-time migration from the rev-2 single-token table: copy rows
	// (deterministic ids so it is idempotent), then drop the old table.
	var oldTable sql.NullString
	if err := db.QueryRow(`SELECT to_regclass('user_cloudflare_tokens')::text`).Scan(&oldTable); err == nil && oldTable.Valid {
		if _, err := db.Exec(`
			INSERT INTO cloudflare_connections (id, user_id, token_ciphertext, created_at)
			SELECT 'cfc_' || md5(user_id), user_id, token_ciphertext, created_at
			FROM user_cloudflare_tokens
			ON CONFLICT (id) DO NOTHING
		`); err != nil {
			db.Close()
			return nil, fmt.Errorf("migrate cloudflare tokens: %w", err)
		}
		if _, err := db.Exec(`DROP TABLE user_cloudflare_tokens`); err != nil {
			db.Close()
			return nil, fmt.Errorf("drop user_cloudflare_tokens: %w", err)
		}
	}
```

REPLACE the three token CRUD methods (`GetCloudflareToken`/`SetCloudflareToken`/`DeleteCloudflareToken`) with:

```go
// CloudflareConnection is one user-provided Cloudflare token and the zones it
// could see when connected (or last refreshed).
type CloudflareConnection struct {
	ID         string
	UserID     string
	Ciphertext []byte
	Zones      []string
	CreatedAt  time.Time
}

// AddCloudflareConnection stores a sealed token + zones snapshot, returning the id.
func (r *Router) AddCloudflareConnection(userID string, ciphertext []byte, zones []string) (string, error) {
	id := "cfc_" + newRandomHex(12)
	_, err := r.db.Exec(`
		INSERT INTO cloudflare_connections (id, user_id, token_ciphertext, zones)
		VALUES ($1, $2, $3, $4)
	`, id, userID, ciphertext, pq.Array(zones))
	if err != nil {
		return "", fmt.Errorf("add cloudflare connection: %w", err)
	}
	return id, nil
}

// ListCloudflareConnections returns all of a user's connections (with ciphertext,
// for internal use — API handlers must not expose it).
func (r *Router) ListCloudflareConnections(userID string) ([]CloudflareConnection, error) {
	rows, err := r.db.Query(`
		SELECT id, user_id, token_ciphertext, zones, created_at
		FROM cloudflare_connections WHERE user_id = $1 ORDER BY created_at
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CloudflareConnection
	for rows.Next() {
		var c CloudflareConnection
		if err := rows.Scan(&c.ID, &c.UserID, &c.Ciphertext, pq.Array(&c.Zones), &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// UpdateCloudflareConnectionZones re-snapshots a connection's zones (owner-scoped).
func (r *Router) UpdateCloudflareConnectionZones(id, userID string, zones []string) error {
	res, err := r.db.Exec(`UPDATE cloudflare_connections SET zones = $1 WHERE id = $2 AND user_id = $3`,
		pq.Array(zones), id, userID)
	if err != nil {
		return fmt.Errorf("update cloudflare connection zones: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteCloudflareConnection removes one connection, scoped to its owner.
func (r *Router) DeleteCloudflareConnection(id, userID string) error {
	res, err := r.db.Exec(`DELETE FROM cloudflare_connections WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return fmt.Errorf("delete cloudflare connection: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// newRandomHex returns n random bytes hex-encoded.
func newRandomHex(n int) string {
	b := make([]byte, n)
	_, _ = cryptorand.Read(b)
	return hex.EncodeToString(b)
}
```

Imports to add in router.go: `cryptorand "crypto/rand"`, `"encoding/hex"` (md5 already imported; `pq` already a named import).

- [ ] **Step 4:** `go build ./...`, vet, test (skip ok), gofmt clean.
- [ ] **Step 5: Commit** via commit-organizer: `feat(gateway): replace single CF token with multi-connection store (+migration)`.

---

### Task 2: API — connections endpoints + zone-matched automation

**Files:** `edd-gateway/internal/api/cloudflare_token.go` → rename to `cloudflare_connections.go`; `server.go`; `server_test.go`

- [ ] **Step 1: Replace `userCF` in server.go** with stored-zone matching:

```go
// cfForDomain returns a Cloudflare client for the user's connection whose
// stored zones best match the domain (longest dot-suffix wins across all
// connections), or nil when the integration is disabled / nothing matches /
// decryption fails.
func (s *Server) cfForDomain(userID, domain string) *cloudflare.Client {
	if s.box == nil {
		return nil
	}
	conns, err := s.router.ListCloudflareConnections(userID)
	if err != nil {
		return nil
	}
	var best *router.CloudflareConnection
	bestLen := 0
	for i := range conns {
		for _, z := range conns[i].Zones {
			if (domain == z || strings.HasSuffix(domain, "."+z)) && len(z) > bestLen {
				best, bestLen = &conns[i], len(z)
			}
		}
	}
	if best == nil {
		return nil
	}
	tok, err := s.box.Open(best.Ciphertext)
	if err != nil {
		slog.Warn("cloudflare token decryption failed (key rotated?)", "user", userID, "connection", best.ID)
		return nil
	}
	return newCFClient(string(tok))
}
```

In `createDomain`, change `if cf := s.userCF(userID); cf != nil` → `if cf := s.cfForDomain(userID, d); cf != nil`. In the DELETE branch, change `s.userCF(userID)` → `s.cfForDomain(userID, cd.Domain)`. Handler() registers:

```go
	mux.HandleFunc("/api/cloudflare-connections", s.auth(s.handleCloudflareConnections))
	mux.HandleFunc("/api/cloudflare-connections/", s.auth(s.handleCloudflareConnectionByID))
```

(remove the `/api/cloudflare-token` registration).

- [ ] **Step 2: Rewrite the handler file** (`git mv internal/api/cloudflare_token.go internal/api/cloudflare_connections.go`):

```go
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"eddisonso.com/edd-gateway/internal/cloudflare"
	"eddisonso.com/edd-gateway/internal/router"
)

type connectionResponse struct {
	ID        string   `json:"id"`
	Zones     []string `json:"zones"`
	CreatedAt string   `json:"created_at"`
}

func toConnectionResponse(c *router.CloudflareConnection) connectionResponse {
	zones := c.Zones
	if zones == nil {
		zones = []string{}
	}
	return connectionResponse{ID: c.ID, Zones: zones, CreatedAt: c.CreatedAt.UTC().Format("2006-01-02T15:04:05Z")}
}

// handleCloudflareConnections serves GET (list) and POST (add) /api/cloudflare-connections.
func (s *Server) handleCloudflareConnections(w http.ResponseWriter, r *http.Request, userID string) {
	if s.box == nil {
		http.Error(w, "cloudflare integration not configured", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		conns, err := s.router.ListCloudflareConnections(userID)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		out := make([]connectionResponse, 0, len(conns))
		for i := range conns {
			// Lazily backfill zones for rows migrated from the single-token era.
			if len(conns[i].Zones) == 0 {
				if tok, err := s.box.Open(conns[i].Ciphertext); err == nil {
					if zones, err := newCFClient(string(tok)).ListZones(); err == nil {
						names := zoneNames(zones)
						if err := s.router.UpdateCloudflareConnectionZones(conns[i].ID, userID, names); err == nil {
							conns[i].Zones = names
						}
					}
				}
			}
			out = append(out, toConnectionResponse(&conns[i]))
		}
		writeJSON(w, http.StatusOK, map[string]any{"connections": out})
	case http.MethodPost:
		var req struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" {
			http.Error(w, "token is required", http.StatusBadRequest)
			return
		}
		zones, err := newCFClient(req.Token).ListZones()
		if err != nil {
			http.Error(w, "token invalid or lacks zone access", http.StatusBadRequest)
			return
		}
		names := zoneNames(zones)
		id, err := s.router.AddCloudflareConnection(userID, s.box.Seal([]byte(req.Token)), names)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		slog.Info("cloudflare connection added", "user", userID, "connection", id, "zones", len(names))
		writeJSON(w, http.StatusCreated, connectionResponse{ID: id, Zones: names})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleCloudflareConnectionByID serves DELETE /api/cloudflare-connections/{id}
// and POST /api/cloudflare-connections/{id}/refresh.
func (s *Server) handleCloudflareConnectionByID(w http.ResponseWriter, r *http.Request, userID string) {
	if s.box == nil {
		http.Error(w, "cloudflare integration not configured", http.StatusServiceUnavailable)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/cloudflare-connections/")
	if rest == "" {
		http.NotFound(w, r)
		return
	}
	if strings.HasSuffix(rest, "/refresh") && r.Method == http.MethodPost {
		s.refreshConnection(w, userID, strings.TrimSuffix(rest, "/refresh"))
		return
	}
	if r.Method == http.MethodDelete {
		if err := s.router.DeleteCloudflareConnection(rest, userID); err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

// refreshConnection re-snapshots a connection's zones with its stored token.
func (s *Server) refreshConnection(w http.ResponseWriter, userID, id string) {
	conns, err := s.router.ListCloudflareConnections(userID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	var conn *router.CloudflareConnection
	for i := range conns {
		if conns[i].ID == id {
			conn = &conns[i]
		}
	}
	if conn == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	tok, err := s.box.Open(conn.Ciphertext)
	if err != nil {
		http.Error(w, "token unreadable — disconnect and reconnect", http.StatusConflict)
		return
	}
	zones, err := newCFClient(string(tok)).ListZones()
	if err != nil {
		http.Error(w, "token no longer valid — disconnect and reconnect", http.StatusBadGateway)
		return
	}
	names := zoneNames(zones)
	if err := s.router.UpdateCloudflareConnectionZones(id, userID, names); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, connectionResponse{ID: id, Zones: names})
}
```

(`zoneNames` already exists; keep it.)

- [ ] **Step 3: Update server_test.go** — `TestPutCloudflareTokenInvalid` becomes `TestPostCloudflareConnectionInvalid` (POST to `/api/cloudflare-connections`, call `s.handleCloudflareConnections`, expect 400 — still DB-free since validation precedes DB). `TestCloudflareTokenDisabled` becomes a GET against `handleCloudflareConnections` with nil box expecting 503. Add a pure test for the matching logic if extracted; otherwise rely on the router test + endpoint tests.

- [ ] **Step 4:** build, binary build, vet, `go test ./internal/...`, gofmt clean.
- [ ] **Step 5: Commit** via commit-organizer: `feat(gateway): multi-connection Cloudflare API with zone-matched automation`.

---

### Task 3: Frontend — connections card + Domains sub-tab

**Files:** `types/domain.ts`, `hooks/useCustomDomains.ts`, `components/networking/CloudflareCard.tsx`, `pages/NetworkingPage.tsx`, `App.tsx`, `lib/constants.ts`

- [ ] **Step 1: Types** — replace `CloudflareStatus` with:

```ts
export interface CloudflareConnection {
  id: string;
  zones: string[];
  created_at?: string;
}
```

- [ ] **Step 2: Hook** — replace `cloudflare`/`loadCloudflare`/`saveCloudflareToken`/`deleteCloudflareToken` with: `connections: CloudflareConnection[]`, `loadConnections()` (GET, sets from `payload.connections`), `addConnection(token)` (POST, then reload, return created), `removeConnection(id)` (DELETE, reload), `refreshConnection(id)` (POST {id}/refresh, reload). Auto-load alongside domains in the user effect.

- [ ] **Step 3: CloudflareCard** — props `{connections, onAdd, onRemove, onRefresh}`. Renders: a row per connection (`zones.join(", ")` or "no zones visible — refresh or reconnect", Refresh + Disconnect buttons with per-row busy state), then the add form (password input + "Add connection" button, helper text: one token per zone is fine — scope each to Zone:Read + DNS:Edit). Empty state: "No Cloudflare connections — add a token to automate DNS."

- [ ] **Step 4: Sub-tab** — `lib/constants.ts`: networking nav item gains `subItems: [{ id: "domains", label: "Domains", icon: Globe, path: "/networking/domains" }]`. `App.tsx`: `<Route path="/networking" element={<Navigate to="/networking/domains" replace />} />` and `<Route path="/networking/domains" element={<NetworkingPage />} />` (mirror the Compute redirect pattern; import Navigate if needed). Update NetworkingPage breadcrumb/header if it references the old path.

- [ ] **Step 5:** `npm run type-check && npm run build` pass.
- [ ] **Step 6: Commit** via commit-organizer: `feat(frontend): multiple Cloudflare connections and Networking→Domains sub-tab`.

---

### Task 4: Docs touch-up

- [ ] Update `edd-cloud-docs/docs/guides/custom-domains.md`: the Connect Cloudflare section now describes connections (plural) — one scoped token per zone, add as many as you own, refresh re-reads a token's zones; the dashboard path is Networking → Domains. `npm run build` passes. Commit: `docs: multiple Cloudflare connections under Networking → Domains`.

---

### Task 5: Merge + E2E

- [ ] Merge `feat/cloudflare-connections` → main (--no-ff) via commit-organizer; actions-monitor; post-deploy manifest tag bump.
- [ ] E2E (user): existing migrated connection shows with its zones (lazy backfill); add a second connection scoped to `eddisonso.com`; map `x.eddisonso.com` → container and a hostname under `eddisonso2.com` → both auto-verify with both connections present.

---

## Self-review (applied)

- Spec rev-3 coverage: connections table + migration (T1), POST/GET/DELETE/refresh + lazy backfill (T2), zone-matched create/delete (T2 cfForDomain), connections card + sub-tab (T3), docs (T4), acceptance test (T5). No gaps.
- Type consistency: `CloudflareConnection{ID,UserID,Ciphertext,Zones,CreatedAt}`, `Add/List/Update…Zones/DeleteCloudflareConnection`, `cfForDomain`, endpoint paths — consistent across tasks.
- The duplicated suffix-match logic (cloudflare.matchZone vs cfForDomain) is intentional: one matches live `[]Zone` (with IDs), the other stored names; extracting a shared helper would couple packages for ~6 lines.
