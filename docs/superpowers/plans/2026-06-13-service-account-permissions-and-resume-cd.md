# Service-Account Permissions + Resume CD — Implementation Plan (Track 1)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `start`/`stop` container actions and a `registry` resource (push/pull/delete) to the service-account scope model, add a per-container image pull-strategy, surface both in the dashboard, and rewire the resume repo's CI to push to the edd-cloud registry and redeploy container `507b1b10` with a least-privilege SA token.

**Architecture:** Backend changes are small, well-isolated edits in three Go services (`edd-cloud-auth`, `services/registry`, `services/compute`), each test-driven. The frontend adds actions/a group to the existing `PermissionPicker` and a `<select>` to the create form, verified by typecheck/build (the frontend has no test runner). The resume CI change is in a second repo. A final operator checklist covers DB migration, redeploys, container reconfigure, and secret minting.

**Tech Stack:** Go 1.x (HS256 JWT via `golang-jwt/jwt/v5`, `k8s.io/api/core/v1`), Postgres, React/TypeScript (Vite), GitHub Actions, Docker.

**Spec:** `docs/superpowers/specs/2026-06-13-service-account-permissions-and-resume-cd-design.md`

**Repos & branches:**
- `~/cloud` — branch `feat/sa-permissions-resume-cd` (already created). All platform changes.
- `~/projects/resume_website` — CI change (branch it: `feat/edd-cloud-cd`).

