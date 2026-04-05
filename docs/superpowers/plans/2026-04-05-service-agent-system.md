# Service Agent System Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create 13 Claude Code custom agents (1 orchestrator + 12 service agents) that enable fast, context-aware development across the edd-cloud monorepo.

**Architecture:** Smart orchestrator dispatches to service-specific agents via Claude Code's Agent tool. Agents have scoped write access to their service directory and read-only access to proto/manifests. Communication is orchestrator-mediated only.

**Tech Stack:** Claude Code custom agents (`.md` files in `.claude/agents/`), frontmatter configuration (name, description, model, color)

**Spec:** `docs/superpowers/specs/2026-04-05-service-agent-system-design.md`

---

## File Structure

All files are created in `/home/eddison/cloud/.claude/agents/`:

| File | Purpose |
|------|---------|
| `cloud-orchestrator.md` | Intent analysis, routing, sequencing, agent dispatch |
| `auth-dev.md` | edd-cloud-auth development (WebAuthn, JWT, sessions) |
| `services-dev.md` | edd-cloud-interface/services + pkg (Go backends) |
| `frontend-dev.md` | edd-cloud-interface/frontend (React/TypeScript) |
| `gateway-dev.md` | edd-gateway (multi-protocol proxy, routing) |
| `gfs-dev.md` | go-gfs (distributed file system) |
| `log-dev.md` | log-service (gRPC logging, NATS) |
| `monitor-dev.md` | cluster-monitor (K8s health, metrics) |
| `manager-dev.md` | cluster-manager (node cron, terminal) |
| `alerting-dev.md` | alerting-service (NATS consumer, Discord) |
| `notification-dev.md` | notification-service (WebSocket, NATS) |
| `infra-dev.md` | manifests/ + proto/ (K8s config, protobuf) |
| `docs-dev.md` | edd-cloud-docs (Docusaurus) |

---

### Task 1: Create the Cloud Orchestrator Agent

**Files:**
- Create: `.claude/agents/cloud-orchestrator.md`

- [ ] **Step 1: Write the orchestrator agent file**

Write `.claude/agents/cloud-orchestrator.md` with:

**Frontmatter:**
- name: `cloud-orchestrator`
- description: Routing description with examples covering single-service tasks, multi-service tasks, and ambiguous requests
- model: `sonnet`
- color: `purple`

**Body content must include:**

1. **Role:** You are the central orchestrator for the edd-cloud monorepo. You analyze user intent, route to the correct service agent(s), and synthesize results. You NEVER read or write code directly.

2. **Service Registry** — table mapping each agent name to its write scope, model, and when to route there:
   - `auth-dev` — `edd-cloud-auth/` — authentication, login, sessions, JWT, WebAuthn, API tokens, SSH keys
   - `services-dev` — `edd-cloud-interface/services/`, `edd-cloud-interface/pkg/` — compute, registry, sfs, health backends
   - `frontend-dev` — `edd-cloud-interface/frontend/` — React UI, dashboard, components, pages
   - `gateway-dev` — `edd-gateway/` — routing, proxy, TLS, SSH tunneling, ingress
   - `gfs-dev` — `go-gfs/` — distributed file system, chunks, replication, master/chunkserver
   - `log-dev` — `log-service/` — centralized logging, log streaming, gRPC log server
   - `monitor-dev` — `cluster-monitor/` — node health, pod metrics, cluster status
   - `manager-dev` — `cluster-manager/` — node cron jobs, terminal access, node info
   - `alerting-dev` — `alerting-service/` — Discord alerts, threshold monitoring, NATS consumers
   - `notification-dev` — `notification-service/` — user notifications, WebSocket delivery
   - `infra-dev` — `manifests/`, `proto/` — K8s manifests, protobuf definitions, cross-service config
   - `docs-dev` — `edd-cloud-docs/` — documentation site, API docs, guides

3. **Routing Rules** (priority order):
   - User explicitly names a service → route there
   - File paths mentioned → map to service directory → route there
   - Ambiguous → ask the user
   - Multi-service → dispatch in dependency order

4. **Sequencing Rules** for multi-service tasks:
   - `proto/` changes first (via `infra-dev`)
   - Backend services in parallel (multiple Agent tool calls in one response)
   - `frontend-dev` after backends it depends on
   - `manifests/` after services (via `infra-dev`)
   - `docs-dev` last if API behavior changed

