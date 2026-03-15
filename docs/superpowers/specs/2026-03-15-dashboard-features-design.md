# Dashboard Features Design Spec — DropdownSelect, Registry Page, Container Logs

**Date:** 2026-03-15
**Status:** Draft

## Overview

Three independent features for the edd-cloud dashboard:

1. **DropdownSelect component** — custom themed dropdown replacing native `<select>` elements, with search, grouping, and keyboard navigation
2. **Registry images page** — browse public and private repos/tags under the Storage nav section
3. **Container logs** — real-time stdout/stderr streaming in the compute container detail view

## Feature 1: DropdownSelect Component

### Component API

```tsx
<DropdownSelect
  value={selected}
  onChange={setSelected}
  placeholder="Select instance type..."
  searchable
  groups={[
    { label: "ARM64", options: [
      { value: "nano", label: "Nano (ARM64, 0.5 CPU)" },
      { value: "micro", label: "Micro (ARM64, 1 CPU)" },
    ]},
    { label: "AMD64", options: [
      { value: "tiny", label: "Tiny (AMD64, 1 CPU)" },
    ]},
  ]}
/>

// Or flat options (no groups):
<DropdownSelect
  value={selected}
  onChange={setSelected}
  options={[
    { value: "v1", label: "test/echo:v1" },
  ]}
/>
```

### Features

- Dark theme matching existing UI (Catppuccin-style backgrounds, borders, text colors)
- Search/filter input (optional, enabled via `searchable` prop)
- Option groups with label dividers
- Keyboard navigation: arrow keys to move, Enter to select, Escape to close
- Click outside to close
- Portal rendering at document body root to avoid parent overflow clipping
- Same `value`/`onChange` API as the current `<Select>` for drop-in replacement

### File

`edd-cloud-interface/frontend/src/components/ui/dropdown-select.tsx`

### Migration

Replace native `<Select>` in:
- `CreateContainerForm.tsx` — instance type selector, image selector
- Any other `<Select>` usage across the frontend

## Feature 2: Registry Images Page

### Backend — Session-Auth API on Registry Service

