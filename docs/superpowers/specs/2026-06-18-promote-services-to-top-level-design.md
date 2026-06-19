# Promote Nested Services to Top Level

**Date:** 2026-06-18
**Status:** Approved (design) — pending implementation plan

## Goal

Make the monorepo layout consistent: backend services currently nested under
`edd-cloud-interface/services/` become top-level directories, like every other
backend (`cluster-monitor`, `log-service`, `edd-cloud-auth`, `edd-gateway`,
`alerting-service`, `notification-service`, `cluster-manager`). `edd-cloud-interface`
is left holding only the frontend. The README/docs are corrected to match.

This is a **mechanical, repo-wide restructure**, independent of the
remove-shared-resources effort. The risk is in the build wiring (Go `replace`
directives, Docker build contexts, CI path filters), not in the application code.

## Decisions (locked)

1. **Move set:** promote the nested backend services to the top level; the frontend
   stays in `edd-cloud-interface` (no rename).
2. **Go module paths unchanged** — only relative `replace` directives are fixed. No
   import statements change.
3. **Duplicate logging:** `edd-cloud-interface/services/logging/` is dead and is
   **deleted**; the live `log-service/` (already top-level) is untouched.

## Current-state facts (verified)

- The nested services are **already independent modules**: each has its own `go.mod`
  and `Dockerfile` and is built/deployed separately by CI. The root
  `edd-cloud-interface/go.mod` is effectively empty (1 line). `edd-cloud-interface/`
  is a pure directory grouping, not a shared module.
- Contents of `edd-cloud-interface/`: `frontend/`, `pkg/events/` (shared module
  `eddisonso.com/edd-cloud/pkg/events`), `services/{compute,health,logging,registry,sfs}`.
- **Module-path naming is already inconsistent:** compute/health/logging/sfs use
  `eddisonso.com/edd-cloud/services/<name>`; **registry is already flat**
  (`eddisonso.com/edd-cloud-registry`).
- **`services/logging/` is dead code:** no reference in `.github/workflows/build-deploy.yml`
  (no path filter, no build step, no deploy). Last touched 2026-02-07; its proto
  `go_package` points at `github.com/eddisonso/log-service`. The live logging service
  is top-level `log-service/` (CI watches `log-service/**`, builds `log-service/Dockerfile`,
  deploys `deployment/log-service`, image `eddisonso/ecloud-logging`; last touched 2026-02-15).
- **`replace` directives use deep relative paths**, e.g. in
  `services/sfs/go.mod`: `replace eddisonso.com/go-gfs => ../../../go-gfs`,
  `replace eddisonso.com/edd-cloud/pkg/events => ../../pkg/events`,
  `replace eddisonso.com/notification-service => ../../../notification-service`.
  Similar in compute/registry/health/logging. Moving a service up two directory
  levels changes every one of these.
- README mislabels `go-gfs` as "shared library" — it is both a deployed system
  (master + chunkservers) and a consumed SDK.

## Final layout

```
<repo root>/
├── compute/            (from edd-cloud-interface/services/compute)
├── health/             (from edd-cloud-interface/services/health)
├── registry/           (from edd-cloud-interface/services/registry)
├── sfs/                (from edd-cloud-interface/services/sfs)
├── pkg/events/         (from edd-cloud-interface/pkg/events) — see Open decision
├── edd-cloud-interface/
│   └── frontend/       (unchanged; root go.mod removed if empty)
├── log-service/        (unchanged — already live & top-level)
├── go-gfs/ notification-service/ ... (unchanged)
```

`edd-cloud-interface/services/logging/` is deleted.

## Plan of work

### 1. Move directories (use `git mv` to preserve history)
- `git mv edd-cloud-interface/services/{compute,health,registry,sfs}` → repo root.
- `git rm -r edd-cloud-interface/services/logging` (dead).
- Decide `pkg/events` home (see Open decision); move with `git mv` if relocating.
- Remove the now-empty `edd-cloud-interface/services/` dir and the empty root
  `edd-cloud-interface/go.mod`.

### 2. Fix Go `replace` directives (module paths stay the same)
For each moved service, recompute relative paths from the new top-level location:
- `../../../go-gfs` → `../go-gfs`
- `../../../notification-service` → `../notification-service`
- `../../pkg/events` → `../pkg/events` (if pkg/events moves to top-level `pkg/events`)
  or `../edd-cloud-interface/pkg/events` (if it stays).