5. **Dispatch mechanism:** Use Claude Code's Agent tool with `subagent_type` parameter matching the agent name. For parallel dispatch, issue multiple Agent tool calls in a single response.

6. **Agent output handling:** After each agent completes, review its report for:
   - Status (success/partial/failed)
   - Cross-service flags (dispatch follow-up agents if needed)
   - If failed: present failure to user, do not dispatch downstream agents
   - If partial: present what worked, ask user whether to continue

7. **Post-development flow:**
   - After all development agents complete, invoke `commit-organizer` agent
   - After push, invoke `actions-monitor` agent in background

8. **What you do NOT do:** read/write code, make architectural decisions, run tests/builds. You dispatch and synthesize.

- [ ] **Step 2: Verify the agent loads**

Run: `ls -la .claude/agents/cloud-orchestrator.md`
Expected: File exists with correct permissions

- [ ] **Step 3: Commit**

Use the `commit-organizer` agent via the Agent tool to commit `.claude/agents/cloud-orchestrator.md`.

---

### Task 2: Create auth-dev Agent (Opus)

**Files:**
- Create: `.claude/agents/auth-dev.md`

- [ ] **Step 1: Write the auth-dev agent file**

**Frontmatter:**
- name: `auth-dev`
- description: Examples covering auth bugs, new endpoints, session management, SSH key operations
- model: `opus`
- color: `blue`

**Body content must include:**

1. **Role:** Expert Go developer specializing in authentication, security, and identity management for the edd-cloud-auth service.

2. **Service Overview:**
   - Entry point: `cmd/auth/main.go`
   - HTTP server on `:8080`
   - PostgreSQL database (via `DATABASE_URL`)
   - JWT-based sessions with configurable TTL

3. **Directory Structure:**
   - `cmd/auth/` — main entry point
   - `internal/db/` — database layer (users, sessions, credentials, API tokens, service accounts)
   - `internal/api/` — HTTP handlers (auth, admin, tokens, webauthn, service accounts, registry tokens, rate limiting)
   - `internal/events/` — NATS event publishing

4. **Key Dependencies:**
   - PostgreSQL (lib/pq), WebAuthn (go-webauthn), JWT, bcrypt
   - go-gfs SDK for logging
   - NATS for events

5. **NATS Events Published:**
   - `auth.user.*` (created, deleted, updated)
   - `auth.session.*` (created, invalidated)
   - `auth.identity.*` (permissions updated/deleted)

6. **Config/Environment:**
   - `DATABASE_URL`, `JWT_SECRET`, `ADMIN_USERNAME`, `DEFAULT_USERNAME`, `DEFAULT_PASSWORD`, `SERVICE_API_KEY`
   - `NATS_URL` (optional), `WEBAUTHN_RP_ID`, `WEBAUTHN_RP_ORIGINS`
   - `--log-service` flag

7. **Cross-Service Awareness:**
   - Gateway consumes auth tokens for request validation
   - Compute/SFS/Registry subscribe to `auth.identity.*` for permission sync
   - Frontend calls auth API endpoints

8. **Write Scope:** `edd-cloud-auth/` only. Flag changes needed in other services.

9. **Build/Test:** `go build ./cmd/auth`, `go test ./...`

10. **Read-Only Access:** `proto/auth/`, `manifests/`

11. **Output Contract:** Always return status, files changed, tests run, cross-service flags, summary.

12. **Error Handling:** If the task is outside your scope, report `status: failed` with a routing suggestion. If tests fail, attempt to fix; if you cannot, report `status: partial` with failure details.

- [ ] **Step 2: Commit**

Use the `commit-organizer` agent via the Agent tool to commit `.claude/agents/auth-dev.md`.

---

### Task 3: Create services-dev Agent (Opus)

**Files:**
- Create: `.claude/agents/services-dev.md`

- [ ] **Step 1: Write the services-dev agent file**

**Frontmatter:**
- name: `services-dev`
- description: Examples covering compute, registry, sfs, health backends under edd-cloud-interface
- model: `opus`
- color: `cyan`

**Body content must include:**

1. **Role:** Expert Go developer for the edd-cloud-interface backend services — compute, registry, sfs, and health. (Note: the `services/logging/` sub-service is within your write scope but is a thin proxy to the main log-service.)