New endpoints on the registry service (`edd-cloud-registry`) under `/api/` prefix, alongside the existing OCI `/v2/` endpoints. These accept session JWTs (already supported by the registry's `authenticate()` function).

**Routing:** Since repo names contain slashes (e.g., `test/echo`), Go's `http.ServeMux` patterns cannot match `{name}` across path segments. Use the same approach as the OCI endpoints: register a single `mux.HandleFunc("/api/", s.routeAPI)` catch-all and parse paths manually with `strings.Index`/`strings.LastIndex`.

**CORS:** The registry service needs CORS headers for `/api/` endpoints since the frontend at `cloud.eddisonso.com` calls them. Add CORS middleware (same pattern as compute service's `main.go` lines 32-37) wrapping the `/api/` handler.

**Owner authorization:** For mutating endpoints (`PUT /api/repos/{name}/visibility`, `DELETE /api/repos/{name}/tags/{tag}`), the handler must explicitly check `auth.UserID == repo.OwnerID`. Session tokens have `IsSession: true` which bypasses `hasAccess()` checks, so ownership must be enforced at the handler level.

**Endpoints:**

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/repos` | session | List repos (own + public for authenticated, public-only for anonymous) |
| GET | `/api/repos/\{name\}` | session | Repo detail: visibility, owner, tag count, total size |
| GET | `/api/repos/\{name\}/tags` | session | List tags with digests, sizes, push dates |
| PUT | `/api/repos/\{name\}/visibility` | session (owner) | Toggle public/private |
| DELETE | `/api/repos/\{name\}/tags/\{tag\}` | session (owner) | Delete a tag (delegates to existing `cleanupManifest` logic) |

**Response formats:**

`GET /api/repos`:
```json
{
  "repositories": [
    {
      "name": "test/echo",
      "visibility": 1,
      "tag_count": 2,
      "total_size": 50331648,
      "last_pushed": "2026-03-15T17:00:00Z"
    }
  ]
}
```

`GET /api/repos/{name}/tags` (note: `pushed_at` maps to `tags.updated_at`, `last_pushed` on repo list is `MAX(tags.updated_at)` — no schema changes needed):
```json
{
  "name": "test/echo",
  "tags": [
    {
      "name": "v1",
      "digest": "sha256:611fec88...",
      "size": 25165824,
      "pushed_at": "2026-03-15T17:00:00Z"
    }
  ]
}
```

### Compute Service Update

`ListImages` updated to call `http://edd-registry:80/api/repos` (with forwarded user token) instead of the current `/v2/_catalog` + `/v2/{name}/tags/list` double call. Single request, cleaner.

### Frontend — Navigation

New sub-item under Storage in the sidebar:

```
Storage
├── Files        (existing, /storage)
└── Registry     (new, /storage/registry)
```

Update `NAV_ITEMS` in `constants.ts` to add the sub-item.

### Frontend — Pages

**Registry repo list** (`/storage/registry`):
- Two tabs: "My Repositories" and "Public"
- Each row: repo name, tag count, total size (formatted), last pushed (relative time)
- Click row → navigate to repo detail
- Uses the new DropdownSelect for any filters

**Registry repo detail** (`/storage/registry/{name}`):
- Breadcrumb: Storage > Registry > {repo name}
- Repo metadata: visibility badge, owner, total size
- Tag list table: tag name, truncated digest (copyable), size, pushed date
- Actions per tag: copy `docker pull registry.cloud.eddisonso.com/{name}:{tag}` command, delete tag (owner only)
- Repo actions: toggle visibility (public/private)

### Frontend — Files

- `edd-cloud-interface/frontend/src/pages/RegistryPage.tsx` — page component with list/detail views
- `edd-cloud-interface/frontend/src/components/registry/RepoList.tsx` — repo list with tabs
- `edd-cloud-interface/frontend/src/components/registry/RepoDetail.tsx` — tag list and actions
- `edd-cloud-interface/frontend/src/hooks/useRegistry.ts` — data fetching hook

### Routing

```tsx
// In App.tsx — single wildcard route since repo names contain slashes
<Route path="/storage/registry/*" element={<RegistryPage />} />
```

The `RegistryPage` component uses `useLocation()` to parse the path and determine whether to show the list view (bare `/storage/registry`) or detail view (e.g., `/storage/registry/test/echo`).

## Feature 3: Container Logs

### Backend — WebSocket Log Streaming

New WebSocket endpoint on the compute service:

`GET /compute/containers/{id}/logs?token={auth_token}&tail=100`

**Implementation:**
1. Authenticate via `token` query param (same pattern as terminal WebSocket)
2. Look up container → get pod name and namespace
3. Call `clientset.CoreV1().Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{Follow: true, TailLines: &tail, Timestamps: true}).Stream()`
4. Read log lines from the stream and write to WebSocket
5. Close WebSocket when stream ends or client disconnects

**Route registration** in `handler.go` — must include auth middleware and scope check (read, not update, since logs are read-only):
```go
mux.HandleFunc("GET /compute/containers/{id}/logs", h.authMiddleware(h.scopeCheckContainer("read", h.HandleContainerLogs)))
```

**Log format:** Raw text lines with timestamps (from K8s `Timestamps: true`):
```
2026-03-15T17:00:01.123Z Server started on :8080
2026-03-15T17:00:03.456Z Connected to database
```

### Frontend — Logs Tab in Container Detail

Add a **Logs** tab alongside the existing **Terminal** button in the container detail view.

**Hook:** `useContainerLogs.ts`
- WebSocket connection to `/compute/containers/{id}/logs`
- Buffer + flush pattern (same as existing `useLogs.ts`)
- Auto-reconnect on disconnect
- Max 2000 lines buffer (drop oldest — conservative to handle long lines like stack traces)
- Pause streaming when user scrolls up

**Component:** `ContainerLogs.tsx`
- Monospace log display (JetBrains Mono, same as terminal)
- Dark background (#0d0d14, matching terminal)
- Toolbar: streaming indicator (green dot), Clear button, Download button
- Auto-scroll to bottom (pauses when user scrolls up, resumes on "Jump to bottom" click)
- Timestamps in muted color, log content in normal color

### Frontend — Files

- `edd-cloud-interface/frontend/src/components/compute/ContainerLogs.tsx`
- `edd-cloud-interface/frontend/src/hooks/useContainerLogs.ts`

### Container Detail Integration

In `ContainerDetail.tsx`, add tab switching between info/logs/terminal views. The logs tab streams continuously while visible and disconnects when switching away.

## Implementation Order

1. **DropdownSelect component** — no dependencies, used by the other features
2. **Registry session-auth API** — backend endpoints on registry service
3. **Registry images page** — frontend consuming the new API
4. **Container logs backend** — WebSocket endpoint on compute service
5. **Container logs frontend** — logs tab in container detail
6. **Compute ListImages update** — switch to `/api/repos` endpoint
7. **Migrate existing selects** — replace native `<Select>` usages with DropdownSelect
