# Image Registry Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a self-hosted OCI-compatible container image registry backed by GFS, with auth integration and compute service image picker.

**Architecture:** Separate Go service (`edd-cloud-registry`) implementing the OCI Distribution Spec. Blobs stored in GFS (namespace `core-registry`), metadata in PostgreSQL. Auth delegated to `edd-cloud-auth` via Docker Token Authentication protocol. Compute service augmented with image picker and `imagePullSecret` for private registry pulls.

**Tech Stack:** Go 1.24, GFS SDK, PostgreSQL (lib/pq), NATS (JetStream), Docker Token Auth (JWT/HS256), Kubernetes client-go, React/TypeScript frontend

**Spec:** `docs/superpowers/specs/2026-03-14-image-registry-design.md`

---

## Chunk 1: GFS SDK Enhancement — Configurable Upload Buffer Size

### Task 1: Add `uploadBufferSize` field to GFS SDK

**Files:**
- Modify: `go-gfs/pkg/go-gfs-sdk/client.go` (lines 32-42 for struct, lines 131-136 area for option)
- Modify: `go-gfs/pkg/go-gfs-sdk/dataplane.go` (lines 119-122 for buffer allocation)

- [ ] **Step 1: Add `uploadBufferSize` field to `clientConfig`**

In `go-gfs/pkg/go-gfs-sdk/client.go`, add the field to the struct:

```go
type clientConfig struct {
	dialOptions      []grpc.DialOption
	chunkTimeout     time.Duration
	maxChunkSize     int64
	readConcurrency  int
	uploadBufferSize int64 // Buffer size for double-buffered uploads (0 = use maxChunkSize)
	secretProvider   SecretProvider
	replicaPicker    ReplicaPicker
	enableConnPool   bool
	connPoolMaxIdle  int
	connPoolIdleTime time.Duration
}
```

- [ ] **Step 2: Add `WithUploadBufferSize` option function**

After the existing `WithMaxChunkSize` function (~line 136):

```go
// WithUploadBufferSize sets the per-buffer size for double-buffered uploads.
// This only affects the memory allocated for streaming buffers in AppendFrom,
// not chunk boundary calculations (which still use maxChunkSize).
// Default: 0 (uses maxChunkSize, typically 64MB).
func WithUploadBufferSize(size int64) Option {
	return func(cfg *clientConfig) {
		if size > 0 {
			cfg.uploadBufferSize = size
		}
	}
}
```

- [ ] **Step 3: Add `uploadBufferSize` to `Client` struct and propagate from config**

In `client.go`, add the field to the `Client` struct (around lines 84-104):

```go
uploadBufferSize int64
```

In the `New()` constructor, propagate from config to client:

```go
client := &Client{
    // ... existing fields ...
    uploadBufferSize: cfg.uploadBufferSize,
}
```

Then add the helper method:

```go
// effectiveUploadBufferSize returns the buffer size for upload double-buffering.
func (c *Client) effectiveUploadBufferSize() int64 {
	if c.uploadBufferSize > 0 {
		return c.uploadBufferSize
	}
	return c.maxChunkSize
}
```

- [ ] **Step 4: Modify `AppendFrom` to use configurable buffer size**

In `go-gfs/pkg/go-gfs-sdk/dataplane.go`, change lines 119-122 from:

```go
bufs := [2][]byte{
    make([]byte, int(c.maxChunkSize)),
    make([]byte, int(c.maxChunkSize)),
}
```

To:

```go
bufSize := c.effectiveUploadBufferSize()
bufs := [2][]byte{
    make([]byte, int(bufSize)),
    make([]byte, int(bufSize)),
}
```

- [ ] **Step 5: Test the change**

Run existing GFS tests to ensure backward compatibility (default buffer size unchanged):

```bash
cd go-gfs && go test ./pkg/go-gfs-sdk/... -v
```

Expected: All existing tests pass (default behavior is the same — `uploadBufferSize` is 0, falls back to `maxChunkSize`).

- [ ] **Step 6: Commit**

```bash
git add go-gfs/pkg/go-gfs-sdk/client.go go-gfs/pkg/go-gfs-sdk/dataplane.go
git commit -m "feat(gfs): add WithUploadBufferSize option for configurable upload buffers"
```

---

## Chunk 2: Auth Service — Docker Token Endpoint

### Task 2: Add `/v2/token` endpoint to edd-cloud-auth

**Files:**
- Create: `edd-cloud-auth/internal/api/registry_token.go`
- Modify: `edd-cloud-auth/internal/api/handler.go` (line ~109 to add route)

- [ ] **Step 1: Create the registry token handler file**

Create `edd-cloud-auth/internal/api/registry_token.go`:

```go
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// registryTokenClaims represents OCI-formatted JWT claims for registry auth.
type registryTokenClaims struct {
	Access []registryAccess `json:"access,omitempty"`
	jwt.RegisteredClaims
}

// registryAccess represents a single OCI scope grant.
type registryAccess struct {
	Type    string   `json:"type"`    // "repository"
	Name    string   `json:"name"`    // e.g., "ecloud-auth"
	Actions []string `json:"actions"` // ["pull"], ["push"], ["pull","push"]
}

// registryTokenResponse is the Docker Token Auth response format.
type registryTokenResponse struct {
	Token     string `json:"token"`
	ExpiresIn int    `json:"expires_in"`
	IssuedAt  string `json:"issued_at"`
}

// handleRegistryToken implements GET /v2/token for Docker Token Authentication.
// This endpoint is self-contained — it validates credentials (user or service account),
// checks repository access, and issues a short-lived JWT with OCI-formatted scopes.
func (h *Handler) handleRegistryToken(w http.ResponseWriter, r *http.Request) {
	service := r.URL.Query().Get("service")
	scopeParam := r.URL.Query().Get("scope")

	// Parse requested scopes
	var requestedAccess []registryAccess
	if scopeParam != "" {
		access, err := parseOCIScope(scopeParam)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid scope: %v", err), http.StatusBadRequest)
			return
		}
		requestedAccess = append(requestedAccess, access)
	}

	// Extract Basic auth credentials (Docker CLI sends these)
	username, password, hasAuth := r.BasicAuth()

	var userID string
	var grantedAccess []registryAccess

	if hasAuth && username != "" && password != "" {
		// Try user authentication first
		uid, err := h.authenticateRegistryUser(username, password)
		if err != nil {
			// Try service account authentication
			uid, saID, err := h.authenticateRegistryServiceAccount(username, password)
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			userID = uid
			// Service account: filter access by SA scopes
			grantedAccess = h.filterAccessByServiceAccount(requestedAccess, saID, userID)
		} else {
			userID = uid
			// User: filter access by ownership/visibility
			grantedAccess = h.filterAccessByUser(requestedAccess, userID)
		}
	} else {
		// Anonymous: only public repos, pull only
		grantedAccess = h.filterAccessAnonymous(requestedAccess)
	}

	// Issue JWT with granted access
	now := time.Now()
	expiresIn := 300 // 5 minutes
	claims := registryTokenClaims{
		Access: grantedAccess,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "edd-cloud-auth",
			Subject:   userID,
			Audience:  jwt.ClaimStrings{service},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(expiresIn) * time.Second)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(h.jwtSecret)
	if err != nil {
		http.Error(w, "failed to sign token", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(registryTokenResponse{
		Token:     tokenString,
		ExpiresIn: expiresIn,
		IssuedAt:  now.Format(time.RFC3339),
	})
}

// parseOCIScope parses "repository:name:actions" format.
func parseOCIScope(scope string) (registryAccess, error) {
	parts := strings.SplitN(scope, ":", 3)
	if len(parts) != 3 {
		return registryAccess{}, fmt.Errorf("expected format 'type:name:actions', got %q", scope)
	}
	if parts[0] != "repository" {
		return registryAccess{}, fmt.Errorf("unsupported scope type %q", parts[0])
	}
	actions := strings.Split(parts[2], ",")
	return registryAccess{
		Type:    parts[0],
		Name:    parts[1],
		Actions: actions,
	}, nil
}

// authenticateRegistryUser validates username/password against the user database.
func (h *Handler) authenticateRegistryUser(username, password string) (string, error) {
	user, err := h.db.GetUserByUsername(username)
	if err != nil {
		return "", fmt.Errorf("user not found")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", fmt.Errorf("invalid password")
	}
	return user.UserID, nil
}

// authenticateRegistryServiceAccount validates SA name + ecloud_ API token.
// Returns (userID, serviceAccountID, error).
func (h *Handler) authenticateRegistryServiceAccount(name, token string) (string, string, error) {
	if !strings.HasPrefix(token, "ecloud_") {
		return "", "", fmt.Errorf("not a service account token")
	}

	// Parse the JWT portion of the ecloud_ token
	jwtPart := strings.TrimPrefix(token, "ecloud_")
	parsed, err := jwt.ParseWithClaims(jwtPart, &APITokenClaims{}, func(t *jwt.Token) (interface{}, error) {
		return h.jwtSecret, nil
	})
	if err != nil {
		return "", "", fmt.Errorf("invalid token: %w", err)
	}

	claims, ok := parsed.Claims.(*APITokenClaims)
	if !ok || claims.Type != "api_token" {
		return "", "", fmt.Errorf("invalid token type")
	}

	// Verify the token hash exists in DB (not revoked)
	h256 := sha256.Sum256([]byte(token))
	tokenHash := fmt.Sprintf("%x", h256)
	exists, err := h.db.CheckTokenHash(tokenHash)
	if err != nil || !exists {
		return "", "", fmt.Errorf("token revoked or not found")
	}

	return claims.UserID, claims.ServiceAccountID, nil
}

// filterAccessByUser grants access based on repository ownership and visibility.
func (h *Handler) filterAccessByUser(requested []registryAccess, userID string) []registryAccess {
	var granted []registryAccess
	for _, req := range requested {
		if req.Type != "repository" {
			continue
		}
		// Check repo ownership and visibility
		repo, err := h.db.GetRepositoryByName(req.Name)
		var allowedActions []string
		for _, action := range req.Actions {
			if action == "pull" {
				// Pull allowed if: repo is public, or user owns it, or repo doesn't exist yet
				if err != nil || repo.Visibility == 1 || repo.OwnerID == userID {
					allowedActions = append(allowedActions, action)
				}
			} else if action == "push" {
				// Push allowed if: user owns the repo, or repo doesn't exist yet (will be created)
				if err != nil || repo.OwnerID == userID {
					allowedActions = append(allowedActions, action)
				}
			}
		}
		if len(allowedActions) > 0 {
			granted = append(granted, registryAccess{
				Type:    req.Type,
				Name:    req.Name,
				Actions: allowedActions,
			})
		}
	}
	return granted
}

// filterAccessByServiceAccount filters access based on SA scopes.
// saID is the service account ID extracted from the token claims.
func (h *Handler) filterAccessByServiceAccount(requested []registryAccess, saID, userID string) []registryAccess {
	// Look up SA scopes from database (GetServiceAccountByID already exists)
	sa, err := h.db.GetServiceAccountByID(saID)
	if err != nil || sa.UserID != userID {
		return nil
	}

	var granted []registryAccess
	for _, req := range requested {
		if req.Type != "repository" {
			continue
		}
		// Map OCI actions to internal scope format and check
		var allowedActions []string
		for _, action := range req.Actions {
			internalAction := "read"
			if action == "push" {
				internalAction = "delete" // push requires write-level access
			}
			scope := fmt.Sprintf("storage.%s.registry.%s", userID, req.Name)
			if hasPermission(sa.Scopes, scope, internalAction) {
				allowedActions = append(allowedActions, action)
			}
		}
		if len(allowedActions) > 0 {
			granted = append(granted, registryAccess{
				Type:    req.Type,
				Name:    req.Name,
				Actions: allowedActions,
			})
		}
	}
	return granted
}

// filterAccessAnonymous grants pull-only access to public repositories.
func (h *Handler) filterAccessAnonymous(requested []registryAccess) []registryAccess {
	var granted []registryAccess
	for _, req := range requested {
		if req.Type != "repository" {
			continue
		}
		// Only grant pull for public repos — actual visibility check happens at registry
		var pullOnly []string
		for _, a := range req.Actions {
			if a == "pull" {
				pullOnly = append(pullOnly, a)
			}
		}
		if len(pullOnly) > 0 {
			granted = append(granted, registryAccess{
				Type:    req.Type,
				Name:    req.Name,
				Actions: pullOnly,
			})
		}
	}
	return granted
}
```