2. **Sub-Services:**

   **Compute** (`services/compute/`):
   - Entry: `main.go`, HTTP on `:8080`
   - `internal/api/` — containers, SSH, HTTPS, ingress, logs, terminal, WebSocket
   - `internal/db/` — database models
   - `internal/k8s/` — Kubernetes client (pod/container management)
   - `internal/events/` — identity sync, container lifecycle
   - NATS publishes: `compute.container.*`, `compute.sshkey.*`, `compute.ingress.*`
   - NATS subscribes: `auth.identity.*`

   **Registry** (`services/registry/`):
   - OCI/Docker registry implementation
   - GFS client for blob storage, PostgreSQL for metadata
   - NATS publishes: `registry.image.*`, `registry.repository.*`
   - Config: `--master` for GFS, `DATABASE_URL`, `JWT_SECRET`

   **SFS** (`services/sfs/`):
   - Shared file system with RBAC
   - GFS client backend, WebSocket progress
   - NATS subscribes: identity events; publishes: `sfs.namespace.*`, `sfs.file.*`

   **Health** (`services/health/`):
   - Kubernetes cluster health via WebSocket
   - JWT token validation for pod filtering

   **Logging** (`services/logging/`):
   - gRPC server for structured logs, WebSocket streaming
   - GFS persistence

3. **Shared Package:** `pkg/events/` — NATS event consumer/producer, identity consumer

4. **Write Scope:** `edd-cloud-interface/services/`, `edd-cloud-interface/pkg/`. Flag changes needed elsewhere.

5. **Build/Test:** `go build .` and `go test ./...` within each service directory.

6. **Read-Only Access:** `proto/registry/`, `proto/sfs/`, `proto/compute/`, `manifests/`

7. **Output Contract:** Always return status, files changed, tests run, cross-service flags, summary.

8. **Error Handling:** If the task is outside your scope, report `status: failed` with a routing suggestion. If tests fail, attempt to fix; if you cannot, report `status: partial` with failure details.

- [ ] **Step 2: Commit**

Use the `commit-organizer` agent via the Agent tool to commit `.claude/agents/services-dev.md`.

---

### Task 4: Create gateway-dev Agent (Opus)

**Files:**
- Create: `.claude/agents/gateway-dev.md`

- [ ] **Step 1: Write the gateway-dev agent file**

**Frontmatter:**
- name: `gateway-dev`
- description: Examples covering routing, proxy, TLS, SSH tunneling
- model: `opus`
- color: `orange`

**Body content must include:**

1. **Role:** Expert Go developer for the edd-gateway multi-protocol proxy and custom ingress.

2. **Service Overview:**
   - Entry point: `main.go`
   - Multi-protocol: SSH (22), HTTP (80), HTTPS (443), custom (8000-8999)
   - Database-backed container routing

3. **Directory Structure:**
   - `internal/proxy/` — SSH, HTTP, TLS proxy implementations
   - `internal/router/` — Database-backed container routing
   - `internal/k8s/` — Kubernetes secret management for SSH keys

4. **Config:** `DATABASE_URL`, `ROUTES_FILE`, `--ssh-port`, `--http-port`, `--https-port`, `--fallback`, `--log-service`, `--tls-cert`, `--tls-key`

5. **Cross-Service Awareness:**
   - Computes service publishes ingress events that gateway consumes
   - Auth service provides JWT validation
   - Routes defined in `edd-gateway/manifests/gateway-routes.yaml` ConfigMap

6. **Key Pattern:** Traefik and k3s servicelb are DISABLED. This gateway handles ALL routing.

7. **Health Endpoints:** `/healthz` (liveness), `/readyz` (readiness)

8. **Read-Only Access:** `proto/gateway/`, `manifests/`

9. **Write Scope:** `edd-gateway/` only.

10. **Build/Test:** `go build .`, `go test ./...`

11. **Output Contract:** Always return status, files changed, tests run, cross-service flags, summary.

12. **Error Handling:** If the task is outside your scope, report `status: failed` with a routing suggestion. If tests fail, attempt to fix; if you cannot, report `status: partial` with failure details.

- [ ] **Step 2: Commit**

Use the `commit-organizer` agent via the Agent tool to commit `.claude/agents/gateway-dev.md`.

---

### Task 5: Create gfs-dev Agent (Opus)

**Files:**
- Create: `.claude/agents/gfs-dev.md`

- [ ] **Step 1: Write the gfs-dev agent file**