Run `go mod tidy` / `go build ./...` in each moved module to confirm resolution.

### 3. Docker build contexts (PRIMARY RISK — validate first)
These are multi-module builds: each service's `replace` points outside its own dir
(`go-gfs`, `pkg/events`, `notification-service`), so the Docker **build context** must
include those sibling dirs, and the Dockerfile `COPY` paths are relative to that
context. Moving services up two levels changes both the context root and those
relative paths.
- Inspect each `Dockerfile` (`COPY ../../...` patterns) and the CI build invocation
  (`-f <dir>/Dockerfile <context>`).
- Adjust `COPY` paths and the CI `-f`/context arguments so the build still reaches
  `go-gfs`, `pkg/events`, `notification-service`.
- This must be proven with a local `docker build` for at least one service (sfs or
  compute, which have the most `replace` deps) before committing.

### 4. CI — `.github/workflows/build-deploy.yml`
Update for each moved service:
- `detect-changes` path filters: `edd-cloud-interface/services/<svc>/**` → `<svc>/**`.
- Build steps: `-f edd-cloud-interface/services/<svc>/Dockerfile` → `-f <svc>/Dockerfile`
  and the corresponding build context.
- Compute base image context: `edd-cloud-interface/services/compute/images/base/**`
  and its build context → `compute/images/base/**`.
- Frontend paths (`edd-cloud-interface/frontend/**`) are unchanged.
- The catch-all `edd-cloud-interface/**` filter (line ~106) must be re-scoped so it no
  longer implies the moved services.
- Image names, deployment names, and `kubectl set image` targets are **unchanged**
  (image tags drive k8s, not paths) — so **no Kubernetes/manifest change is required**.

### 5. Docs
- README "Structure" section: list the promoted services at top level; fix the
  `go-gfs` description ("distributed file system: master + chunkservers, plus a
  consumed Go SDK"); drop the "(sfs, logging, compute, frontend)" grouping line so
  `edd-cloud-interface` reads as the frontend.
- Update `edd-cloud-docs` architecture/structure pages if they reference the old paths.
- CLAUDE.md already calls `edd-cloud-interface` the frontend dashboard — leave it
  (it is gitignored and not committed).

### 6. Repo-wide path references
Grep for any remaining hardcoded `edd-cloud-interface/services/` strings (Makefiles,
scripts, proto generation paths, `claude_context.md`, `.claude/agents/*`) and update
them. The agent definitions (`cloud-orchestrator`, `services-dev`, etc.) reference
these paths and should be corrected.

## Validation / rollout

- **Atomic change:** the directory moves, `replace` fixes, Docker context fixes, and CI
  edits must land in **one commit/PR** — a partial move breaks the build.
- Pre-merge: `go build ./...` (or `go vet`) green in each moved module; one local
  `docker build` per high-dependency service succeeds.
- Post-merge: watch the CI run — confirm `detect-changes` flags the moved services,
  they build, and deploy to the **same** deployment names (no new ReplicaSets).
- No image-tag or manifest change means the running cluster is unaffected beyond the
  normal redeploy of rebuilt services.

## Open decision (recommended default)

- **`pkg/events` home:** recommend moving it to top-level `pkg/events`. Leaving a
  shared backend module inside `edd-cloud-interface/` (now the frontend dir) is the
  same smell we're removing. If you'd rather minimize churn, it can stay and the
  `replace` becomes `../edd-cloud-interface/pkg/events`. Defaulting to **move it**.

## Risks

- Docker multi-module build contexts are the most fragile part — `replace` deps mean
  the context must reach sibling repos; relative `COPY`/context paths change with the
  move (mitigated by step 3 validation).
- Missed hardcoded path in a script/Makefile/agent breaks that tool until fixed.
- CI catch-all `edd-cloud-interface/**` filter, if left, could mis-trigger or
  under-trigger builds after the move.

## Out of scope

- Renaming Go module paths to flat names (explicitly deferred — paths stay unchanged).
- Renaming `edd-cloud-interface` → `edd-cloud-frontend` (frontend stays put).
- Any application/behavior change (this is purely structural).
- The remove-shared-resources work (separate spec:
  `2026-06-18-remove-shared-resources-design.md`).
```
