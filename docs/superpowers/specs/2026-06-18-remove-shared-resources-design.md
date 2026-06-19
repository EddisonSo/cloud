# Remove Shared Resources — Private-by-Default Platform

**Date:** 2026-06-18
**Status:** Approved (design) — pending implementation plan

## Goal

Eliminate "shared resources" across the edd-cloud platform: anything readable by
users other than the owner, or by anonymous/unauthenticated callers, where that
exposure is not strictly intended. The platform becomes **private-by-default**.

This covers two distinct categories that require different treatment:

- **Category A — intentional shared-visibility features** (SFS storage namespaces,
  registry repositories). These are *features being narrowed/removed*.
- **Category B — cross-user / anonymous leaks** (cluster-monitor metrics,
  log-service log streaming, health cluster-info). These are *missing-auth bugs
  being closed*.

## Decisions (locked)

1. **SFS** keeps a **2-level** model: `private` (owner + service account only) and
   `public` (anyone *with the direct link* can read, but **never listed/advertised**).
   The thing removed is cross-user *discovery*, not link access.
2. **Registry** becomes **private-only** — no public level at all.
3. **Existing data** in both SFS and registry is **force-migrated to private**.
   Anonymous pulls / public links that users currently rely on will break (accepted).
   SFS owners may opt namespaces back to public afterward.
4. **Category B is in scope** in this same effort.
5. **Global infrastructure endpoints are admin-only.**
6. **SFS create defaults to `private`** (was `public`).
7. **`core` namespace metrics become admin-only** — non-admins see only their own
   `compute-{userID}-*` pods.

## Current-state facts (verified)

- **SFS** (`edd-cloud-interface/services/sfs/main.go`): 3 visibility levels —
  `visibilityPrivate=0`, `visibilityVisible=1` (unlisted, link-accessible),
  `visibilityPublic=2` (advertised + link-accessible). Default on create is
  `public(2)`. Namespace `name` is a **global primary key** (flat global namespace,
  one owner per name). Seeded global namespaces: `default` (public), `hidden`
  (private demo). Access enforced in `canAccessNamespace()` (`:1815`), listing in
  `handleNamespaceList()` (`:484`), ownership in `isNamespaceOwner()` (`:1838`).
  SA scope model in `rbac.go` (`storage.<user_id>.<resource>...`).
- **Registry** (`edd-cloud-interface/services/registry/`): `visibility` int,
  `0=private`, `>0=public` (`db.go:23`). Anonymous manifest pull when `visibility==1`
  (`manifests.go:87`); public filtering in `/v2/_catalog` (`:418`) and `/api/repos`
  (`api.go:140`); visibility set via `PUT /api/repos/{name}/visibility` (`api.go:278`).
  Blobs always require a token (`blobs.go:103,145`). Auth side issues anonymous /
  public pull tokens in `edd-cloud-auth/internal/api/registry_token.go` (~`:284,351`).
- **cluster-monitor** (`cluster-monitor/main.go`): `/pod-metrics`, `/ws/pod-metrics`,
  `/sse/pod-metrics`, `/api/metrics/pods` return all pods unfiltered/unauthenticated;
  `/cluster-info` and `/api/graph/dependencies` expose node + topology data publicly.
  `/sse/health` (`:1008-1067`) is the correct pattern: JWT → `userID` →
  `filterPodsForUser()` (`:83-102`), namespace convention `compute-{userID}-*`
  (also currently includes `core`).
- **log-service** (`log-service/main.go:119`): `/ws/logs` has **no auth**; entries
  keyed only by `source` (pod name) — `LogEntry` (`proto/logging/logging.proto:12-18`)
  has **no user/namespace field**. gRPC `GetLogs`/`StreamLogs` accept `source` +
  `min_level` only. Per-user filtering is not possible without a data-model change.
- **health** (`edd-cloud-interface/services/health/main.go:103`): `/cluster-info`,
  `/ws/cluster-info` expose cluster-wide node data unauthenticated.