**Frontmatter:**
- name: `gfs-dev`
- description: Examples covering chunk management, replication, master/chunkserver, client SDK
- model: `opus`
- color: `red`

**Body content must include:**

1. **Role:** Expert Go developer for the go-gfs distributed file system — master, chunkserver, and client SDK.

2. **Three Binaries:**
   - `cmd/master/main.go` — metadata server (gRPC on 9000)
   - `cmd/chunkserver/main.go` — storage server (HTTP 8080, gRPC 8081)
   - `cmd/client/main.go` — CLI client

3. **Key Internal Packages:**
   - `internal/master/` — metadata, chunk management, namespace isolation
   - `internal/chunkserver/` — chunk storage, replication, data integrity
   - `pkg/go-gfs-sdk/` — client library (used by all other services)
   - `pkg/gfslog/` — structured logging client (used project-wide)
   - `proto/` — gRPC definitions (master.proto, logging.proto, chunkreplication.proto)

4. **Performance Characteristics:**
   - 550 ops/sec (83 MB/s) with 10 concurrent workers
   - ~7ms read path latency (0.67ms master + 0.71ms chunks + 5.5ms TCP read)
   - Optimized for large sequential I/O, not small file serving

5. **Build:** Makefile with `make proto`, `make master`, `make chunkserver`, `make client`, `make all`

6. **Cross-Service Impact:** Changes to `pkg/go-gfs-sdk/` or `pkg/gfslog/` affect ALL services. Always flag SDK changes.

7. **Read-Only Access:** `proto/`, `manifests/`

8. **Write Scope:** `go-gfs/` only.

9. **Build/Test:** Makefile — `make all`, `go test ./...`

10. **Output Contract:** Always return status, files changed, tests run, cross-service flags, summary.

11. **Error Handling:** If the task is outside your scope, report `status: failed` with a routing suggestion. If tests fail, attempt to fix; if you cannot, report `status: partial` with failure details.

- [ ] **Step 2: Commit**

Use the `commit-organizer` agent via the Agent tool to commit `.claude/agents/gfs-dev.md`.

---

### Task 6: Create infra-dev Agent (Opus)

**Files:**
- Create: `.claude/agents/infra-dev.md`

- [ ] **Step 1: Write the infra-dev agent file**

**Frontmatter:**
- name: `infra-dev`
- description: Examples covering K8s manifests, protobuf definitions, networking, storage config
- model: `opus`
- color: `yellow`

**Body content must include:**

1. **Role:** Expert in Kubernetes infrastructure and protobuf definitions for the edd-cloud platform.

2. **Manifests Scope** (`manifests/`):
   - Deployment, Service, ConfigMap, PVC manifests for all services
   - NATS, PostgreSQL, Calico networking, HAProxy, CoreDNS configs
   - CronJobs for maintenance tasks

3. **Proto Scope** (`proto/`):
   - Service definitions: auth, cluster, common, compute, gateway, log, notification, registry, sfs
   - Shared types in `proto/common/`

4. **Node Layout:**
   | Node | Role | Labels |
   |------|------|--------|
   | s0 | Database primary | `db-role=primary` |
   | rp1 | Database replica, HAProxy | `db-role=replica` |
   | rp2, rp3, rp4 | Backend services | `backend=true` |
   | s1, s2, s3 | GFS chunkservers | (hostNetwork) |

5. **Critical Rules:**
   - NEVER use `latest` tag — always timestamp tags (`YYYYMMDD-HHMMSS`)
   - NEVER use `kubectl apply` directly — deploy through CI/CD
   - Secrets in Kubernetes Secrets, NOT ConfigMaps
   - `externalTrafficPolicy: Local` for client IP preservation

6. **Cross-Service Awareness:** Read access to all service dirs for understanding what manifests/protos each service needs.

7. **Write Scope:** `manifests/`, `proto/` only.

8. **Proto Build:** Run `make proto` in the relevant service directory after proto changes.

9. **Output Contract:** Always return status, files changed, tests run, cross-service flags, summary.

10. **Error Handling:** If the task is outside your scope, report `status: failed` with a routing suggestion.

- [ ] **Step 2: Commit**

Use the `commit-organizer` agent via the Agent tool to commit `.claude/agents/infra-dev.md`.

---

### Task 7: Create frontend-dev Agent (Sonnet)

**Files:**
- Create: `.claude/agents/frontend-dev.md`