- [ ] **Step 2: Add helper functions needed by the handler**

The handler references `verifyPassword`, `hashToken`, `hasPermission`, and DB methods. Some may already exist. Check and add what's missing.

`verifyPassword` — likely exists as `bcrypt.CompareHashAndPassword` wrapper in auth.go. Reuse it.

`hashToken` — exists in tokens.go for hashing ecloud_ tokens. Reuse it.

`hasPermission` — scope checking function. May need to be added or reused from existing scope validation.

`h.db.CheckTokenHash(hash)` — new DB method:

Add to `edd-cloud-auth/internal/db/api_tokens.go`:

```go
// CheckTokenHash returns true if a token with this hash exists and is not expired.
func (d *DB) CheckTokenHash(hash string) (bool, error) {
	var exists bool
	err := d.db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM api_tokens
			WHERE token_hash = $1
			AND (expires_at = 0 OR expires_at > $2)
		)
	`, hash, time.Now().Unix()).Scan(&exists)
	return exists, err
}
```

`h.db.GetServiceAccountByID(saID)` — already exists in `edd-cloud-auth/internal/db/service_accounts.go` (line 78). No new DB method needed.

`h.db.GetRepositoryByName(name)` — new DB method for access control. Add to `edd-cloud-auth/internal/db/repositories.go`:

```go
type Repository struct {
	ID         int
	Name       string
	OwnerID    string
	Visibility int
}

// GetRepositoryByName queries the registry repositories table.
// Note: This requires the auth service to have read access to the registry DB,
// or the registry exposes an internal API for access checks.
// For v1, the auth service queries the same PostgreSQL database.
func (d *DB) GetRepositoryByName(name string) (*Repository, error) {
	var r Repository
	err := d.db.QueryRow(
		`SELECT id, name, owner_id, visibility FROM repositories WHERE name = $1`, name,
	).Scan(&r.ID, &r.Name, &r.OwnerID, &r.Visibility)
	if err != nil {
		return nil, err
	}
	return &r, nil
}
```

- [ ] **Step 3: Register the route**

In `edd-cloud-auth/internal/api/handler.go`, add after the existing routes (~line 109):

```go
// Docker Token Authentication for container registry
mux.HandleFunc("GET /v2/token", h.handleRegistryToken)
```

- [ ] **Step 4: Test the endpoint**

Run existing auth tests plus manual verification:

```bash
cd edd-cloud-auth && go build ./...
```

Expected: Compiles successfully.

- [ ] **Step 5: Commit**

```bash
git add edd-cloud-auth/internal/api/registry_token.go edd-cloud-auth/internal/api/handler.go edd-cloud-auth/internal/db/api_tokens.go edd-cloud-auth/internal/db/service_accounts.go
git commit -m "feat(auth): add /v2/token endpoint for Docker Token Authentication"
```

---

## Chunk 3: Registry Service — Project Scaffolding & Database

### Task 3: Scaffold the registry service project

**Files:**
- Create: `edd-cloud-interface/services/registry/go.mod`
- Create: `edd-cloud-interface/services/registry/main.go`
- Create: `edd-cloud-interface/services/registry/Dockerfile`

- [ ] **Step 1: Create `go.mod`**

```
module eddisonso.com/edd-cloud-registry

go 1.24

require (
	eddisonso.com/go-gfs v0.0.0
	github.com/golang-jwt/jwt/v5 v5.3.0
	github.com/lib/pq v1.10.9
)

replace (
	eddisonso.com/go-gfs => ../../../go-gfs
)
```

Note: NATS/events and notification-service dependencies will be added in Task 9 when event publishing is implemented. Adding them now would cause `go mod tidy` to remove them as unused.

Run `go mod tidy` after creating.

- [ ] **Step 2: Create `main.go` with server skeleton**

```go
package main

import (
	"context"
	"database/sql"
	"flag"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	gfs "eddisonso.com/go-gfs/pkg/go-gfs-sdk"
	_ "github.com/lib/pq"
)

const gfsNamespace = "core-registry"

type server struct {
	gfs       *gfs.Client
	db        *sql.DB
	jwtSecret []byte
}

func main() {
	addr := flag.String("addr", "0.0.0.0:8080", "listen address")
	master := flag.String("master", "gfs-master:9000", "GFS master address")
	flag.Parse()

	// Database
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	if err := db.Ping(); err != nil {
		log.Fatalf("failed to ping database: %v", err)
	}
	defer db.Close()

	if err := initDB(db); err != nil {
		log.Fatalf("failed to init database: %v", err)
	}

	// GFS client with small upload buffers for registry workload
	ctx := context.Background()
	gfsClient, err := gfs.New(ctx, *master,
		gfs.WithConnectionPool(8, 60*time.Second),
		gfs.WithUploadBufferSize(64*1024), // 64KB per buffer
	)
	if err != nil {
		log.Fatalf("failed to connect to gfs master: %v", err)
	}
	defer gfsClient.Close()

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET is required")
	}

	srv := &server{
		gfs:       gfsClient,
		db:        db,
		jwtSecret: []byte(jwtSecret),
	}

	mux := http.NewServeMux()
	srv.registerRoutes(mux)

	httpServer := &http.Server{
		Addr:    *addr,
		Handler: mux,
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		slog.Info("shutting down")
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		httpServer.Shutdown(shutCtx)
	}()

	slog.Info("starting registry", "addr", *addr)
	if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

func (s *server) registerRoutes(mux *http.ServeMux) {
	// Single catch-all — OCI repo names can contain slashes
	mux.HandleFunc("/v2/", s.routeV2)
}

// routeV2 dispatches all /v2/ requests. Placeholder — will be fully implemented in Task 6.
func (s *server) routeV2(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/v2/" && r.Method == http.MethodGet {
		w.Header().Set("Docker-Distribution-API-Version", "registry/2.0")
		w.WriteHeader(http.StatusOK)
		return
	}
	http.NotFound(w, r)
}
```

- [ ] **Step 3: Create `Dockerfile`**

Follow the SFS Dockerfile pattern (`edd-cloud-interface/services/sfs/Dockerfile`):

```dockerfile
FROM golang:1.24.11 AS builder

RUN apt-get update && apt-get install -y protobuf-compiler && \
    go install google.golang.org/protobuf/cmd/protoc-gen-go@latest && \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

WORKDIR /src

# Copy dependencies
COPY go-gfs/ /go-gfs/
COPY proto/ /proto/
COPY notification-service/ /notification-service/
COPY edd-cloud-interface/pkg/ /pkg/

# Copy service source
COPY edd-cloud-interface/services/registry/go.mod edd-cloud-interface/services/registry/go.sum /src/

# Fix local replace paths for Docker build context
RUN sed -i 's|../../../go-gfs|/go-gfs|' go.mod && \
    sed -i 's|../../../notification-service|/notification-service|' go.mod && \
    sed -i 's|../../../pkg/events|/pkg/events|' go.mod