- **Admin concept**: weak. `isAdmin()` compares `claims.Username` to the
  `ADMIN_USERNAME` env var at response time (`edd-cloud-auth/internal/api/handler.go:123`);
  **not stored in the JWT or DB**. `JWTClaims` (`handler.go:116-121`) carries
  `Username`, `DisplayName`, `UserID` + `RegisteredClaims`; no `scopes`, no `is_admin`.
  Validated across services with the shared secret.
- **Clean services** (no cross-user exposure): compute, cluster-manager, notification,
  alerting, go-gfs.

## Workstream 1 — SFS storage

**Outcome:** 2-level visibility, no cross-user discovery.

Changes in `edd-cloud-interface/services/sfs/`:

- Collapse constants to `visibilityPrivate=0`, `visibilityPublic=1`. Map legacy
  values on read so any residual `2` is treated as public(1); writes only emit 0/1.
- `handleNamespaceList()` (`main.go:484`): **always** filter to
  `owner_id == currentUserID`. A namespace owned by another user is never listed,
  regardless of visibility. Anonymous callers receive an empty list.
- `canAccessNamespace()` (`main.go:1815`): `private` → owner OR a matching SA scope
  for that owner; `public` → anyone, read-only (direct-link access preserved).
- Namespace create (`main.go:530-585`): default visibility = **private(0)**;
  accept only 0/1 in the payload.
- Visibility-update endpoints (`PATCH` `:641`, `PUT` `:731`): accept only {0,1};
  keep owner-only enforcement via `isNamespaceOwner()`.
- Stop seeding the global public `default`/`hidden` demo namespaces (`main.go:391`).
  Remove the seed, or seed nothing; `default` is no longer auto-public. (Confirm no
  service hard-depends on a global `default` namespace existing; if so, seed it
  private with a defined owner or skip.)
- **Migration (one-time, guarded):** `UPDATE namespaces SET visibility=0` for all
  existing rows. The `visibility` column is retained with domain {0,1}. The `hidden`
  column is left in place (vestigial) but no longer written.
- Verify SA-scoped tokens (`storage.<owner>.files...`) still resolve to owner-level
  access on private namespaces after the change.

## Workstream 2 — Registry

**Outcome:** private-only; no public repos, no anonymous pulls, no public catalog.

Changes in `edd-cloud-interface/services/registry/`:

- `manifests.go:87` (and `:393` tags): remove the `visibility==1` anonymous-pull
  bypass — always require a token with the `pull` action.
- `/v2/_catalog` (`manifests.go:418`): drop the `visibility>0` branch; authenticated
  callers see only their own repos; anonymous → 401.
- `/api/repos` (`api.go:140`): query becomes `owner_id = $1` only (drop
  `OR visibility>0`); anonymous → 401.
- `/api/repos/{name}` and `/tags` (`api.go:196`): owner/SA only; remove the
  `visibility!=1` public allowance.
- Remove the `PUT /api/repos/{name}/visibility` endpoint (`api.go:278`).
- **Migration:** `UPDATE repositories SET visibility=0`. Column retained, always 0.

Changes in `edd-cloud-auth/internal/api/registry_token.go`:

- Remove anonymous and public-repo pull-token issuance (~`:284`, `:351`). Tokens are
  issued only for owner or a SA with explicit access. Remove the anonymous auth path.

**Compute impact:** compute mints owner-scoped pull tokens
(`compute/internal/api/containers.go:827`), so a user's own images keep working.
Pulling another user's (formerly public) base image breaks (accepted). **Action:**
confirm whether any shared/system base image is distributed via a public repo; if so,
it needs a separate distribution mechanism (out of scope — flag, do not silently break).

## Workstream 3 — Close cross-user / anonymous leaks (Category B)

### 3a. Per-user data endpoints → JWT-required + own-namespace filter

`cluster-monitor/main.go`: `/pod-metrics`, `/ws/pod-metrics`, `/sse/pod-metrics`,
`/api/metrics/pods`.