- [ ] **Step 1: Write the frontend-dev agent file**

**Frontmatter:**
- name: `frontend-dev`
- description: Examples covering React components, pages, UI bugs, dashboard features
- model: `sonnet`
- color: `green`

**Body content must include:**

1. **Role:** Expert React/TypeScript developer for the edd-cloud dashboard.

2. **Service Overview:**
   - Location: `edd-cloud-interface/frontend/`
   - Vite + React 18.3 + TypeScript
   - Tailwind CSS + Radix UI components
   - React Router for navigation

3. **Key Libraries:** xterm (terminal), xyflow (DAG visualization), recharts (metrics), Radix UI (components)

4. **Build/Test:** `npm run build`, `npm run lint`, `npm run type-check`

5. **Frontend Patterns (from project memory):**
   - Cross-origin downloads: use hidden `<iframe>`, not `link.click()`
   - Responsive/mobile: div-based layouts, `hidden md:grid`, overlay sidebar with backdrop

6. **API Domains:**
   - `auth.cloud.eddisonso.com` — Auth API
   - `storage.cloud.eddisonso.com` — Storage API
   - `compute.cloud.eddisonso.com` — Compute API
   - `health.cloud.eddisonso.com` — Health API
   - `docs.cloud.eddisonso.com` — Docs
   - NOT `cloud-api.eddisonso.com` (deprecated)

7. **Read-Only Access:** `edd-cloud-interface/services/` (for understanding backend APIs)

8. **Write Scope:** `edd-cloud-interface/frontend/` only.

9. **Output Contract:** Always return status, files changed, tests run, cross-service flags, summary.

10. **Error Handling:** If the task is outside your scope, report `status: failed` with a routing suggestion. If build/lint fails, attempt to fix; if you cannot, report `status: partial` with failure details.

- [ ] **Step 2: Commit**

Use the `commit-organizer` agent via the Agent tool to commit `.claude/agents/frontend-dev.md`.

---

### Task 8: Create log-dev Agent (Sonnet)

**Files:**
- Create: `.claude/agents/log-dev.md`

- [ ] **Step 1: Write the log-dev agent file**

**Frontmatter:**
- name: `log-dev`
- description: Examples covering log ingestion, streaming, gRPC server, NATS integration
- model: `sonnet`
- color: `gray`

**Body content must include:**

1. **Role:** Go developer for the log-service — centralized logging with gRPC and NATS.

2. **Service Overview:**
   - Entry: `main.go`
   - gRPC on `:50051`, HTTP/WebSocket on `:8080`
   - GFS persistence, NATS JetStream for error events

3. **Directory Structure:**
   - `internal/server/` — log aggregation server
   - `proto/logging/` — gRPC service definition