RUN go mod download
COPY edd-cloud-interface/services/registry/*.go /src/
RUN CGO_ENABLED=0 go build -o /registry .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /registry /usr/local/bin/registry
ENTRYPOINT ["registry"]
```

- [ ] **Step 4: Verify it compiles**

```bash
cd edd-cloud-interface/services/registry && go mod tidy && go build ./...
```

Expected: Compiles (with placeholder handlers).

- [ ] **Step 5: Commit**

```bash
git add edd-cloud-interface/services/registry/
git commit -m "feat(registry): scaffold registry service with project structure"
```

### Task 4: Database schema initialization

**Files:**
- Create: `edd-cloud-interface/services/registry/db.go`

- [ ] **Step 1: Create `db.go` with schema init and query methods**

```go
package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

func initDB(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS repositories (
			id SERIAL PRIMARY KEY,
			name TEXT UNIQUE NOT NULL,
			owner_id TEXT,
			visibility INT DEFAULT 0,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS manifests (
			id SERIAL PRIMARY KEY,
			repository_id INT REFERENCES repositories(id) ON DELETE CASCADE,
			digest TEXT NOT NULL,
			media_type TEXT NOT NULL,
			size BIGINT,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(repository_id, digest)
		)`,
		`CREATE TABLE IF NOT EXISTS tags (
			id SERIAL PRIMARY KEY,
			repository_id INT REFERENCES repositories(id) ON DELETE CASCADE,
			name TEXT NOT NULL,
			manifest_digest TEXT NOT NULL,
			updated_at TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(repository_id, name)
		)`,
		`CREATE TABLE IF NOT EXISTS repository_blobs (
			repository_id INT REFERENCES repositories(id) ON DELETE CASCADE,
			digest TEXT NOT NULL,
			size BIGINT,
			gc_marked_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			PRIMARY KEY(repository_id, digest)
		)`,
		`CREATE TABLE IF NOT EXISTS manifest_blobs (
			manifest_id INT REFERENCES manifests(id) ON DELETE CASCADE,
			blob_digest TEXT NOT NULL,
			PRIMARY KEY(manifest_id, blob_digest)
		)`,
		`CREATE TABLE IF NOT EXISTS upload_sessions (
			uuid TEXT PRIMARY KEY,
			repository_id INT REFERENCES repositories(id) ON DELETE CASCADE,
			hash_state BYTEA,
			bytes_received BIGINT DEFAULT 0,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)`,
	}
	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("exec %q: %w", stmt[:40], err)
		}
	}
	return nil
}

// Repository operations

func getOrCreateRepo(ctx context.Context, db *sql.DB, name, ownerID string) (int, error) {
	var id int
	err := db.QueryRowContext(ctx,
		`INSERT INTO repositories (name, owner_id) VALUES ($1, $2)
		 ON CONFLICT (name) DO UPDATE SET updated_at = NOW()
		 RETURNING id`, name, ownerID).Scan(&id)
	return id, err
}

func getRepoByName(ctx context.Context, db *sql.DB, name string) (int, string, int, error) {
	var id int
	var ownerID string
	var visibility int
	err := db.QueryRowContext(ctx,
		`SELECT id, owner_id, visibility FROM repositories WHERE name = $1`, name).
		Scan(&id, &ownerID, &visibility)
	return id, ownerID, visibility, err
}

func listRepos(ctx context.Context, db *sql.DB, ownerID string, includePublic bool, limit, offset int) ([]string, error) {
	query := `SELECT name FROM repositories WHERE owner_id = $1`
	args := []interface{}{ownerID}
	if includePublic {
		query = `SELECT name FROM repositories WHERE owner_id = $1 OR visibility = 1`
	}
	query += ` ORDER BY name LIMIT $2 OFFSET $3`
	args = append(args, limit, offset)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// Blob operations

func insertRepoBlob(ctx context.Context, db *sql.DB, repoID int, digest string, size int64) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO repository_blobs (repository_id, digest, size)
		 VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, repoID, digest, size)
	return err
}

func blobExistsInRepo(ctx context.Context, db *sql.DB, repoID int, digest string) (bool, int64, error) {
	var size int64
	err := db.QueryRowContext(ctx,
		`SELECT size FROM repository_blobs WHERE repository_id = $1 AND digest = $2`,
		repoID, digest).Scan(&size)
	if err == sql.ErrNoRows {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, err
	}
	return true, size, nil
}

func clearBlobGCMark(ctx context.Context, db *sql.DB, repoID int, digest string) error {
	_, err := db.ExecContext(ctx,
		`UPDATE repository_blobs SET gc_marked_at = NULL
		 WHERE repository_id = $1 AND digest = $2`, repoID, digest)
	return err
}

// Manifest operations

func upsertManifest(ctx context.Context, db *sql.DB, repoID int, digest, mediaType string, size int64) (int, error) {
	var id int
	err := db.QueryRowContext(ctx,
		`INSERT INTO manifests (repository_id, digest, media_type, size)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (repository_id, digest) DO UPDATE SET media_type = $3, size = $4
		 RETURNING id`, repoID, digest, mediaType, size).Scan(&id)
	return id, err
}

func getManifest(ctx context.Context, db *sql.DB, repoID int, digest string) (int, string, int64, error) {
	var id int
	var mediaType string
	var size int64
	err := db.QueryRowContext(ctx,
		`SELECT id, media_type, size FROM manifests
		 WHERE repository_id = $1 AND digest = $2`, repoID, digest).
		Scan(&id, &mediaType, &size)
	return id, mediaType, size, err
}

func insertManifestBlobs(ctx context.Context, db *sql.DB, manifestID int, blobDigests []string) error {
	for _, digest := range blobDigests {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO manifest_blobs (manifest_id, blob_digest) VALUES ($1, $2)
			 ON CONFLICT DO NOTHING`, manifestID, digest); err != nil {
			return err
		}
	}
	return nil
}

// Tag operations

func upsertTag(ctx context.Context, db *sql.DB, repoID int, tag, digest string) (string, error) {
	// Returns the old digest if tag was updated (for manifest cleanup)
	var oldDigest sql.NullString
	err := db.QueryRowContext(ctx,
		`SELECT manifest_digest FROM tags WHERE repository_id = $1 AND name = $2`,
		repoID, tag).Scan(&oldDigest)
	if err != nil && err != sql.ErrNoRows {
		return "", err
	}

	_, err = db.ExecContext(ctx,
		`INSERT INTO tags (repository_id, name, manifest_digest)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (repository_id, name) DO UPDATE SET manifest_digest = $3, updated_at = NOW()`,
		repoID, tag, digest)
	if err != nil {
		return "", err
	}

	if oldDigest.Valid && oldDigest.String != digest {
		return oldDigest.String, nil
	}
	return "", nil
}

func getTagDigest(ctx context.Context, db *sql.DB, repoID int, tag string) (string, error) {
	var digest string
	err := db.QueryRowContext(ctx,
		`SELECT manifest_digest FROM tags WHERE repository_id = $1 AND name = $2`,
		repoID, tag).Scan(&digest)
	return digest, err
}

func listTags(ctx context.Context, db *sql.DB, repoID int) ([]string, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT name FROM tags WHERE repository_id = $1 ORDER BY name`, repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}

func manifestHasOtherTags(ctx context.Context, db *sql.DB, repoID int, digest, excludeTag string) (bool, error) {
	var count int
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tags
		 WHERE repository_id = $1 AND manifest_digest = $2 AND name != $3`,
		repoID, digest, excludeTag).Scan(&count)
	return count > 0, err
}

// Upload session operations

func createUploadSession(ctx context.Context, db *sql.DB, uuid string, repoID int) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO upload_sessions (uuid, repository_id) VALUES ($1, $2)`, uuid, repoID)
	return err
}

func getUploadSession(ctx context.Context, db *sql.DB, uuid string) (int, []byte, int64, error) {
	var repoID int
	var hashState []byte
	var bytesReceived int64
	err := db.QueryRowContext(ctx,
		`SELECT repository_id, hash_state, bytes_received FROM upload_sessions WHERE uuid = $1`,
		uuid).Scan(&repoID, &hashState, &bytesReceived)
	return repoID, hashState, bytesReceived, err
}

func updateUploadSession(ctx context.Context, db *sql.DB, uuid string, hashState []byte, bytesReceived int64) error {
	_, err := db.ExecContext(ctx,
		`UPDATE upload_sessions SET hash_state = $1, bytes_received = $2 WHERE uuid = $3`,
		hashState, bytesReceived, uuid)
	return err
}

func deleteUploadSession(ctx context.Context, db *sql.DB, uuid string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM upload_sessions WHERE uuid = $1`, uuid)
	return err
}

func deleteStaleUploadSessions(ctx context.Context, db *sql.DB, olderThan time.Duration) (int64, error) {
	result, err := db.ExecContext(ctx,
		`DELETE FROM upload_sessions WHERE created_at < $1`,
		time.Now().Add(-olderThan))
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// GC operations

type gcOrphanedBlob struct {
	RepoID int
	Digest string
}

func markOrphanedBlobs(ctx context.Context, db *sql.DB) (int64, error) {
	result, err := db.ExecContext(ctx,
		`UPDATE repository_blobs SET gc_marked_at = NOW()
		 WHERE gc_marked_at IS NULL
		 AND (repository_id, digest) NOT IN (
			SELECT rb.repository_id, mb.blob_digest
			FROM manifest_blobs mb
			JOIN manifests m ON m.id = mb.manifest_id
			JOIN repository_blobs rb ON rb.repository_id = m.repository_id AND rb.digest = mb.blob_digest
		 )`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func sweepMarkedBlobs(ctx context.Context, db *sql.DB, markedBefore time.Time) ([]gcOrphanedBlob, error) {
	rows, err := db.QueryContext(ctx,
		`DELETE FROM repository_blobs
		 WHERE gc_marked_at IS NOT NULL AND gc_marked_at < $1
		 RETURNING repository_id, digest`, markedBefore)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var blobs []gcOrphanedBlob
	for rows.Next() {
		var b gcOrphanedBlob
		if err := rows.Scan(&b.RepoID, &b.Digest); err != nil {
			return nil, err
		}
		blobs = append(blobs, b)
	}
	return blobs, rows.Err()
}

```

- [ ] **Step 2: Verify it compiles**

```bash
cd edd-cloud-interface/services/registry && go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add edd-cloud-interface/services/registry/db.go
git commit -m "feat(registry): add database schema and query functions"
```

---

## Chunk 4: Registry Service — Auth Middleware & Blob Upload

### Task 5: Auth middleware for OCI token validation

**Files:**
- Create: `edd-cloud-interface/services/registry/auth.go`

- [ ] **Step 1: Create `auth.go`**

```go
package main

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// registryAccess matches the OCI scope format in JWT claims.
type registryAccess struct {
	Type    string   `json:"type"`
	Name    string   `json:"name"`
	Actions []string `json:"actions"`
}

type registryClaims struct {
	Access []registryAccess `json:"access,omitempty"`
	jwt.RegisteredClaims
}

// authResult holds the identity extracted from a validated registry token.
type authResult struct {
	UserID string
	Access []registryAccess
}

// authenticate validates the registry JWT from Authorization header.
// Returns nil authResult for anonymous requests.
func (s *server) authenticate(r *http.Request) *authResult {
	auth := r.Header.Get("Authorization")
	if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
		return nil
	}
	tokenStr := strings.TrimPrefix(auth, "Bearer ")

	token, err := jwt.ParseWithClaims(tokenStr, &registryClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		return nil
	}
	claims, ok := token.Claims.(*registryClaims)
	if !ok {
		return nil
	}
	return &authResult{
		UserID: claims.Subject,
		Access: claims.Access,
	}
}

// hasAccess checks if the auth result grants the given action on the repository.
func hasAccess(auth *authResult, repoName, action string) bool {
	if auth == nil {
		return false
	}
	for _, a := range auth.Access {
		if a.Type == "repository" && a.Name == repoName {
			for _, act := range a.Actions {
				if act == action {
					return true
				}
			}
		}
	}
	return false
}

// requireAuth returns 401 with WWW-Authenticate challenge header.
func (s *server) requireAuth(w http.ResponseWriter, repoName, action string) {
	scope := ""
	if repoName != "" {
		scope = fmt.Sprintf(",scope=\"repository:%s:%s\"", repoName, action)
	}
	w.Header().Set("WWW-Authenticate",
		fmt.Sprintf(`Bearer realm="https://auth.cloud.eddisonso.com/v2/token",service="registry.cloud.eddisonso.com"%s`, scope))
	w.Header().Set("Docker-Distribution-API-Version", "registry/2.0")
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}
```

- [ ] **Step 2: Verify it compiles**

```bash
cd edd-cloud-interface/services/registry && go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add edd-cloud-interface/services/registry/auth.go
git commit -m "feat(registry): add OCI token auth middleware"
```

### Task 6: Blob upload handlers (POST, PATCH, PUT)

**Files:**
- Create: `edd-cloud-interface/services/registry/blobs.go`
- Modify: `edd-cloud-interface/services/registry/main.go` (add routes)

- [ ] **Step 1: Create `blobs.go`**

```go
package main

import (
	"crypto/sha256"
	"encoding"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// handleBlobHead checks if a blob exists and returns its size.
// HEAD /v2/{name}/blobs/{digest}
func (s *server) handleBlobHead(w http.ResponseWriter, r *http.Request) {
	repoName, digest := s.parseBlobPath(r)
	if repoName == "" || digest == "" {
		http.NotFound(w, r)
		return
	}

	auth := s.authenticate(r)
	if !hasAccess(auth, repoName, "pull") {
		s.requireAuth(w, repoName, "pull")
		return
	}

	repoID, _, _, err := getRepoByName(r.Context(), s.db, repoName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	exists, size, err := blobExistsInRepo(r.Context(), s.db, repoID, digest)
	if err != nil || !exists {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	w.Header().Set("Docker-Content-Digest", digest)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
}

// handleBlobGet downloads a blob.
// GET /v2/{name}/blobs/{digest}
func (s *server) handleBlobGet(w http.ResponseWriter, r *http.Request) {
	repoName, digest := s.parseBlobPath(r)
	if repoName == "" || digest == "" {
		http.NotFound(w, r)
		return
	}

	auth := s.authenticate(r)
	if !hasAccess(auth, repoName, "pull") {
		s.requireAuth(w, repoName, "pull")
		return
	}

	repoID, _, _, err := getRepoByName(r.Context(), s.db, repoName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	exists, size, err := blobExistsInRepo(r.Context(), s.db, repoID, digest)
	if err != nil || !exists {
		http.NotFound(w, r)
		return
	}

	// Stream blob from GFS
	gfsPath := "blobs/" + digest
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	w.Header().Set("Docker-Content-Digest", digest)
	w.Header().Set("Content-Type", "application/octet-stream")

	if _, err := s.gfs.ReadToWithNamespace(r.Context(), gfsPath, gfsNamespace, w); err != nil {
		slog.Error("blob read failed", "digest", digest, "error", err)
		// Headers already sent, can't return error status
		return
	}
}

// handleBlobDelete deletes a blob.
// DELETE /v2/{name}/blobs/{digest}
func (s *server) handleBlobDelete(w http.ResponseWriter, r *http.Request) {
	repoName, digest := s.parseBlobPath(r)
	if repoName == "" || digest == "" {
		http.NotFound(w, r)
		return
	}

	auth := s.authenticate(r)
	if !hasAccess(auth, repoName, "push") {
		s.requireAuth(w, repoName, "push")
		return
	}

	repoID, _, _, err := getRepoByName(r.Context(), s.db, repoName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Remove blob reference from this repo
	_, err = s.db.ExecContext(r.Context(),
		`DELETE FROM repository_blobs WHERE repository_id = $1 AND digest = $2`,
		repoID, digest)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

// handleUploadStart initiates a blob upload.
// POST /v2/{name}/blobs/uploads/
func (s *server) handleUploadStart(w http.ResponseWriter, r *http.Request) {
	repoName := s.parseUploadRepoName(r)
	if repoName == "" {
		http.NotFound(w, r)
		return
	}

	auth := s.authenticate(r)
	if auth == nil || !hasAccess(auth, repoName, "push") {
		s.requireAuth(w, repoName, "push")
		return
	}

	// Get or create repo
	repoID, err := getOrCreateRepo(r.Context(), s.db, repoName, auth.UserID)
	if err != nil {
		http.Error(w, "failed to create repository", http.StatusInternalServerError)
		return
	}

	// Check for monolithic upload (digest in query = single PUT)
	if digest := r.URL.Query().Get("digest"); digest != "" {
		s.handleMonolithicUpload(w, r, repoName, repoID, digest)
		return
	}

	// Create upload session
	uploadUUID := uuid.New().String()

	// Initialize hasher and serialize state
	hasher := sha256.New()
	hashState, err := hasher.(encoding.BinaryMarshaler).MarshalBinary()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if err := createUploadSession(r.Context(), s.db, uploadUUID, repoID); err != nil {
		http.Error(w, "failed to create upload session", http.StatusInternalServerError)
		return
	}
	if err := updateUploadSession(r.Context(), s.db, uploadUUID, hashState, 0); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Create empty GFS file for the upload
	if _, err := s.gfs.CreateFileWithNamespace(r.Context(), "uploads/"+uploadUUID, gfsNamespace); err != nil {
		http.Error(w, "failed to create upload file", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Location", fmt.Sprintf("/v2/%s/blobs/uploads/%s", repoName, uploadUUID))
	w.Header().Set("Docker-Upload-UUID", uploadUUID)
	w.Header().Set("Range", "0-0")
	w.WriteHeader(http.StatusAccepted)
}

// handleUploadPatch streams chunk data to an in-progress upload.
// PATCH /v2/{name}/blobs/uploads/{uuid}
func (s *server) handleUploadPatch(w http.ResponseWriter, r *http.Request) {
	repoName, uploadUUID := s.parseUploadPath(r)
	if repoName == "" || uploadUUID == "" {
		http.NotFound(w, r)
		return
	}

	auth := s.authenticate(r)
	if !hasAccess(auth, repoName, "push") {
		s.requireAuth(w, repoName, "push")
		return
	}

	// Get upload session
	_, hashState, bytesReceived, err := getUploadSession(r.Context(), s.db, uploadUUID)
	if err != nil {
		http.Error(w, "upload not found", http.StatusNotFound)
		return
	}

	// Restore hasher state
	hasher := sha256.New()
	if hashState != nil {
		if err := hasher.(encoding.BinaryUnmarshaler).UnmarshalBinary(hashState); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	}

	// Stream through hasher and into GFS
	tee := io.TeeReader(r.Body, hasher)
	gfsPath := "uploads/" + uploadUUID
	n, err := s.gfs.AppendFromWithNamespace(r.Context(), gfsPath, gfsNamespace, tee)
	if err != nil {
		http.Error(w, "upload failed", http.StatusInternalServerError)
		return
	}

	// Checkpoint hash state
	newHashState, err := hasher.(encoding.BinaryMarshaler).MarshalBinary()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	newBytesReceived := bytesReceived + n
	if err := updateUploadSession(r.Context(), s.db, uploadUUID, newHashState, newBytesReceived); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Location", fmt.Sprintf("/v2/%s/blobs/uploads/%s", repoName, uploadUUID))
	w.Header().Set("Docker-Upload-UUID", uploadUUID)
	w.Header().Set("Range", fmt.Sprintf("0-%d", newBytesReceived-1))
	w.WriteHeader(http.StatusAccepted)
}

// handleUploadComplete finalizes an upload, verifies digest, and commits the blob.
// PUT /v2/{name}/blobs/uploads/{uuid}?digest=sha256:...
func (s *server) handleUploadComplete(w http.ResponseWriter, r *http.Request) {
	repoName, uploadUUID := s.parseUploadPath(r)
	if repoName == "" || uploadUUID == "" {
		http.NotFound(w, r)
		return
	}

	auth := s.authenticate(r)
	if !hasAccess(auth, repoName, "push") {
		s.requireAuth(w, repoName, "push")
		return
	}

	expectedDigest := r.URL.Query().Get("digest")
	if expectedDigest == "" {
		http.Error(w, "digest parameter required", http.StatusBadRequest)
		return
	}

	// Get upload session
	repoID, hashState, bytesReceived, err := getUploadSession(r.Context(), s.db, uploadUUID)
	if err != nil {
		http.Error(w, "upload not found", http.StatusNotFound)
		return
	}

	// Restore hasher
	hasher := sha256.New()
	if hashState != nil {
		if err := hasher.(encoding.BinaryUnmarshaler).UnmarshalBinary(hashState); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	}

	// Read any remaining body data (PUT can include final chunk)
	if r.Body != nil && r.ContentLength != 0 {
		tee := io.TeeReader(r.Body, hasher)
		gfsPath := "uploads/" + uploadUUID
		n, err := s.gfs.AppendFromWithNamespace(r.Context(), gfsPath, gfsNamespace, tee)
		if err != nil {
			http.Error(w, "upload failed", http.StatusInternalServerError)
			return
		}
		bytesReceived += n
	}

	// Verify digest
	computedDigest := fmt.Sprintf("sha256:%x", hasher.Sum(nil))
	if computedDigest != expectedDigest {
		// Clean up
		s.gfs.DeleteFileWithNamespace(r.Context(), "uploads/"+uploadUUID, gfsNamespace)
		deleteUploadSession(r.Context(), s.db, uploadUUID)
		http.Error(w, fmt.Sprintf("digest mismatch: computed %s, expected %s", computedDigest, expectedDigest), http.StatusBadRequest)
		return
	}

	// Check dedup — does this blob already exist?
	blobPath := "blobs/" + expectedDigest
	existing, _ := s.gfs.GetFileWithNamespace(r.Context(), blobPath, gfsNamespace)
	if existing != nil {
		// Blob already exists — dedup: just record the reference
		s.gfs.DeleteFileWithNamespace(r.Context(), "uploads/"+uploadUUID, gfsNamespace)
	} else {
		// Rename upload to final blob path
		err := s.gfs.RenameFileWithNamespace(r.Context(), "uploads/"+uploadUUID, blobPath, gfsNamespace)
		if err != nil {
			if strings.Contains(err.Error(), "already exists") {
				// Concurrent dedup race — another push completed first
				s.gfs.DeleteFileWithNamespace(r.Context(), "uploads/"+uploadUUID, gfsNamespace)
			} else {
				http.Error(w, "failed to commit blob", http.StatusInternalServerError)
				return
			}
		}
	}

	// Record blob reference
	if err := insertRepoBlob(r.Context(), s.db, repoID, expectedDigest, bytesReceived); err != nil {
		slog.Error("failed to record blob", "digest", expectedDigest, "error", err)
	}

	// Clear GC mark if blob was previously marked for deletion
	clearBlobGCMark(r.Context(), s.db, repoID, expectedDigest)

	// Clean up upload session (best-effort)
	deleteUploadSession(r.Context(), s.db, uploadUUID)

	w.Header().Set("Location", fmt.Sprintf("/v2/%s/blobs/%s", repoName, expectedDigest))
	w.Header().Set("Docker-Content-Digest", expectedDigest)
	w.WriteHeader(http.StatusCreated)
}

// handleMonolithicUpload handles single-request blob upload (POST with digest).
func (s *server) handleMonolithicUpload(w http.ResponseWriter, r *http.Request, repoName string, repoID int, digest string) {
	// Check dedup
	blobPath := "blobs/" + digest
	existing, _ := s.gfs.GetFileWithNamespace(r.Context(), blobPath, gfsNamespace)
	if existing != nil {
		// Already exists
		insertRepoBlob(r.Context(), s.db, repoID, digest, existing.Size)
		w.Header().Set("Location", fmt.Sprintf("/v2/%s/blobs/%s", repoName, digest))
		w.Header().Set("Docker-Content-Digest", digest)
		w.WriteHeader(http.StatusCreated)
		return
	}

	// Hash and write directly
	hasher := sha256.New()
	tee := io.TeeReader(r.Body, hasher)

	tmpPath := "uploads/" + uuid.New().String()
	if _, err := s.gfs.CreateFileWithNamespace(r.Context(), tmpPath, gfsNamespace); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	n, err := s.gfs.AppendFromWithNamespace(r.Context(), tmpPath, gfsNamespace, tee)
	if err != nil {
		s.gfs.DeleteFileWithNamespace(r.Context(), tmpPath, gfsNamespace)
		http.Error(w, "upload failed", http.StatusInternalServerError)
		return
	}

	computed := fmt.Sprintf("sha256:%x", hasher.Sum(nil))
	if computed != digest {
		s.gfs.DeleteFileWithNamespace(r.Context(), tmpPath, gfsNamespace)
		http.Error(w, "digest mismatch", http.StatusBadRequest)
		return
	}

	err = s.gfs.RenameFileWithNamespace(r.Context(), tmpPath, blobPath, gfsNamespace)
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		http.Error(w, "failed to commit blob", http.StatusInternalServerError)
		return
	}
	if err != nil {
		// Concurrent dedup
		s.gfs.DeleteFileWithNamespace(r.Context(), tmpPath, gfsNamespace)
	}

	insertRepoBlob(r.Context(), s.db, repoID, digest, n)

	w.Header().Set("Location", fmt.Sprintf("/v2/%s/blobs/%s", repoName, digest))
	w.Header().Set("Docker-Content-Digest", digest)
	w.WriteHeader(http.StatusCreated)
}

// Path parsing helpers

func (s *server) parseBlobPath(r *http.Request) (repoName, digest string) {
	// Path: /v2/{name}/blobs/{digest}
	// {name} can have slashes (e.g., /v2/eddison/ecloud-auth/blobs/sha256:abc)
	path := strings.TrimPrefix(r.URL.Path, "/v2/")
	idx := strings.Index(path, "/blobs/")
	if idx < 0 {
		return "", ""
	}
	return path[:idx], path[idx+7:]
}

func (s *server) parseUploadRepoName(r *http.Request) string {
	// Path: /v2/{name}/blobs/uploads/
	path := strings.TrimPrefix(r.URL.Path, "/v2/")
	idx := strings.Index(path, "/blobs/uploads")
	if idx < 0 {
		return ""
	}
	return path[:idx]
}

func (s *server) parseUploadPath(r *http.Request) (repoName, uuid string) {
	// Path: /v2/{name}/blobs/uploads/{uuid}
	path := strings.TrimPrefix(r.URL.Path, "/v2/")
	idx := strings.Index(path, "/blobs/uploads/")
	if idx < 0 {
		return "", ""
	}
	repoName = path[:idx]
	uuid = path[idx+15:] // len("/blobs/uploads/") = 15
	return repoName, uuid
}
```

- [ ] **Step 2: Add routes to `main.go`**

Update `registerRoutes` in `main.go`:

```go
func (s *server) registerRoutes(mux *http.ServeMux) {
	// Single catch-all handler for /v2/ — repository names can contain slashes,
	// so Go's built-in mux patterns are insufficient for OCI Distribution paths.
	mux.HandleFunc("/v2/", s.routeV2)
}

// routeV2 dispatches /v2/ requests based on path structure since repository
// names can contain slashes, making Go's built-in mux patterns insufficient.
func (s *server) routeV2(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// /v2/ exact — API version check
	if path == "/v2/" {
		if r.Method == http.MethodGet {
			s.handleAPIVersion(w, r)
			return
		}
	}

	// /v2/_catalog
	if path == "/v2/_catalog" {
		s.handleCatalog(w, r)
		return
	}

	// /v2/{name}/blobs/uploads/{uuid}
	if strings.Contains(path, "/blobs/uploads/") {
		switch r.Method {
		case http.MethodPatch:
			s.handleUploadPatch(w, r)
		case http.MethodPut:
			s.handleUploadComplete(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	// /v2/{name}/blobs/uploads/ (trailing slash, no UUID)
	if strings.HasSuffix(path, "/blobs/uploads/") || strings.HasSuffix(path, "/blobs/uploads") {
		if r.Method == http.MethodPost {
			s.handleUploadStart(w, r)
			return
		}
	}

	// /v2/{name}/blobs/{digest}
	if strings.Contains(path, "/blobs/") {
		switch r.Method {
		case http.MethodHead:
			s.handleBlobHead(w, r)
		case http.MethodGet:
			s.handleBlobGet(w, r)
		case http.MethodDelete:
			s.handleBlobDelete(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	// /v2/{name}/manifests/{reference}
	if strings.Contains(path, "/manifests/") {
		switch r.Method {
		case http.MethodGet, http.MethodHead:
			s.handleManifestGet(w, r)
		case http.MethodPut:
			s.handleManifestPut(w, r)
		case http.MethodDelete:
			s.handleManifestDelete(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	// /v2/{name}/tags/list
	if strings.HasSuffix(path, "/tags/list") {
		s.handleTagsList(w, r)
		return
	}

	http.NotFound(w, r)
}
```

- [ ] **Step 3: Add uuid dependency**

```bash
cd edd-cloud-interface/services/registry && go get github.com/google/uuid
```

- [ ] **Step 4: Verify it compiles** (will have undefined references to manifest handlers — add stubs)

Add temporary stubs to `main.go` if needed:

```go
func (s *server) handleManifestGet(w http.ResponseWriter, r *http.Request)    { http.NotFound(w, r) }
func (s *server) handleManifestPut(w http.ResponseWriter, r *http.Request)    { http.NotFound(w, r) }
func (s *server) handleManifestDelete(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }
func (s *server) handleTagsList(w http.ResponseWriter, r *http.Request)       { http.NotFound(w, r) }
```

```bash
cd edd-cloud-interface/services/registry && go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add edd-cloud-interface/services/registry/
git commit -m "feat(registry): add blob upload/download handlers with digest verification"
```

---

## Chunk 5: Registry Service — Manifests, Tags, Catalog & GC

### Task 7: Manifest and tag handlers

**Files:**
- Create: `edd-cloud-interface/services/registry/manifests.go`

- [ ] **Step 1: Create `manifests.go`**

```go
package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
)

// ociManifest is a minimal OCI manifest for validation.
type ociManifest struct {
	SchemaVersion int    `json:"schemaVersion"`
	MediaType     string `json:"mediaType"`
	Config        struct {
		Digest string `json:"digest"`
		Size   int64  `json:"size"`
	} `json:"config"`
	Layers []struct {
		Digest string `json:"digest"`
		Size   int64  `json:"size"`
	} `json:"layers"`
}

// handleManifestGet retrieves a manifest by tag or digest.
// GET/HEAD /v2/{name}/manifests/{reference}
func (s *server) handleManifestGet(w http.ResponseWriter, r *http.Request) {
	repoName, reference := s.parseManifestPath(r)
	if repoName == "" || reference == "" {
		http.NotFound(w, r)
		return
	}

	auth := s.authenticate(r)
	repoID, _, visibility, err := getRepoByName(r.Context(), s.db, repoName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Public repos allow anonymous pull; private repos require auth
	if visibility != 1 && !hasAccess(auth, repoName, "pull") {
		s.requireAuth(w, repoName, "pull")
		return
	}

	// Resolve reference to digest
	var digest string
	if strings.HasPrefix(reference, "sha256:") {
		digest = reference
	} else {
		// It's a tag
		d, err := getTagDigest(r.Context(), s.db, repoID, reference)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		digest = d
	}

	// Get manifest metadata from DB
	_, mediaType, size, err := getManifest(r.Context(), s.db, repoID, digest)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("Docker-Content-Digest", digest)
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))

	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Read manifest from GFS
	gfsPath := fmt.Sprintf("manifests/%s/%s", repoName, digest)
	if _, err := s.gfs.ReadToWithNamespace(r.Context(), gfsPath, gfsNamespace, w); err != nil {
		slog.Error("manifest read failed", "repo", repoName, "digest", digest, "error", err)
	}
}

// handleManifestPut pushes a manifest (by tag or digest).
// PUT /v2/{name}/manifests/{reference}
func (s *server) handleManifestPut(w http.ResponseWriter, r *http.Request) {
	repoName, reference := s.parseManifestPath(r)
	if repoName == "" || reference == "" {
		http.NotFound(w, r)
		return
	}

	auth := s.authenticate(r)
	if auth == nil || !hasAccess(auth, repoName, "push") {
		s.requireAuth(w, repoName, "push")
		return
	}

	// Read manifest body
	body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20)) // 10MB limit for manifests
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	// Compute digest
	digest := fmt.Sprintf("sha256:%x", sha256.Sum256(body))

	// Validate manifest
	mediaType := r.Header.Get("Content-Type")
	if mediaType == "" {
		mediaType = "application/vnd.oci.image.manifest.v1+json"
	}

	var manifest ociManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		http.Error(w, "invalid manifest JSON", http.StatusBadRequest)
		return
	}
	if manifest.SchemaVersion != 2 {
		http.Error(w, "unsupported schema version", http.StatusBadRequest)
		return
	}

	// Get or create repo
	repoID, err := getOrCreateRepo(r.Context(), s.db, repoName, auth.UserID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Verify referenced blobs exist
	var blobDigests []string
	if manifest.Config.Digest != "" {
		exists, _, err := blobExistsInRepo(r.Context(), s.db, repoID, manifest.Config.Digest)
		if err != nil || !exists {
			http.Error(w, fmt.Sprintf("config blob %s not found", manifest.Config.Digest), http.StatusBadRequest)
			return
		}
		blobDigests = append(blobDigests, manifest.Config.Digest)
	}
	for _, layer := range manifest.Layers {
		exists, _, err := blobExistsInRepo(r.Context(), s.db, repoID, layer.Digest)
		if err != nil || !exists {
			http.Error(w, fmt.Sprintf("layer blob %s not found", layer.Digest), http.StatusBadRequest)
			return
		}
		blobDigests = append(blobDigests, layer.Digest)
	}

	// Store manifest in GFS
	gfsPath := fmt.Sprintf("manifests/%s/%s", repoName, digest)
	existing, _ := s.gfs.GetFileWithNamespace(r.Context(), gfsPath, gfsNamespace)
	if existing == nil {
		if _, err := s.gfs.CreateFileWithNamespace(r.Context(), gfsPath, gfsNamespace); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if _, err := s.gfs.WriteWithNamespace(r.Context(), gfsPath, gfsNamespace, body); err != nil {
			http.Error(w, "failed to store manifest", http.StatusInternalServerError)
			return
		}
	}

	// Upsert manifest in DB
	manifestID, err := upsertManifest(r.Context(), s.db, repoID, digest, mediaType, int64(len(body)))
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Record manifest-blob references
	if err := insertManifestBlobs(r.Context(), s.db, manifestID, blobDigests); err != nil {
		slog.Error("failed to record manifest blobs", "error", err)
	}

	// Tag handling
	if !strings.HasPrefix(reference, "sha256:") {
		// It's a tag — upsert and check for old manifest cleanup
		oldDigest, err := upsertTag(r.Context(), s.db, repoID, reference, digest)
		if err != nil {
			http.Error(w, "failed to create tag", http.StatusInternalServerError)
			return
		}

		// Eager manifest cleanup: if old digest has no other tags, delete it
		if oldDigest != "" {
			s.cleanupManifest(r.Context(), repoID, repoName, oldDigest, reference)
		}
	}

	w.Header().Set("Location", fmt.Sprintf("/v2/%s/manifests/%s", repoName, digest))
	w.Header().Set("Docker-Content-Digest", digest)
	w.WriteHeader(http.StatusCreated)
}

// handleManifestDelete deletes a manifest.
// DELETE /v2/{name}/manifests/{reference}
func (s *server) handleManifestDelete(w http.ResponseWriter, r *http.Request) {
	repoName, reference := s.parseManifestPath(r)
	if repoName == "" || reference == "" {
		http.NotFound(w, r)
		return
	}

	auth := s.authenticate(r)
	if !hasAccess(auth, repoName, "push") {
		s.requireAuth(w, repoName, "push")
		return
	}

	repoID, _, _, err := getRepoByName(r.Context(), s.db, repoName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Resolve to digest
	var digest string
	if strings.HasPrefix(reference, "sha256:") {
		digest = reference
	} else {
		d, err := getTagDigest(r.Context(), s.db, repoID, reference)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		digest = d
		// Delete the tag
		s.db.ExecContext(r.Context(),
			`DELETE FROM tags WHERE repository_id = $1 AND name = $2`, repoID, reference)
	}

	s.cleanupManifest(r.Context(), repoID, repoName, digest, "")

	w.WriteHeader(http.StatusAccepted)
}

// cleanupManifest deletes a manifest if no tags reference it.
func (s *server) cleanupManifest(ctx context.Context, repoID int, repoName, digest, excludeTag string) {
	hasOther, err := manifestHasOtherTags(ctx, s.db, repoID, digest, excludeTag)
	if err != nil || hasOther {
		return
	}

	// No tags reference this manifest — delete it
	gfsPath := fmt.Sprintf("manifests/%s/%s", repoName, digest)
	if err := s.gfs.DeleteFileWithNamespace(ctx, gfsPath, gfsNamespace); err != nil {
		slog.Warn("failed to delete manifest from GFS", "path", gfsPath, "error", err)
	}

	s.db.ExecContext(ctx,
		`DELETE FROM manifests WHERE repository_id = $1 AND digest = $2`, repoID, digest)
}

// handleTagsList lists tags for a repository.
// GET /v2/{name}/tags/list
func (s *server) handleTagsList(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v2/")
	repoName := strings.TrimSuffix(path, "/tags/list")
	if repoName == "" {
		http.NotFound(w, r)
		return
	}

	auth := s.authenticate(r)
	repoID, _, visibility, err := getRepoByName(r.Context(), s.db, repoName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if visibility != 1 && !hasAccess(auth, repoName, "pull") {
		s.requireAuth(w, repoName, "pull")
		return
	}

	tags, err := listTags(r.Context(), s.db, repoID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"name": repoName,
		"tags": tags,
	})
}

// handleCatalog lists repositories with pagination.
// GET /v2/_catalog
func (s *server) handleCatalog(w http.ResponseWriter, r *http.Request) {
	auth := s.authenticate(r)

	limit := 100
	offset := 0
	if n, err := strconv.Atoi(r.URL.Query().Get("n")); err == nil && n > 0 {
		limit = n
	}
	last := r.URL.Query().Get("last")

	// Use cursor-based pagination (more efficient and correct than offset)
	var repos []string
	var err error
	if auth != nil && auth.UserID != "" {
		query := `SELECT name FROM repositories WHERE (owner_id = $1 OR visibility = 1)`
		args := []interface{}{auth.UserID}
		if last != "" {
			query += ` AND name > $2 ORDER BY name LIMIT $3`
			args = append(args, last, limit+1)
		} else {
			query += ` ORDER BY name LIMIT $2`
			args = append(args, limit+1)
		}
		rows, qerr := s.db.QueryContext(r.Context(), query, args...)
		if qerr != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			rows.Scan(&name)
			repos = append(repos, name)
		}
		err = rows.Err()
	} else {
		// Anonymous: public only
		query := `SELECT name FROM repositories WHERE visibility = 1`
		args := []interface{}{}
		if last != "" {
			query += ` AND name > $1 ORDER BY name LIMIT $2`
			args = append(args, last, limit+1)
		} else {
			query += ` ORDER BY name LIMIT $1`
			args = append(args, limit+1)
		}
		rows, qerr := s.db.QueryContext(r.Context(), query, args...)
		if qerr != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			rows.Scan(&name)
			repos = append(repos, name)
		}
		err = rows.Err()
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Pagination link header
	if len(repos) > limit {
		repos = repos[:limit]
		last := repos[len(repos)-1]
		w.Header().Set("Link", fmt.Sprintf(`</v2/_catalog?n=%d&last=%s>; rel="next"`, limit, last))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"repositories": repos,
	})
}

func (s *server) parseManifestPath(r *http.Request) (repoName, reference string) {
	path := strings.TrimPrefix(r.URL.Path, "/v2/")
	idx := strings.Index(path, "/manifests/")
	if idx < 0 {
		return "", ""
	}
	return path[:idx], path[idx+11:]
}
```

- [ ] **Step 2: Remove ALL stub handlers from `main.go`**

Delete the placeholder `handleManifestGet`, `handleManifestPut`, `handleManifestDelete`, `handleTagsList`, AND `handleCatalog` stubs. All of these are now implemented in `manifests.go`.

- [ ] **Step 3: Verify it compiles**

```bash
cd edd-cloud-interface/services/registry && go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add edd-cloud-interface/services/registry/manifests.go edd-cloud-interface/services/registry/main.go
git commit -m "feat(registry): add manifest, tag, and catalog handlers"
```

### Task 8: Garbage collection

**Files:**
- Create: `edd-cloud-interface/services/registry/gc.go`

- [ ] **Step 1: Create `gc.go`**

```go
package main

import (
	"context"
	"log/slog"
	"time"
)

// startGC launches the background garbage collection goroutine.
func (s *server) startGC(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		slog.Info("GC started", "interval", interval)
		for {
			select {
			case <-ctx.Done():
				slog.Info("GC stopped")
				return
			case <-ticker.C:
				s.runGC(ctx, interval)
			}
		}
	}()
}

func (s *server) runGC(ctx context.Context, interval time.Duration) {
	start := time.Now()
	slog.Info("GC cycle starting")

	// Phase 1: Sweep — delete blobs marked before the previous GC cycle
	// This ensures a blob must be orphaned for at least one full interval
	sweepBefore := start.Add(-interval)
	swept, err := sweepMarkedBlobs(ctx, s.db, sweepBefore)
	if err != nil {
		slog.Error("GC sweep failed", "error", err)
	} else {
		for _, blob := range swept {
			gfsPath := "blobs/" + blob.Digest
			if err := s.gfs.DeleteFileWithNamespace(ctx, gfsPath, gfsNamespace); err != nil {
				slog.Warn("GC: failed to delete blob from GFS", "digest", blob.Digest, "error", err)
			} else {
				slog.Info("GC: deleted orphaned blob", "digest", blob.Digest)
			}
		}
		if len(swept) > 0 {
			slog.Info("GC sweep complete", "deleted", len(swept))
		}
	}

	// Phase 2: Mark — mark currently orphaned blobs for next cycle
	marked, err := markOrphanedBlobs(ctx, s.db)
	if err != nil {
		slog.Error("GC mark failed", "error", err)
	} else if marked > 0 {
		slog.Info("GC marked orphaned blobs", "count", marked)
	}

	// Phase 3: Clean up stale upload sessions
	deleted, err := deleteStaleUploadSessions(ctx, s.db, 24*time.Hour)
	if err != nil {
		slog.Error("GC: failed to clean upload sessions", "error", err)
	} else if deleted > 0 {
		slog.Info("GC: cleaned stale uploads", "count", deleted)
		// Also delete corresponding GFS files
		// (upload files without sessions are harmless but waste space)
	}

	slog.Info("GC cycle complete", "duration", time.Since(start))
}
```

- [ ] **Step 2: Start GC in `main.go`**

Add after server creation, before `ListenAndServe`:

```go
// Start garbage collection
gcCtx, gcCancel := context.WithCancel(context.Background())
defer gcCancel()
srv.startGC(gcCtx, 24*time.Hour)
```

- [ ] **Step 3: Verify it compiles**

```bash
cd edd-cloud-interface/services/registry && go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add edd-cloud-interface/services/registry/gc.go edd-cloud-interface/services/registry/main.go
git commit -m "feat(registry): add two-phase mark-sweep garbage collection"
```

---

## Chunk 6: Registry Service — NATS Events, Proto, & Kubernetes Manifests

### Task 9: Proto definitions and NATS event publishing

**Files:**
- Create: `proto/registry/events.proto`
- Modify: `edd-cloud-interface/services/registry/main.go` (add NATS publisher)

- [ ] **Step 1: Create `proto/registry/events.proto`**

```protobuf
syntax = "proto3";

package registry;

option go_package = "eddisonso.com/edd-cloud/proto/registry";

import "common/types.proto";

message ImagePushed {
  common.EventMetadata metadata = 1;
  string repository = 2;
  string tag = 3;
  string digest = 4;
  string actor = 5;  // user_id or service_account_id
}

message ImageDeleted {
  common.EventMetadata metadata = 1;
  string repository = 2;
  string tag = 3;
  string digest = 4;
  string actor = 5;
}

message RepositoryCreated {
  common.EventMetadata metadata = 1;
  string repository = 2;
  string owner_id = 3;
}

message RepositoryDeleted {
  common.EventMetadata metadata = 1;
  string repository = 2;
}
```

- [ ] **Step 2: Add NATS publisher to server struct and init**

Add to server struct in `main.go`:

```go
import notifypub "eddisonso.com/notification-service/pkg/publisher"

type server struct {
	gfs       *gfs.Client
	db        *sql.DB
	jwtSecret []byte
	notifier  *notifypub.Publisher
}
```

Add NATS init after GFS client setup:

```go
natsURL := os.Getenv("NATS_URL")
if natsURL != "" {
	np, err := notifypub.New(natsURL, "edd-registry")
	if err != nil {
		slog.Warn("failed to create notification publisher", "error", err)
	} else {
		srv.notifier = np
		defer np.Close()
	}
}
```

- [ ] **Step 3: Add event emission to all relevant handlers**

**In `manifests.go` — `handleManifestPut`** (after successful push, before writing response):

```go
// Emit NATS event
if s.notifier != nil && auth != nil {
	tag := ""
	if !strings.HasPrefix(reference, "sha256:") {
		tag = reference
	}
	s.notifier.Notify(r.Context(), auth.UserID,
		"Image Pushed",
		fmt.Sprintf("Image %s:%s pushed to registry", repoName, reference),
		fmt.Sprintf("https://registry.cloud.eddisonso.com/v2/%s/manifests/%s", repoName, reference),
		"registry",
		fmt.Sprintf("registry.%s", repoName),
	)
	_ = tag // Future: structured event with proto
}
```

**In `manifests.go` — `handleManifestDelete`** (after successful delete):

```go
if s.notifier != nil && auth != nil {
	s.notifier.Notify(r.Context(), auth.UserID,
		"Image Deleted",
		fmt.Sprintf("Image %s:%s deleted from registry", repoName, reference),
		"", "registry",
		fmt.Sprintf("registry.%s", repoName),
	)
}
```

**In `db.go` — `getOrCreateRepo`**: Emit `registry.repository.created` when a new repo is actually created (not on conflict update). Modify to return a boolean indicating creation:

```go
func getOrCreateRepo(ctx context.Context, db *sql.DB, name, ownerID string) (int, bool, error) {
	// Try insert first
	var id int
	err := db.QueryRowContext(ctx,
		`INSERT INTO repositories (name, owner_id) VALUES ($1, $2)
		 ON CONFLICT (name) DO NOTHING RETURNING id`, name, ownerID).Scan(&id)
	if err == nil {
		return id, true, nil // created
	}
	// Already exists — fetch
	err = db.QueryRowContext(ctx,
		`SELECT id FROM repositories WHERE name = $1`, name).Scan(&id)
	return id, false, err
}
```

Then in `handleUploadStart` and `handleManifestPut`, check the `created` flag and emit `registry.repository.created` notification.

- [ ] **Step 4: Commit**

```bash
git add proto/registry/events.proto edd-cloud-interface/services/registry/
git commit -m "feat(registry): add NATS event publishing and proto definitions"
```

### Task 10: Kubernetes manifests and gateway route

**Files:**
- Create: `manifests/edd-registry/edd-registry.yaml`
- Modify: `edd-gateway/manifests/gateway-routes.yaml`

- [ ] **Step 1: Create Kubernetes manifest**

Create `manifests/edd-registry/edd-registry.yaml` following the SFS pattern (`manifests/sfs/simple-file-share.yaml`):

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: edd-registry
  namespace: default
spec:
  replicas: 2
  selector:
    matchLabels:
      app: edd-registry
  template:
    metadata:
      labels:
        app: edd-registry
    spec:
      topologySpreadConstraints:
        - maxSkew: 1
          topologyKey: kubernetes.io/hostname
          whenUnsatisfiable: DoNotSchedule
          labelSelector:
            matchLabels:
              app: edd-registry
      nodeSelector:
        backend: "true"
      containers:
        - name: registry
          image: eddisonso/ecloud-registry:latest
          args:
            - -addr
            - 0.0.0.0:8080
            - -master
            - gfs-master:9000
          ports:
            - containerPort: 8080
          env:
            - name: DATABASE_URL
              valueFrom:
                secretKeyRef:
                  name: edd-cloud-db
                  key: url
            - name: JWT_SECRET
              valueFrom:
                secretKeyRef:
                  name: edd-cloud-auth-secret
                  key: jwt-secret
            - name: NATS_URL
              value: "nats://nats:4222"
          resources:
            requests:
              cpu: 100m
              memory: 128Mi
            limits:
              cpu: "1"
              memory: 512Mi
          livenessProbe:
            httpGet:
              path: /v2/
              port: 8080
            initialDelaySeconds: 5
            periodSeconds: 10
          readinessProbe:
            httpGet:
              path: /v2/
              port: 8080
            initialDelaySeconds: 3
            periodSeconds: 5
---
apiVersion: v1
kind: Service
metadata:
  name: edd-registry
  namespace: default
spec:
  selector:
    app: edd-registry
  ports:
    - port: 80
      targetPort: 8080
```

- [ ] **Step 2: Add gateway route**

Add to `edd-gateway/manifests/gateway-routes.yaml`:

```yaml
registry.cloud.eddisonso.com:
  path: /
  target: edd-registry:80
```

- [ ] **Step 3: Commit**

```bash
git add manifests/edd-registry/ edd-gateway/manifests/gateway-routes.yaml
git commit -m "feat(registry): add Kubernetes deployment and gateway route"
```

---

## Chunk 7: Compute Service Integration

### Task 11: Backend — accept image field and create imagePullSecret

**Files:**
- Modify: `edd-cloud-interface/services/compute/internal/api/containers.go` (line ~23 for request struct, line ~184 for image handling)
- Modify: `edd-cloud-interface/services/compute/internal/k8s/client.go` (line ~237 for CreatePod, add imagePullSecret creation)

- [ ] **Step 1: Add `image` field to `containerRequest`**

In `containers.go`, add the `Image` field to the existing `containerRequest` struct. Do NOT change any other fields — keep existing types and field names as-is:

```go
// Add this single field to the existing containerRequest struct:
Image string `json:"image,omitempty"` // Optional — defaults to base image
```

- [ ] **Step 2: Use the image field in container creation**

In the create handler, after validation, resolve the image:

```go
image := defaultImage
if req.Image != "" {
	// Validate it's either the default or a registry.cloud.eddisonso.com image
	if req.Image != defaultImage && !strings.HasPrefix(req.Image, "registry.cloud.eddisonso.com/") {
		http.Error(w, "invalid image: must be from registry.cloud.eddisonso.com", http.StatusBadRequest)
		return
	}
	image = req.Image
}

container := &db.Container{
	...
	Image: image,
	...
}
```

- [ ] **Step 3: Add `CreateImagePullSecret` to K8s client**

In `client.go`, add:

```go
// CreateImagePullSecret creates a docker-registry secret for pulling from the private registry.
func (c *Client) CreateImagePullSecret(ctx context.Context, namespace, registryURL, username, token string) error {
	dockerConfig := fmt.Sprintf(`{"auths":{"%s":{"username":"%s","password":"%s","auth":"%s"}}}`,
		registryURL, username, token,
		base64.StdEncoding.EncodeToString([]byte(username+":"+token)))

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "registry-pull-secret",
			Namespace: namespace,
		},
		Type: corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			corev1.DockerConfigJsonKey: []byte(dockerConfig),
		},
	}
	_, err := c.clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
	return err
}
```

- [ ] **Step 4: Add `ImagePullSecrets` to pod spec**

In `CreatePod`, add parameter for whether to use pull secret, and add to pod spec:

```go
// Add to pod spec if using registry image
if strings.HasPrefix(image, "registry.cloud.eddisonso.com/") {
	pod.Spec.ImagePullSecrets = []corev1.LocalObjectReference{
		{Name: "registry-pull-secret"},
	}
}
```

- [ ] **Step 5: Create pull secret during provisioning**

In `provisionContainer` (containers.go), after namespace creation but before pod creation, if using a registry image:

```go
if strings.HasPrefix(container.Image, "registry.cloud.eddisonso.com/") {
	// Create imagePullSecret for the registry
	// Use a service-to-service token to pull on behalf of the user
	registryToken := os.Getenv("REGISTRY_PULL_TOKEN")
	if err := h.k8s.CreateImagePullSecret(ctx, container.Namespace,
		"registry.cloud.eddisonso.com", "service", registryToken); err != nil {
		slog.Error("failed to create pull secret", "error", err)
		// Non-fatal — pod will fail to pull but won't crash the provisioning
	}
}
```

- [ ] **Step 6: Verify it compiles**

```bash
cd edd-cloud-interface/services/compute && go build ./...
```

- [ ] **Step 7: Commit**

```bash
git add edd-cloud-interface/services/compute/
git commit -m "feat(compute): accept custom image and create imagePullSecret for registry images"
```

### Task 12: Add registry images endpoint to compute API

**Files:**
- Modify: `edd-cloud-interface/services/compute/internal/api/handler.go` (add route)
- Modify: `edd-cloud-interface/services/compute/internal/api/containers.go` (add handler)

- [ ] **Step 1: Add available images endpoint**

In `containers.go`, add:

```go
// handleListImages returns available images for container creation.
func (h *Handler) handleListImages(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.authenticate(w, r)
	if !ok {
		return
	}

	images := []map[string]string{
		{"name": "Debian (Base)", "image": defaultImage, "source": "builtin"},
	}

	// Query registry for user's accessible repos
	registryURL := os.Getenv("REGISTRY_URL")
	if registryURL == "" {
		registryURL = "http://edd-registry:80"
	}

	// Fetch catalog from registry using internal service call
	// The internal registry URL is cluster-internal, so we can use anonymous access
	// which returns public repos. For user-specific repos, the frontend will
	// need to query the registry directly with the user's token in a future iteration.
	resp, err := http.Get(registryURL + "/v2/_catalog")
	if err == nil && resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		var catalog struct {
			Repositories []string `json:"repositories"`
		}
		if json.NewDecoder(resp.Body).Decode(&catalog) == nil {
			for _, repo := range catalog.Repositories {
				// Fetch tags for each repo
				tagResp, err := http.Get(fmt.Sprintf("%s/v2/%s/tags/list", registryURL, repo))
				if err != nil || tagResp.StatusCode != http.StatusOK {
					continue
				}
				var tagList struct {
					Tags []string `json:"tags"`
				}
				json.NewDecoder(tagResp.Body).Decode(&tagList)
				tagResp.Body.Close()

				for _, tag := range tagList.Tags {
					images = append(images, map[string]string{
						"name":   fmt.Sprintf("%s:%s", repo, tag),
						"image":  fmt.Sprintf("registry.cloud.eddisonso.com/%s:%s", repo, tag),
						"source": "registry",
					})
				}
			}
		}
	}

	_ = claims // Used for access filtering in future
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(images)
}
```

- [ ] **Step 2: Register the route**

In `handler.go`, add:

```go
mux.HandleFunc("GET /compute/images", h.requireAuth(h.handleListImages))
```

- [ ] **Step 3: Commit**

```bash
git add edd-cloud-interface/services/compute/
git commit -m "feat(compute): add GET /compute/images endpoint for registry image listing"
```

### Task 13: Frontend — image picker in CreateContainerForm

**Files:**
- Modify: `edd-cloud-interface/frontend/src/types/domain.ts`
- Modify: `edd-cloud-interface/frontend/src/hooks/useContainers.ts`
- Modify: `edd-cloud-interface/frontend/src/components/compute/CreateContainerForm.tsx`

- [ ] **Step 1: Add `image` field to `CreateContainerData`**

In `domain.ts`:

```typescript
export interface CreateContainerData {
  name: string;
  memory_mb: number;
  storage_gb: number;
  instance_type: string;
  ssh_key_ids: string[];
  enable_ssh: boolean;
  ingress_rules: IngressRule[];
  mount_paths: string[];
  image?: string; // Optional — defaults to base image on backend
}
```

- [ ] **Step 2: Add `fetchImages` to `useContainers` hook**

In `useContainers.ts`, add:

```typescript
interface AvailableImage {
  name: string;
  image: string;
  source: "builtin" | "registry";
}

const [images, setImages] = useState<AvailableImage[]>([]);

const loadImages = useCallback(async () => {
  try {
    const response = await fetch(`${buildComputeBase()}/compute/images`, {
      headers: getAuthHeaders(),
    });
    if (response.ok) {
      setImages(await response.json());
    }
  } catch {
    // Non-fatal — form still works with default image
  }
}, []);

useEffect(() => { loadImages(); }, [loadImages]);
```

Return `images` from the hook.

- [ ] **Step 3: Add image selector to `CreateContainerForm`**

In `CreateContainerForm.tsx`, add after the instance type select:

```tsx
const [selectedImage, setSelectedImage] = useState("");

// In the form JSX, after instance type:
<div>
  <label htmlFor="c-image" className="block text-sm font-medium mb-1">Image</label>
  <Select
    id="c-image"
    value={selectedImage}
    onChange={(e) => setSelectedImage(e.target.value)}
    className="w-full"
  >
    <option value="">Default (Debian Base)</option>
    {images.filter(i => i.source === "registry").map(img => (
      <option key={img.image} value={img.image}>{img.name}</option>
    ))}
  </Select>
</div>
```

In the `onCreate` call, add the image:

```typescript
onCreate({
  ...existingFields,
  image: selectedImage || undefined,
});
```

- [ ] **Step 4: Verify frontend compiles**

```bash
cd edd-cloud-interface/frontend && npm run build
```

- [ ] **Step 5: Commit**

```bash
git add edd-cloud-interface/frontend/
git commit -m "feat(compute): add image picker to container creation form"
```

---

## Chunk 8: CI/CD Pipeline & Documentation

### Task 14: CI/CD workflow for registry service

**Files:**
- Modify: `.github/workflows/deploy.yml` (or equivalent — add registry build job)

- [ ] **Step 1: Add registry to the CI/CD pipeline**

Follow the existing pattern for SFS/auth/compute services. Add a job that:

1. Detects changes in `edd-cloud-interface/services/registry/`
2. Builds Docker image: `eddisonso/ecloud-registry:$TAG`
3. Pushes to Docker Hub
4. Updates manifest image tag in `manifests/edd-registry/edd-registry.yaml`

The exact implementation depends on the existing workflow structure — follow the pattern of existing service build jobs.

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/
git commit -m "ci(registry): add registry service to CI/CD pipeline"
```

### Task 15: Documentation

**Files:**
- Create: `edd-cloud-docs/docs/services/registry.md`
- Create: `edd-cloud-docs/docs/api/registry.md`

- [ ] **Step 1: Create service documentation**

`edd-cloud-docs/docs/services/registry.md` — Overview of the registry service, architecture, GFS integration, auth flow.

- [ ] **Step 2: Create API documentation**

`edd-cloud-docs/docs/api/registry.md` — All OCI endpoints, authentication, examples with `docker` CLI.

- [ ] **Step 3: Commit**

```bash
git add edd-cloud-docs/
git commit -m "docs(registry): add registry service and API documentation"
```