- Require a valid JWT (401 otherwise).
- Filter results with `filterPodsForUser(pods, userID)`.
- `/api/metrics/pods`: the `namespace` query param is honored only if it belongs to
  the caller (`compute-{userID}-*`) or the caller is admin; otherwise ignored/403.
- **`core` is admin-only**: update `filterPodsForUser()` so non-admins see only
  `compute-{userID}-*` (drop `core`); admins see all namespaces. Pass an `isAdmin`
  signal into the filter.

### 3b. Global infrastructure endpoints → admin-only

Admin-required: `cluster-monitor` `/cluster-info`, `/api/graph/dependencies`
(+ any ws variant); `health` `/cluster-info`, `/ws/cluster-info`.

**Admin mechanism (new):** add `IsAdmin bool` to `JWTClaims`
(`edd-cloud-auth/internal/api/handler.go:116`), populated at token issuance from the
existing `ADMIN_USERNAME` check. No DB change. Consumer services validate the claim
(`claims.IsAdmin`) for admin-gated routes. (Alternative considered and rejected:
per-service `ADMIN_USERNAME` compared to `claims.Username` — avoids the token change
but duplicates config across every service.)

Token-version note: existing issued tokens won't carry `IsAdmin` until reissued.
Acceptable — admins re-login; the claim is absent → treated as non-admin until then.

### 3c. log-service — phased (no user field exists today)

`log-service`: `/ws/logs` (`main.go:119`) and gRPC `GetLogs`/`StreamLogs`.

- **Phase 1 (this effort):** require auth on `/ws/logs` (no anonymous). Streaming an
  arbitrary `source` (cross-pod) becomes admin-only. Without per-entry ownership data,
  non-admins cannot yet be scoped to their own logs, so non-admin access is denied at
  this phase except where a source can be proven to belong to the caller.
- **Phase 2 (follow-up, same spec):** add a `namespace` field to `LogEntry`
  (`proto/logging/logging.proto`), have producer services tag logs with their
  namespace, then filter `/ws/logs` and `GetLogs` by ownership
  (`namespace` has prefix `compute-{userID}-`). Non-admins stream their own container
  logs. Requires coordinated producer changes across services.

## Cross-cutting concerns

- **Migrations:** implement as one-time guarded `UPDATE`s (tracked so they do not
  re-run on every service startup and clobber later public opt-ins). The services
  currently auto-create schema at startup — add the migration in a way that runs once
  (e.g. a `schema_migrations` marker or run-once guard).
- **Deploy order:** `edd-cloud-auth` (JWT `IsAdmin` claim) first → `cluster-monitor`
  and `health` (claim consumers) → `sfs` and `registry`. Bump each service manifest's
  pinned image tag to the CI-built tag so a later unrelated deploy doesn't revert the
  service (deploy-discipline lesson).
- **CORS:** every newly auth-gated endpoint must keep the shared `corsMiddleware` so a
  tokenless preflight returns 200 before auth runs (dashboard cross-origin rule).
- **Docs:** update `edd-cloud-docs` storage, registry, health, and networking pages to
  reflect the 2-level SFS model, private-only registry, removed public catalog, and
  admin-gated infra endpoints.
- **Frontend (flag for frontend-dev):** removing the public catalog/listing, dropping
  `core` from user metrics, and admin-gating cluster-info may require dashboard
  adjustments (hide public-toggle UI, admin-only infra views).

## Risks

- Forcing existing public/unlisted → private breaks live anonymous pulls and public
  links (accepted per decision 3).
- A shared/system base image distributed via a public registry repo would break
  compute pulls — must be confirmed before rollout (Workstream 2 action).
- `core`-namespace metric removal changes the dashboard health view for non-admins.
- log-service per-user filtering is not available until Phase 2 (producer changes).
- New issued tokens needed for `IsAdmin`; admins re-login after auth deploy.

## Out of scope

- Migrating the SFS global flat-namespace model to per-user namespaces.
- A full RBAC/admin role system in the DB (only the single `IsAdmin` JWT claim is added).
- A replacement distribution mechanism for shared/system base images (if any exist).