4. **Config:** `--grpc` (50051), `--http` (8080), `--master` (gfs-master:9000), `--nats` (nats://nats:4222)

5. **NATS:** Creates LOGS stream, publishes to `log.error.>`, consumed by alerting-service.

6. **Build:** Makefile with `make proto`, `make build`

7. **Read-Only Access:** `proto/log/`, `manifests/`

8. **Write Scope:** `log-service/` only.

9. **Output Contract:** Always return status, files changed, tests run, cross-service flags, summary.

10. **Error Handling:** If the task is outside your scope, report `status: failed` with a routing suggestion. If tests fail, attempt to fix; if you cannot, report `status: partial` with failure details.

- [ ] **Step 2: Commit**

Use the `commit-organizer` agent via the Agent tool to commit `.claude/agents/log-dev.md`.

---

### Task 9: Create monitor-dev Agent (Sonnet)

**Files:**
- Create: `.claude/agents/monitor-dev.md`

- [ ] **Step 1: Write the monitor-dev agent file**

**Frontmatter:**
- name: `monitor-dev`
- description: Examples covering node health, pod metrics, cluster status, WebSocket streaming
- model: `sonnet`
- color: `teal`

**Body content must include:**

1. **Role:** Go developer for cluster-monitor — Kubernetes health monitoring and metrics.

2. **Service Overview:**
   - Entry: `main.go`, HTTP/WebSocket on `:8080`
   - Kubernetes API for node/pod metrics
   - NATS JetStream for publishing metrics

3. **Key Packages:**
   - `internal/graph/` — metrics aggregation
   - `internal/timeseries/` — time-series data handling
   - `pkg/pb/` — protobuf definitions

4. **NATS Publishes:**
   - `cluster.metrics` — ClusterMetrics protobuf
   - `cluster.pods` — PodStatusSnapshot protobuf

5. **Config:** `--addr` (8080), `--log-service`, `JWT_SECRET`

6. **Pattern:** JWT token validation for pod filtering, user isolation (core ns vs compute-{userID}-* namespaces)

7. **Read-Only Access:** `proto/cluster/`, `manifests/`

8. **Write Scope:** `cluster-monitor/` only.

9. **Build/Test:** `go build .`, `go test ./...`

10. **Output Contract:** Always return status, files changed, tests run, cross-service flags, summary.

11. **Error Handling:** If the task is outside your scope, report `status: failed` with a routing suggestion. If tests fail, attempt to fix; if you cannot, report `status: partial` with failure details.

- [ ] **Step 2: Commit**

Use the `commit-organizer` agent via the Agent tool to commit `.claude/agents/monitor-dev.md`.

---

### Task 10: Create manager-dev Agent (Sonnet)

**Files:**
- Create: `.claude/agents/manager-dev.md`

- [ ] **Step 1: Write the manager-dev agent file**

**Frontmatter:**
- name: `manager-dev`
- description: Examples covering node cron jobs, terminal access, node info
- model: `sonnet`
- color: `brown`

**Body content must include:**

1. **Role:** Go developer for cluster-manager — lightweight node management utilities.

2. **Service Overview:**
   - Entry: `main.go`, HTTP on `:9090`
   - PTY-based terminal access via WebSocket
   - Node cron job scheduling
   - Host filesystem access via mount

3. **API Endpoints:**
   - `GET /healthz` — health check
   - `GET /info` — node info (requires auth)
   - `GET/POST/PUT/DELETE /cron` — cron job management (requires auth)
   - `GET /terminal` — WebSocket terminal

4. **Config:** `--addr` (9090), `--data-dir` (/var/lib/cluster-manager), `--host-root` (/host), `CLUSTER_MANAGER_SECRET`

5. **Read-Only Access:** `manifests/`

6. **Write Scope:** `cluster-manager/` only.

7. **Build/Test:** `go build .`, `go test ./...`

8. **Output Contract:** Always return status, files changed, tests run, cross-service flags, summary.

9. **Error Handling:** If the task is outside your scope, report `status: failed` with a routing suggestion. If tests fail, attempt to fix; if you cannot, report `status: partial` with failure details.

- [ ] **Step 2: Commit**

Use the `commit-organizer` agent via the Agent tool to commit `.claude/agents/manager-dev.md`.

---

### Task 11: Create alerting-dev Agent (Sonnet)

**Files:**
- Create: `.claude/agents/alerting-dev.md`

- [ ] **Step 1: Write the alerting-dev agent file**

**Frontmatter:**
- name: `alerting-dev`
- description: Examples covering Discord alerts, threshold monitoring, NATS event consumers
- model: `sonnet`
- color: `pink`

**Body content must include:**

1. **Role:** Go developer for alerting-service — event-driven alerting via NATS and Discord.

2. **Service Overview:**
   - Entry: `main.go`
   - Purely event-driven consumer (no inbound API calls)
   - Discord webhook for alert delivery
   - Health check on `:8080`

3. **Key Package:** `internal/alerting/` — alert evaluation and Discord sending

4. **NATS Subscriptions:**
   - `cluster.metrics` — CPU/memory/disk thresholds
   - `cluster.pods` — OOM tracking (state-based dedup, not time-based)
   - `log.error.>` — error burst detection (5 errors in 30s)

5. **Hardcoded Thresholds:** CPU 90%, Memory 85%, Disk 90%

6. **Config:** `--nats`, `--discord-webhook`, `--alert-cooldown` (5m), `--log-service-grpc`

7. **Pattern:** Event deduplication by state (restart count), not cooldown timers.

8. **Build:** Makefile with `make proto`, `make build`

9. **Read-Only Access:** `proto/`, `manifests/`

10. **Write Scope:** `alerting-service/` only.

11. **Output Contract:** Always return status, files changed, tests run, cross-service flags, summary.

12. **Error Handling:** If the task is outside your scope, report `status: failed` with a routing suggestion. If tests fail, attempt to fix; if you cannot, report `status: partial` with failure details.

- [ ] **Step 2: Commit**

Use the `commit-organizer` agent via the Agent tool to commit `.claude/agents/alerting-dev.md`.

---

### Task 12: Create notification-dev Agent (Sonnet)

**Files:**
- Create: `.claude/agents/notification-dev.md`

- [ ] **Step 1: Write the notification-dev agent file**

**Frontmatter:**
- name: `notification-dev`
- description: Examples covering user notifications, WebSocket delivery, NATS consumers
- model: `sonnet`
- color: `indigo`

**Body content must include:**

1. **Role:** Go developer for notification-service — user notification delivery.

2. **Service Overview:**
   - Entry: `cmd/notification/main.go`, HTTP on `:8080`
   - WebSocket for real-time notification delivery
   - NATS JetStream consumer for `notify.>` subjects
   - PostgreSQL for notification storage

3. **Directory Structure:**
   - `internal/api/` — HTTP/WebSocket handlers
   - `internal/db/` — database models
   - `internal/consumer/` — NATS event consumer
   - `pkg/publisher/` — notification publisher helper (used by other services)

4. **NATS:** Creates `NOTIFICATIONS` stream, durable consumer `notification-service`, subscribes to `notify.>`

5. **Config:** `DATABASE_URL`, `JWT_SECRET`, `NATS_URL`, `--addr` (8080), `--log-service`

6. **Cross-Service Note:** `pkg/publisher/` is imported by compute and sfs services for sending notifications.

7. **Build:** Makefile with `make proto`, `make build`

8. **Read-Only Access:** `proto/notification/`, `manifests/`

9. **Write Scope:** `notification-service/` only.

10. **Output Contract:** Always return status, files changed, tests run, cross-service flags, summary.

11. **Error Handling:** If the task is outside your scope, report `status: failed` with a routing suggestion. If tests fail, attempt to fix; if you cannot, report `status: partial` with failure details.

- [ ] **Step 2: Commit**

Use the `commit-organizer` agent via the Agent tool to commit `.claude/agents/notification-dev.md`.

---

### Task 13: Create docs-dev Agent (Sonnet)

**Files:**
- Create: `.claude/agents/docs-dev.md`

- [ ] **Step 1: Write the docs-dev agent file**

**Frontmatter:**
- name: `docs-dev`
- description: Examples covering documentation updates, API docs, guides, architecture pages
- model: `sonnet`
- color: `white`

**Body content must include:**

1. **Role:** Technical writer and Docusaurus developer for edd-cloud-docs.

2. **Service Overview:**
   - Location: `edd-cloud-docs/`
   - Docusaurus 3.9.2 with React 19, MDX, Mermaid diagrams
   - Served via nginx in production

3. **Build/Test:** `npm run build`, `npm run typecheck`

4. **Cross-Service Awareness:** Read access to ALL service directories to understand what needs documenting. When APIs change in any service, this agent updates the corresponding docs.

5. **Read-Only Access:** All service directories

6. **Write Scope:** `edd-cloud-docs/` only.

7. **Output Contract:** Always return status, files changed, tests run, cross-service flags, summary.

8. **Error Handling:** If the task is outside your scope, report `status: failed` with a routing suggestion. If build fails, attempt to fix; if you cannot, report `status: partial` with failure details.

- [ ] **Step 2: Commit**

Use the `commit-organizer` agent via the Agent tool to commit `.claude/agents/docs-dev.md`.

---

### Task 14: Smoke Test the Orchestrator

- [ ] **Step 1: Verify all agent files exist**

Run: `ls -la .claude/agents/*.md | wc -l`
Expected: 17 files (13 new + 4 existing: actions-monitor, docs-writer, issue-diagnostician, k8s-log-scanner)

- [ ] **Step 2: Verify frontmatter format**

Run: `head -7 .claude/agents/cloud-orchestrator.md`
Expected: Valid YAML frontmatter with name, description, model, color fields

- [ ] **Step 3: Test orchestrator routing with a simple request**

Invoke `cloud-orchestrator` with prompt: "What agent would you route 'fix SSH key deletion' to?"
Expected: Routes to `auth-dev` (backend) and potentially `frontend-dev` (UI)

- [ ] **Step 4: Test direct agent invocation**

Invoke `auth-dev` with prompt: "List the files in your write scope"
Expected: Lists files under `edd-cloud-auth/`

- [ ] **Step 5: Final commit of any adjustments**

Commit any fixes discovered during smoke testing.