**Commit policy:** No `Co-Authored-By` trailers (repo owner's global rule).

## File structure

| File | Change |
|---|---|
| `~/cloud/edd-cloud-auth/internal/api/tokens.go` | `validateScopes`: resource-aware action sets (+`start`/`stop` on containers, +`registry` resource under storage with `push`/`pull`/`delete`) |
| `~/cloud/edd-cloud-auth/internal/api/registry_token.go` | `translateSAActionToOCI`: `delete → delete` |
| `~/cloud/edd-cloud-auth/internal/api/tokens_test.go` | **new** — unit tests for `validateScopes` |
| `~/cloud/edd-cloud-auth/internal/api/registry_token_test.go` | **new** — unit tests for `translateSAActionToOCI` |
| `~/cloud/edd-cloud-interface/services/registry/*` (auth.go + manifest handler) | honor OCI `delete`; implement manifest delete |
| `~/cloud/edd-cloud-interface/services/compute/internal/db/db.go` | migration: add `pull_policy` column |
| `~/cloud/edd-cloud-interface/services/compute/internal/db/containers.go` | persist + read `pull_policy` |
| `~/cloud/edd-cloud-interface/services/compute/internal/api/containers.go` | accept/validate/default `pull_policy` |
| `~/cloud/edd-cloud-interface/services/compute/internal/k8s/client.go` | `CreatePod` sets `ImagePullPolicy` |
| `~/cloud/edd-cloud-interface/services/compute/internal/k8s/pullpolicy.go` | **new** — `resolvePullPolicy` helper (unit-testable) |
| `~/cloud/edd-cloud-interface/services/compute/internal/api/handler.go` | start/stop routes → `start`/`stop` scope |
| `~/cloud/edd-cloud-interface/frontend/src/components/service-accounts/PermissionPicker.tsx` | start/stop actions; Registry group |
| `~/cloud/edd-cloud-interface/frontend/src/components/compute/CreateContainerForm.tsx` | pull-strategy `<select>` |
| `~/projects/resume_website/.github/workflows/build-push.yml` | replace GKE `deploy` job |

---

### Task 1: Auth — registry resource + start/stop actions

**Files:**
- Modify: `~/cloud/edd-cloud-auth/internal/api/tokens.go` (`validateScopes`, lines ~248-300; `validActions` at line 27-28)
- Create: `~/cloud/edd-cloud-auth/internal/api/tokens_test.go`

- [ ] **Step 1: Write the failing tests**

Create `~/cloud/edd-cloud-auth/internal/api/tokens_test.go`:

```go
package api

import "testing"

func TestValidateScopes_StartStop(t *testing.T) {
	err := validateScopes(map[string][]string{
		"compute.u1.containers.507b1b10": {"start", "stop"},
	}, "u1")
	if err != nil {
		t.Fatalf("start/stop on a container should be valid: %v", err)
	}
}

func TestValidateScopes_RegistryPushPullDelete(t *testing.T) {
	err := validateScopes(map[string][]string{
		"storage.u1.registry.eddison/resume": {"push", "pull", "delete"},
	}, "u1")
	if err != nil {
		t.Fatalf("registry push/pull/delete should be valid: %v", err)
	}
}

func TestValidateScopes_RejectsStartOnStorage(t *testing.T) {
	err := validateScopes(map[string][]string{
		"storage.u1.files": {"start"},
	}, "u1")
	if err == nil {
		t.Fatal("start is not a valid action for storage.files")
	}
}

func TestValidateScopes_RejectsPushOnContainers(t *testing.T) {
	err := validateScopes(map[string][]string{
		"compute.u1.containers": {"push"},
	}, "u1")
	if err == nil {
		t.Fatal("push is not a valid action for compute.containers")
	}
}

func TestValidateScopes_LegacyCRUDStillValid(t *testing.T) {
	err := validateScopes(map[string][]string{
		"compute.u1.containers":  {"create", "read", "update", "delete"},
		"storage.u1.namespaces":  {"create", "read", "update", "delete"},
		"storage.u1.files":       {"create", "read", "delete"},
		"compute.u1.keys":        {"create", "read", "delete"},
	}, "u1")
	if err != nil {
		t.Fatalf("existing CRUD scopes must remain valid: %v", err)
	}
}

func TestValidateScopes_RejectsCrossUser(t *testing.T) {
	err := validateScopes(map[string][]string{
		"compute.u2.containers": {"read"},
	}, "u1")
	if err == nil {
		t.Fatal("must reject scopes for another user's id")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd ~/cloud/edd-cloud-auth && go test ./internal/api/ -run TestValidateScopes`
Expected: FAIL — `start`/`stop`/`push`/`pull` rejected by the current flat `validActions`, and `registry` rejected as an invalid storage resource.

- [ ] **Step 3: Make actions resource-aware in `validateScopes`**

In `tokens.go`, the package-level vars (lines 26-29) currently are:

```go
var (
	validRoots   = map[string]bool{"compute": true, "storage": true}
	validActions = map[string]bool{"create": true, "read": true, "update": true, "delete": true}
)
```

Replace with (keep `validRoots`, keep `validActions` as the base set for root-level scopes, add per-resource action sets):

```go
var (
	validRoots   = map[string]bool{"compute": true, "storage": true}
	validActions = map[string]bool{"create": true, "read": true, "update": true, "delete": true}

	// validResourceActions defines the allowed actions per <root>.<resource>.
	// Falls back to validActions when a resource has no specific entry.
	validResourceActions = map[string]map[string]map[string]bool{
		"compute": {
			"containers": {"create": true, "read": true, "update": true, "delete": true, "start": true, "stop": true},
			"keys":       {"create": true, "read": true, "delete": true},
		},
		"storage": {
			"namespaces": {"create": true, "read": true, "update": true, "delete": true},
			"files":      {"create": true, "read": true, "delete": true},
			"registry":   {"push": true, "pull": true, "delete": true},
		},
	}
)
```

Then in `validateScopes`, update the resource and action validation blocks. Replace the existing `validResources` map and the resource/action checks with:

```go
func validateScopes(scopes map[string][]string, userID string) error {
	for scope, actions := range scopes {
		parts := strings.Split(scope, ".")
		if len(parts) < 2 || len(parts) > 4 {
			return fmt.Errorf("invalid scope path: %s (must be <root>.<userid>[.<resource>[.<id>]])", scope)
		}

		root := parts[0]
		if !validRoots[root] {
			return fmt.Errorf("invalid scope root: %s (must be compute or storage)", root)
		}

		scopeUserID := parts[1]
		if scopeUserID != userID {
			return fmt.Errorf("cannot create token for another user's resources")
		}

		// Determine the allowed action set for this scope.
		allowed := validActions
		if len(parts) >= 3 {
			resource := parts[2]
			resActions, ok := validResourceActions[root][resource]
			if !ok {
				return fmt.Errorf("invalid resource: %s for root %s", resource, root)
			}
			allowed = resActions
		}

		if len(parts) == 4 && parts[3] == "" {
			return fmt.Errorf("resource ID cannot be empty in scope %s", scope)
		}

		if len(actions) == 0 {
			return fmt.Errorf("at least one action required for scope %s", scope)
		}
		for _, action := range actions {
			if !allowed[action] {
				return fmt.Errorf("invalid action %q for scope %s", action, scope)
			}
		}
	}
	return nil
}
```

Note: registry repo names contain a `/` (e.g. `eddison/resume`), so `parts[3]` (the `.<id>` leaf) may itself contain a slash — that's fine, `strings.Split` on `.` keeps it intact, and the leaf is only checked for non-emptiness.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd ~/cloud/edd-cloud-auth && go test ./internal/api/ -run TestValidateScopes -v`
Expected: PASS (all six).

- [ ] **Step 5: Commit**

```bash
cd ~/cloud
git add edd-cloud-auth/internal/api/tokens.go edd-cloud-auth/internal/api/tokens_test.go
git commit -m "feat(auth): add start/stop container actions and registry resource to SA scopes"
```

---

### Task 2: Auth — registry delete maps to OCI delete

**Files:**
- Modify: `~/cloud/edd-cloud-auth/internal/api/registry_token.go` (`translateSAActionToOCI`)
- Create: `~/cloud/edd-cloud-auth/internal/api/registry_token_test.go`

- [ ] **Step 1: Write the failing test**

Create `~/cloud/edd-cloud-auth/internal/api/registry_token_test.go`:

```go
package api

import "testing"

func TestTranslateSAActionToOCI(t *testing.T) {
	cases := map[string]string{
		"pull":   "pull",
		"push":   "push",
		"read":   "pull", // legacy
		"create": "push", // legacy
		"update": "push", // legacy
		"delete": "delete",
	}
	for in, want := range cases {
		if got := translateSAActionToOCI(in); got != want {
			t.Errorf("translateSAActionToOCI(%q) = %q, want %q", in, got, want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd ~/cloud/edd-cloud-auth && go test ./internal/api/ -run TestTranslateSAActionToOCI`
Expected: FAIL — `delete` currently returns `push`; `push`/`pull` pass via default (those already pass), so the failing assertion is `delete`.

- [ ] **Step 3: Update `translateSAActionToOCI`**

Replace the function body in `registry_token.go`:

```go
func translateSAActionToOCI(saAction string) string {
	switch saAction {
	case "read":
		return "pull"
	case "create", "update":
		return "push"
	case "delete":
		return "delete"
	default:
		return saAction // push, pull pass through
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd ~/cloud/edd-cloud-auth && go test ./internal/api/ -run TestTranslateSAActionToOCI -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd ~/cloud
git add edd-cloud-auth/internal/api/registry_token.go edd-cloud-auth/internal/api/registry_token_test.go
git commit -m "feat(auth): map registry delete action to OCI delete"
```

---

### Task 3: Registry — honor OCI delete + manifest deletion

**Files:**
- Modify: `~/cloud/edd-cloud-interface/services/registry/auth.go` (`hasAccess`)
- Modify: registry manifest routing (`routeV2` / manifest handler in `services/registry/*.go`)
- Create: a test next to the manifest handler

- [ ] **Step 1: Locate the manifest handler and access check**

Run:
```bash
cd ~/cloud/edd-cloud-interface/services/registry
grep -rn "manifests\|hasAccess\|func.*[Mm]anifest\|DELETE" *.go | grep -v _test
sed -n '/func hasAccess/,/^}/p' auth.go
```
Read the manifest handler and `hasAccess`. Confirm pull→`pull`, push→`push` already map; the gap is a `delete` action and a `DELETE .../manifests/<ref>` path.

- [ ] **Step 2: Write the failing test for `hasAccess` delete**

Create `~/cloud/edd-cloud-interface/services/registry/auth_delete_test.go`:

```go
package main

import "testing"

func TestHasAccess_Delete(t *testing.T) {
	// A token granted the "delete" action on the repo may delete; pull-only may not.
	deleter := &authResult{access: []registryAccess{{Type: "repository", Name: "eddison/resume", Actions: []string{"delete"}}}}
	puller := &authResult{access: []registryAccess{{Type: "repository", Name: "eddison/resume", Actions: []string{"pull"}}}}

	if !hasAccess(deleter, "eddison/resume", "delete") {
		t.Error("delete token should be allowed to delete")
	}
	if hasAccess(puller, "eddison/resume", "delete") {
		t.Error("pull-only token must not be allowed to delete")
	}
}
```

Adjust the `authResult`/`registryAccess` literal field names to match the actual structs found in Step 1 if they differ; keep the assertions identical.

- [ ] **Step 3: Run test to verify it fails / compiles**

Run: `cd ~/cloud/edd-cloud-interface/services/registry && go test ./... -run TestHasAccess_Delete`
Expected: FAIL (delete not yet honored) or a compile error guiding struct field names — fix names, then it should FAIL on the assertion.

- [ ] **Step 4: Implement delete in `hasAccess` + manifest DELETE**

In `hasAccess`, ensure the requested `action` (`delete`) is matched against the token's granted actions for the repo (same matching used for `pull`/`push`). In the manifest router, add handling for `DELETE /v2/<name>/manifests/<reference>`: require `hasAccess(auth, name, "delete")`, then delete the manifest/tag from the blob store (use the existing blob/manifest store API discovered in Step 1). Return `202 Accepted` on success, `404` if absent, `401`/`403` per the existing auth challenge flow.

- [ ] **Step 5: Run test to verify it passes + full package build**

Run:
```bash
cd ~/cloud/edd-cloud-interface/services/registry
go test ./... -run TestHasAccess_Delete -v && go build ./...
```
Expected: PASS and clean build.

- [ ] **Step 6: Commit**

```bash
cd ~/cloud
git add edd-cloud-interface/services/registry
git commit -m "feat(registry): honor OCI delete action and support manifest deletion"
```

---

### Task 4: Compute — per-container image pull policy

**Files:**
- Create: `~/cloud/edd-cloud-interface/services/compute/internal/k8s/pullpolicy.go`
- Create: `~/cloud/edd-cloud-interface/services/compute/internal/k8s/pullpolicy_test.go`
- Modify: `internal/db/db.go` (migration), `internal/db/containers.go` (struct/INSERT/scan), `internal/api/containers.go` (request/validate/default), `internal/k8s/client.go` (`CreatePod` signature + use), and the two `CreatePod` call sites in `internal/api/containers.go`.

- [ ] **Step 1: Write the failing test for the pull-policy helper**

Create `~/cloud/edd-cloud-interface/services/compute/internal/k8s/pullpolicy_test.go`:

```go
package k8s

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestResolvePullPolicy(t *testing.T) {
	cases := map[string]corev1.PullPolicy{
		"Always":       corev1.PullAlways,
		"IfNotPresent": corev1.PullIfNotPresent,
		"":             corev1.PullIfNotPresent, // default
		"garbage":      corev1.PullIfNotPresent, // unknown → safe default
	}
	for in, want := range cases {
		if got := resolvePullPolicy(in); got != want {
			t.Errorf("resolvePullPolicy(%q) = %v, want %v", in, got, want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd ~/cloud/edd-cloud-interface/services/compute && go test ./internal/k8s/ -run TestResolvePullPolicy`
Expected: FAIL — `undefined: resolvePullPolicy`.

- [ ] **Step 3: Implement the helper**

Create `~/cloud/edd-cloud-interface/services/compute/internal/k8s/pullpolicy.go`:

```go
package k8s

import corev1 "k8s.io/api/core/v1"

// resolvePullPolicy maps a stored pull_policy string to a Kubernetes pull policy.
// Anything other than "Always" resolves to IfNotPresent (the safe default that
// preserves prior behavior).
func resolvePullPolicy(s string) corev1.PullPolicy {
	if s == "Always" {
		return corev1.PullAlways
	}
	return corev1.PullIfNotPresent
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd ~/cloud/edd-cloud-interface/services/compute && go test ./internal/k8s/ -run TestResolvePullPolicy -v`
Expected: PASS.

- [ ] **Step 5: Add `pull_policy` column migration**

In `internal/db/db.go`, the `migrate()` function has a `migrations := []string{...}` slice executed in order with `CREATE TABLE IF NOT EXISTS`. Append a new idempotent migration entry to that slice (after the `containers` table entry):

```go
		`ALTER TABLE containers ADD COLUMN IF NOT EXISTS pull_policy TEXT DEFAULT 'IfNotPresent'`,
```

- [ ] **Step 6: Persist and read `pull_policy`**

In `internal/db/containers.go`:

Add to the `Container` struct (after `Image`):
```go
	PullPolicy   string   // "Always" | "IfNotPresent"
```

Update the `CreateContainer` INSERT (add column + placeholder + arg):
```go
		INSERT INTO containers (id, user_id, owner_username, name, namespace, status, memory_mb, storage_gb, image, instance_type, ssh_enabled, mount_paths, pull_policy)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		c.ID, c.UserID, c.Owner, c.Name, c.Namespace, c.Status, c.MemoryMB, c.StorageGB, c.Image, c.InstanceType, c.SSHEnabled, string(mountPathsJSON), c.PullPolicy,
```

Update `GetContainer`'s SELECT and `Scan` to read it (this is the row `StartContainer` uses):
- Add `COALESCE(pull_policy, 'IfNotPresent')` to the SELECT column list (e.g. after the `image` column).
- Add `&c.PullPolicy` to the `.Scan(...)` args in the matching position.

(Other read sites — `ListContainersByUser`, `ListAllContainers` — don't need it; leaving `PullPolicy` empty there is harmless because `resolvePullPolicy("")` defaults safely and those paths don't recreate pods.)

- [ ] **Step 7: Thread `pull_policy` through `CreatePod`**

In `internal/k8s/client.go`, change `CreatePod`'s signature to accept the policy:
```go
func (c *Client) CreatePod(ctx context.Context, namespace string, image string, memoryMB int, arch string, cpuCores string, mountPaths []string, pullPolicy string) error {
```
In the `main` container spec (where `Image: image` is set), add:
```go
					Name:            "main",
					Image:           image,
					ImagePullPolicy: resolvePullPolicy(pullPolicy),
```

- [ ] **Step 8: Update both `CreatePod` call sites + request handling**

In `internal/api/containers.go`:

Add to the create request struct (after `Image`):
```go
	PullPolicy   string   `json:"pull_policy,omitempty"`
```

After the image-resolve block, validate/default the policy:
```go
	// Resolve pull policy (default IfNotPresent; only Always is the alternative)
	pullPolicy := "IfNotPresent"
	if req.PullPolicy == "Always" {
		pullPolicy = "Always"
	} else if req.PullPolicy != "" && req.PullPolicy != "IfNotPresent" {
		http.Error(w, "invalid pull_policy: must be 'Always' or 'IfNotPresent'", http.StatusBadRequest)
		return
	}
```

Set it on the `db.Container` literal:
```go
		Image:        image,
		PullPolicy:   pullPolicy,
```

Update the create-path `CreatePod` call (≈ line 288) and the `StartContainer` call (≈ line 551) to pass the policy:
```go
	// create path:
	h.k8s.CreatePod(ctx, container.Namespace, container.Image, container.MemoryMB, spec.Arch, spec.CPUCores, container.MountPaths, container.PullPolicy)
	// StartContainer:
	h.k8s.CreatePod(ctx, container.Namespace, container.Image, container.MemoryMB, spec.Arch, spec.CPUCores, container.MountPaths, container.PullPolicy)
```

- [ ] **Step 9: Build the whole compute service**

Run: `cd ~/cloud/edd-cloud-interface/services/compute && go build ./... && go test ./internal/k8s/ -v`
Expected: clean build (all `CreatePod` callers updated), pull-policy test PASS.

- [ ] **Step 10: Commit**

```bash
cd ~/cloud
git add edd-cloud-interface/services/compute
git commit -m "feat(compute): per-container image pull policy (default IfNotPresent)"
```

---

### Task 5: Compute — split start/stop out of the update scope

**Files:**
- Modify: `~/cloud/edd-cloud-interface/services/compute/internal/api/handler.go` (lines 47-48)

- [ ] **Step 1: Change the start/stop route scopes**

In `handler.go`, the two routes currently read:
```go
	h.mux.HandleFunc("POST /compute/containers/{id}/stop", h.authMiddleware(h.scopeCheckContainer("update", h.StopContainer)))
	h.mux.HandleFunc("POST /compute/containers/{id}/start", h.authMiddleware(h.scopeCheckContainer("update", h.StartContainer)))
```
Change the action argument:
```go
	h.mux.HandleFunc("POST /compute/containers/{id}/stop", h.authMiddleware(h.scopeCheckContainer("stop", h.StopContainer)))
	h.mux.HandleFunc("POST /compute/containers/{id}/start", h.authMiddleware(h.scopeCheckContainer("start", h.StartContainer)))
```

- [ ] **Step 2: Confirm the scope walk-up still admits broad/session grants**

Read `scopeCheckContainer` and the `requireScope`/`hasPermission` walk-up (handler.go:253 + the permission store). Verify that a session (full identity) and a broad `compute.<uid>.containers` grant that includes the `start`/`stop` actions both still pass — i.e., the change only *narrows* what a fine-grained token needs, it doesn't break the dashboard (which uses a session). If `requireScope` matches actions exactly per node, confirm a token granting `start`/`stop` at the `compute.<uid>.containers` level walks down to the specific-container check. Document the finding in the commit message.

- [ ] **Step 3: Build**

Run: `cd ~/cloud/edd-cloud-interface/services/compute && go build ./...`
Expected: clean build.

- [ ] **Step 4: Commit**

```bash
cd ~/cloud
git add edd-cloud-interface/services/compute/internal/api/handler.go
git commit -m "feat(compute): gate container start/stop behind dedicated start/stop scopes"
```

---

### Task 6: Frontend — picker actions/Registry group + pull-strategy select

**Files:**
- Modify: `~/cloud/edd-cloud-interface/frontend/src/components/service-accounts/PermissionPicker.tsx`
- Modify: `~/cloud/edd-cloud-interface/frontend/src/components/compute/CreateContainerForm.tsx`

No frontend test runner exists; verify with typecheck/build and a manual UI check.

- [ ] **Step 1: Add start/stop to container actions**

In `PermissionPicker.tsx`, update the action constant:
```ts
export const CONTAINER_ACTIONS: string[] = ["create", "read", "update", "delete", "start", "stop"];
```
This renders start/stop pills on both the **All Containers** (`compute.<uid>.containers`) and **Specific Containers** (`compute.<uid>.containers.<id>`) rows, which both already iterate `CONTAINER_ACTIONS`.

- [ ] **Step 2: Add a Registry group under Storage**

In `PermissionPicker.tsx`, add a registry action constant near the others:
```ts
export const REGISTRY_ACTIONS: string[] = ["push", "pull", "delete"];
```
Add a broad registry scope key alongside the existing broad keys:
```ts
  const broadRegistryKey = `storage.${userId}.registry`;
```
In the Storage `<div>` section (after the All Files block), render a Registry row using the same `ScopeRow`/action-pill pattern the other resources use, bound to `broadRegistryKey` and `REGISTRY_ACTIONS`. Match the existing markup for "ALL FILES"/"ALL NAMESPACES" exactly, substituting label `REGISTRY`, key `broadRegistryKey`, actions `REGISTRY_ACTIONS`.

- [ ] **Step 3: Add the pull-strategy select to the create form**

In `CreateContainerForm.tsx`:
Add state (near the other `useState` calls, ~line 36):
```ts
  const [pullPolicy, setPullPolicy] = useState<string>("IfNotPresent");
```
Add the request field in the submit body (where `image: selectedImage || undefined` is set, ~line 66):
```ts
      pull_policy: pullPolicy,
```
Add a labeled `<select>` near the Image field (mirror the existing Image control's markup), with options:
```tsx
      <Label htmlFor="c-pull">Image pull strategy</Label>
      <select id="c-pull" value={pullPolicy} onChange={(e) => setPullPolicy(e.target.value)}>
        <option value="IfNotPresent">If Not Present (default)</option>
        <option value="Always">Always (re-pull on restart)</option>
      </select>
```
Use the project's existing Select component if the form uses one (match the Image field's component); otherwise a native `<select>` is fine.

- [ ] **Step 4: Typecheck / build**

Run:
```bash
cd ~/cloud/edd-cloud-interface/frontend && npm run build
```
Expected: build succeeds (TypeScript compiles). If the repo exposes `npm run typecheck` or `tsc --noEmit`, run that too.

- [ ] **Step 5: Manual UI check (note in commit, don't block on infra)**

If a dev server is available (`npm run dev`), open the Service Accounts create dialog and confirm: container rows show `start`/`stop` pills; a `REGISTRY` row appears under Storage with `push`/`pull`/`delete`; the create-container form shows the pull-strategy select. If no dev server is reachable in this environment, state that explicitly and rely on the successful build.

- [ ] **Step 6: Commit**

```bash
cd ~/cloud
git add edd-cloud-interface/frontend/src/components/service-accounts/PermissionPicker.tsx \
        edd-cloud-interface/frontend/src/components/compute/CreateContainerForm.tsx
git commit -m "feat(frontend): start/stop + registry scopes in picker, pull-strategy in create form"
```

---

### Task 7: Resume CI — push to edd-registry + redeploy via compute API

**Files:**
- Modify: `~/projects/resume_website/.github/workflows/build-push.yml`

- [ ] **Step 1: Branch the resume repo**

```bash
cd ~/projects/resume_website
git checkout main && git pull
git checkout -b feat/edd-cloud-cd
```

- [ ] **Step 2: Replace the registry target and deploy job**

In `.github/workflows/build-push.yml`:

(a) In `build-push-website`, change the image build/push to target the edd-cloud registry instead of Docker Hub. Add a `docker login` step and update tags:
```yaml
      - name: Login to edd-cloud registry
        run: echo "${{ secrets.ECLOUD_TOKEN }}" | docker login registry.cloud.eddisonso.com -u eddison --password-stdin

      - name: Build and Push web-service
        uses: docker/build-push-action@v5
        with:
          platforms: linux/amd64
          push: true
          tags: registry.cloud.eddisonso.com/eddison/resume:amd64
```

(b) Replace the entire GKE `deploy` job with a compute-API redeploy:
```yaml
  deploy:
    runs-on: ubuntu-latest
    needs: [build-push-website]
    steps:
      - name: Redeploy resume container (stop + start re-pulls :amd64)
        env:
          TOKEN: ${{ secrets.ECLOUD_TOKEN }}
          CID: "507b1b10"
          API: "https://cloud-api.eddisonso.com/compute/containers"
        run: |
          set -euo pipefail
          curl -fsS -X POST "$API/$CID/stop"  -H "Authorization: Bearer $TOKEN"
          # wait for the pod to be torn down, then start (re-pulls because pull_policy=Always)
          sleep 5
          curl -fsS -X POST "$API/$CID/start" -H "Authorization: Bearer $TOKEN"
```
Remove the now-unused `GKE_CLUSTER`/`GKE_ZONE`/`TIMESTAMP` env and the `setup` job's timestamp plumbing only if nothing else references them; otherwise leave `setup` intact and just drop the GKE steps. Keep the `test` job and its `needs` gating.

- [ ] **Step 3: Validate the workflow YAML**

Run:
```bash
cd ~/projects/resume_website
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/build-push.yml')); print('workflow YAML valid')"
```
Expected: `workflow YAML valid`.

- [ ] **Step 4: Commit**

```bash
cd ~/projects/resume_website
git add .github/workflows/build-push.yml
git commit -m "ci: push resume image to edd-cloud registry and redeploy via compute API"
```

---

### Task 8: Operator rollout checklist (run after code is merged & deployed)

This task is manual/operational — no code. Execute in order and verify each.

- [ ] **Step 1: Deploy the updated services.** Build and push new images for `edd-cloud-auth`, `services/registry`, `services/compute`, and the `frontend`, then roll them out (`kubectl rollout restart deploy/<name> -n core` per the existing per-service deploy process). The compute service runs its `pull_policy` migration on startup. Verify each rollout is healthy.

- [ ] **Step 2: Mint the CD service-account token.** In the dashboard → Service Accounts → create, name it `resume-cd`, grant exactly:
  - Storage → Registry (specific repo if the UI supports it, else broad): `push`
  - Compute → Specific Containers → `507b1b10` (RESUME): `start`, `stop`
  Copy the `ecloud_...` token.

- [ ] **Step 3: Store the secret.** Add it to the resume repo:
  ```bash
  gh secret set ECLOUD_TOKEN --repo EddisonSo/resume-website
  ```
  (paste the token when prompted).

- [ ] **Step 4: Reconfigure the resume container to `Always` pull.** The running container `507b1b10` was created with `IfNotPresent`. Recreate its pod once with `Always` so future stop/start re-pulls. Either: (a) if a future "update pull policy" path exists, use it; or (b) delete + recreate the container via the API with `pull_policy: Always`, `image: registry.cloud.eddisonso.com/eddison/resume:amd64` — **note** recreation changes the namespace/LB IP and requires re-pointing `eddisonso.com` and re-adding ingress/SSH, so prefer (a). If neither is acceptable, fall back to the Track-1 spec's alternative (explicit force-repull) — out of scope for this plan. Confirm with the operator before recreating.

- [ ] **Step 5: End-to-end verify.** Push the resume `feat/edd-cloud-cd` branch (open a PR / merge to main per the resume repo's flow). Watch the workflow: `test` → `build-push-website` (pushes to `registry.cloud.eddisonso.com/eddison/resume:amd64`) → `deploy` (stop/start). Then confirm the live site updated:
  ```bash
  curl -fsS https://eddisonso.com | grep -c "Eddison So"
  ```
  and that the served content reflects the new build. Confirm the SA token cannot do more than intended (e.g. a `terminal`/`delete container` call returns 403).

---

## Self-review notes

- **Spec coverage:** B → Task 1 (+5 for enforcement). D (registry resource) → Task 1; D (delete) → Tasks 2–3. Pull strategy → Task 4 (+ dropdown in Task 6). E → Task 6. Resume CI → Task 7. Rollout/one-time → Task 8. Networking (A) and gap C are explicitly out (Track 2 / deferred).
- **Type consistency:** `CreatePod(..., pullPolicy string)` defined in Task 4 Step 7 and called with `container.PullPolicy` in Step 8; `resolvePullPolicy` defined in Step 3 and used in Step 7; `pull_policy` JSON field, DB column, and struct field (`PullPolicy`) consistent across Tasks 4. `CONTAINER_ACTIONS`/`REGISTRY_ACTIONS` names consistent in Task 6. Scope action vocab (`start`,`stop`,`push`,`pull`,`delete`) consistent between auth validation (Task 1) and compute route checks (Task 5).
- **Verification honesty:** Go logic is TDD; frontend and route-wiring (no test harness) use build/typecheck + manual checks, called out explicitly rather than faked.
