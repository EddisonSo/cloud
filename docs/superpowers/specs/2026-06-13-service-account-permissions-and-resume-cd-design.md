# Service-Account Permissions Expansion + Resume CD — Design

**Date:** 2026-06-13
**Status:** Approved
**Track:** 1 of 2 (Track 2 = networking SA auth, separate spec)

## Goal

Give the service-account permission model the granularity needed to drive a real
deploy from CI, and use it to wire continuous deployment for the resume site
(`registry.cloud.eddisonso.com/eddison/resume`) running as compute container
`507b1b10`. After this, pushing to the resume repo rebuilds the image, pushes it
to the edd-cloud registry, and redeploys the container — with a least-privilege
token that can do exactly that and nothing more.

## Background (current state, verified in code)

- **Scope model:** `<root>.<userid>[.<resource>[.<id>]]`. `validateScopes`
  (`edd-cloud-auth/internal/api/tokens.go`) allows roots `{compute, storage}`,
  resources `compute → {containers, keys}`, `storage → {namespaces, files}`,
  actions `create/read/update/delete`.
- **SA tokens are JWTs:** `ecloud_` + an HS256 JWT carrying a `scopes`
  (`map[string][]string`) claim, signed with the shared `jwtSecret`
  (`tokens.go:handleCreateToken`).
- **Registry already keys on `storage`:** `filterAccessByServiceAccount`
  (`edd-cloud-auth/internal/api/registry_token.go`) looks up
  `storage.<uid>.registry.<repo>` (or wildcard `storage.<uid>.registry`), but
  `validateScopes` has no `registry` resource under `storage`, so that scope
  **cannot be stored today** — registry SA access is currently impossible.
- **Action translation:** `translateSAActionToOCI` maps `read → pull` and
  `create/update/delete → push`. There is no OCI `delete`.
- **Compute lifecycle is coarse:** `POST /compute/containers/{id}/stop` and
  `/start` are both gated by `scopeCheckContainer("update", …)`
  (`services/compute/internal/api/handler.go`), the same action that also covers
  the interactive terminal, SSH toggle, ingress, and mounts — so a start/stop
  token would also grant shell access.
- **Pull policy:** `CreatePod` (`services/compute/internal/k8s/client.go`) does
  not set `ImagePullPolicy`, so Kubernetes defaults it from the tag (`:amd64` →
  `IfNotPresent`). `StartContainer` recreates the pod from the stored
  `container.Image`, so a stop/start re-pulls only when the policy is `Always`.

## Scope

**In:** B (start/stop actions), D (registry resource under storage, push/pull/delete),
E (frontend picker for the above), compute pull-strategy field + dropdown,
resume CI rewiring.

**Out:** Networking SA auth (Track 2). Splitting ingress/mounts/per-container SSH
toggle out of `containers:update` (deferred — gap C). Zero-downtime deploys.

## Design by component

### 1. Auth service (`edd-cloud-auth`)

- **B — start/stop actions.** Add `start` and `stop` to the allowed compute
  container actions in `validateScopes`. Scope strings are unchanged
  (`compute.<uid>.containers` and `compute.<uid>.containers.<id>`); only the
  action vocabulary grows.
- **D — registry resource.** Add `registry` to the allowed resources under the
  `storage` root in `validateScopes`, with actions `push`, `pull`, `delete`.
  Because `filterAccessByServiceAccount` already reads
  `storage.<uid>.registry.<repo>`, this immediately makes registry SA scopes
  storable and honored.
- **D — real delete.** The registry resource stores its actions natively as
  `push`/`pull`/`delete`. In `translateSAActionToOCI`, `push` and `pull` already
  pass through via the `default` case; the only change needed is mapping
  `delete → delete` (it currently returns `push`). The legacy
  `read → pull` and `create/update → push` cases stay for back-compat with any
  older stored scopes.

### 2. Registry service (`edd-cloud-interface/services/registry`)

- Honor an OCI `delete` action in the auth check (`hasAccess`) and implement
  manifest deletion (`DELETE /v2/<name>/manifests/<reference>`), so a token with
  the `delete` registry action can remove an image/tag. (Push/pull already work.)

### 3. Compute service (`edd-cloud-interface/services/compute`)

- **Pull strategy field.** Add `pull_policy` to the container model
  (`db/containers.go`, new column; DB migration), accept it in the
  `CreateContainer` request body (`internal/api/containers.go`), validate it to
  `{Always, IfNotPresent}` and **default `IfNotPresent`** (no behavior change for
  existing/other containers). Thread it into `CreatePod`, which sets
  `Container.ImagePullPolicy` accordingly. `StartContainer` already recreates
  from the stored container, so the policy persists across stop/start — making
  stop/start a true redeploy when `Always`.
- **Start/stop scope split.** Change the two routes in `handler.go` from
  `scopeCheckContainer("update", …)` to `scopeCheckContainer("start", …)` and
  `("stop", …)`. The existing scope walk-up still lets a broad
  `compute.<uid>.containers:update` (or `:start`/`:stop`) token through; the new
  capability is that a token can be granted *only* start/stop.

### 4. Frontend (`edd-cloud-interface/frontend`)

- **E — picker.** In `PermissionPicker.tsx`, add `start` and `stop` to
  `CONTAINER_ACTIONS` so they render on both the **All Containers** and
  **Specific Containers** rows. Add a **Registry** group under Storage with the
  three native actions `push` / `pull` / `delete` (its own resource, mapping to
  `storage.<uid>.registry[.<repo>]`).
- **Pull-strategy dropdown.** Add an image-pull-strategy `<select>`
  (`Always` / `If Not Present`, default `If Not Present`) to the container create
  form, sent as `pull_policy` in the create request.

### 5. Resume CI (`resume_website/.github/workflows/build-push.yml`)

Replace the dead GKE `deploy` job with a `deploy` job that:
1. Logs in: `docker login registry.cloud.eddisonso.com -u <user> -p $ECLOUD_TOKEN`
   (the SA token is the Basic-auth password; `ecloud_` recognized by
   `authenticateRegistryServiceAccount`).
2. Builds and pushes `registry.cloud.eddisonso.com/eddison/resume:amd64`
   (replacing the Docker Hub push, which the live container never pulled).
3. Redeploys: `POST cloud-api.eddisonso.com/compute/containers/507b1b10/stop`
   then `/start`, with `Authorization: Bearer $ECLOUD_TOKEN`.

`$ECLOUD_TOKEN` is a GitHub Actions secret holding an SA token scoped to exactly:
`storage.<uid>.registry.eddison/resume: [push]` and
`compute.<uid>.containers.507b1b10: [start, stop]`.

## Data flow (deploy)

```
push to main
  └─ CI: go test + generate HTML  (existing)
  └─ CI: docker build
  └─ CI: docker push registry.cloud.eddisonso.com/eddison/resume:amd64   (SA token)
  └─ CI: POST /compute/containers/507b1b10/stop                          (SA token)
  └─ CI: POST /compute/containers/507b1b10/start                         (SA token)
        └─ compute recreates pod from stored image, pull_policy=Always
             └─ node re-pulls :amd64, new digest goes live (few-sec downtime)
```

## One-time setup (rollout)

1. Apply the compute DB migration (`pull_policy` column).
2. Redeploy auth, registry, compute, and frontend with the changes.
3. Reconfigure the resume container to `pull_policy: Always` (recreate the pod
   once via the API; namespace/LB IP/ingress/SSH are preserved — only the pod is
   replaced, so `eddisonso.com` routing is untouched).
4. Mint the scoped SA token; store it as the `ECLOUD_TOKEN` GitHub secret.
5. Update and merge the resume CI change.

## Security

- The CD token is least-privilege: `registry push` on one repo + `start`/`stop`
  on one container. It cannot open a terminal, change SSH, edit ingress/mounts,
  delete the container, or touch other users' resources (`validateScopes`
  enforces the `<uid>` segment).
- `pull_policy` defaults to `IfNotPresent`: no behavior change for any existing
  container; `Always` is opt-in per container.
- Registry `delete` is a distinct grantable action, not implied by push.

## Testing

- **Unit (auth):** `validateScopes` accepts `start`/`stop` and
  `storage.<uid>.registry[.<repo>]: push/pull/delete`, and still rejects unknown
  roots/resources/actions and cross-user scopes. `translateSAActionToOCI`:
  `delete → delete`, `read → pull`, `push → push`.
- **Unit (compute):** `CreatePod` sets `ImagePullPolicy=Always` when configured
  and leaves the default otherwise; `CreateContainer` validates `pull_policy` and
  defaults `IfNotPresent`; start/stop routes return 403 for a token lacking the
  new action and 200 with it.
- **Unit (registry):** `hasAccess` grants manifest delete only with the `delete`
  action; manifest delete removes the tag.
- **Integration:** with a scoped SA token, push a new image and run stop/start;
  assert the served digest changes.
- **Migration check:** after redeploy, `eddisonso.com` still serves the resume
  (text-content unchanged) — proves the pod recreation preserved routing.

## Open items / follow-ups

- Gap C (split ingress/mounts/ssh-toggle out of `containers:update`) — deferred.
- Track 2 (networking SA auth) — separate spec.
