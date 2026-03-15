# Dashboard Features Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a custom DropdownSelect component, a registry images browsing page under Storage, and real-time container log streaming in compute.

**Architecture:** DropdownSelect is a standalone UI component replacing native `<select>`. Registry page adds session-auth `/api/` endpoints to the existing registry service, consumed by a new React page under Storage. Container logs adds a WebSocket endpoint to the compute service using K8s `GetLogs` API, with a new logs component in the container detail view.

**Tech Stack:** React 18, TypeScript, Tailwind CSS, Go 1.24, K8s client-go, WebSocket

**Spec:** `docs/superpowers/specs/2026-03-15-dashboard-features-design.md`

---

## Chunk 1: DropdownSelect Component

### Task 1: Create DropdownSelect component

**Files:**
- Create: `edd-cloud-interface/frontend/src/components/ui/dropdown-select.tsx`

- [ ] **Step 1: Create the component**

```tsx
import React, { useState, useRef, useEffect, useCallback, useMemo } from "react";
import { createPortal } from "react-dom";
import { cn } from "@/lib/utils";
import { ChevronDown, Search, Check } from "lucide-react";

export interface DropdownOption {
  value: string;
  label: string;
  icon?: React.ReactNode;
}

export interface DropdownGroup {
  label: string;
  options: DropdownOption[];
}

interface DropdownSelectProps {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  searchable?: boolean;
  options?: DropdownOption[];
  groups?: DropdownGroup[];
  className?: string;
  disabled?: boolean;
}

export function DropdownSelect({
  value,
  onChange,
  placeholder = "Select...",
  searchable = false,
  options = [],
  groups = [],
  className,
  disabled = false,
}: DropdownSelectProps) {
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState("");
  const [highlightIndex, setHighlightIndex] = useState(-1);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const dropdownRef = useRef<HTMLDivElement>(null);
  const searchRef = useRef<HTMLInputElement>(null);
  const [dropdownStyle, setDropdownStyle] = useState<React.CSSProperties>({});

  // Flatten all options for keyboard nav and search
  const allOptions = useMemo(() => {
    if (groups.length > 0) {
      return groups.flatMap((g) => g.options);
    }
    return options;
  }, [options, groups]);

  // Filter by search
  const filteredGroups = useMemo(() => {
    const q = search.toLowerCase();
    if (groups.length > 0) {
      return groups
        .map((g) => ({
          ...g,
          options: g.options.filter((o) => o.label.toLowerCase().includes(q)),
        }))
        .filter((g) => g.options.length > 0);
    }
    return [{ label: "", options: options.filter((o) => o.label.toLowerCase().includes(q)) }];
  }, [search, options, groups]);

  const filteredFlat = useMemo(
    () => filteredGroups.flatMap((g) => g.options),
    [filteredGroups]
  );

  // Selected label
  const selectedLabel = allOptions.find((o) => o.value === value)?.label;

  // Position dropdown below trigger
  const updatePosition = useCallback(() => {
    if (!triggerRef.current) return;
    const rect = triggerRef.current.getBoundingClientRect();
    setDropdownStyle({
      position: "fixed",
      top: rect.bottom + 4,
      left: rect.left,
      width: rect.width,
      zIndex: 9999,
    });
  }, []);

  // Open/close
  const toggle = () => {
    if (disabled) return;
    if (!open) {
      updatePosition();
      setOpen(true);
      setSearch("");
      setHighlightIndex(-1);
    } else {
      setOpen(false);
    }
  };

  // Focus search on open
  useEffect(() => {
    if (open && searchable && searchRef.current) {
      searchRef.current.focus();
    }
  }, [open, searchable]);

  // Click outside
  useEffect(() => {
    if (!open) return;
    const handler = (e: MouseEvent) => {
      if (
        triggerRef.current?.contains(e.target as Node) ||
        dropdownRef.current?.contains(e.target as Node)
      )
        return;
      setOpen(false);
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, [open]);

  // Keyboard nav
  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (!open) {
      if (e.key === "ArrowDown" || e.key === "Enter" || e.key === " ") {
        e.preventDefault();
        toggle();
      }
      return;
    }
    switch (e.key) {
      case "ArrowDown":
        e.preventDefault();
        setHighlightIndex((i) => Math.min(i + 1, filteredFlat.length - 1));
        break;
      case "ArrowUp":
        e.preventDefault();
        setHighlightIndex((i) => Math.max(i - 1, 0));
        break;
      case "Enter":
        e.preventDefault();
        if (highlightIndex >= 0 && highlightIndex < filteredFlat.length) {
          onChange(filteredFlat[highlightIndex].value);
          setOpen(false);
        }
        break;
      case "Escape":
        e.preventDefault();
        setOpen(false);
        triggerRef.current?.focus();
        break;
    }
  };

  const select = (val: string) => {
    onChange(val);
    setOpen(false);
    triggerRef.current?.focus();
  };

  return (
    <>
      <button
        ref={triggerRef}
        type="button"
        onClick={toggle}
        onKeyDown={handleKeyDown}
        disabled={disabled}
        className={cn(
          "flex h-9 w-full items-center justify-between rounded-md border border-input bg-background px-3 py-1 text-sm shadow-sm transition-colors",
          "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
          "disabled:cursor-not-allowed disabled:opacity-50",
          !selectedLabel && "text-muted-foreground",
          className
        )}
      >
        <span className="truncate">{selectedLabel || placeholder}</span>
        <ChevronDown className="h-4 w-4 shrink-0 opacity-50" />
      </button>

      {open &&
        createPortal(
          <div
            ref={dropdownRef}
            style={dropdownStyle}
            onKeyDown={handleKeyDown}
            className="rounded-md border border-border bg-popover text-popover-foreground shadow-md animate-in fade-in-0 zoom-in-95"
          >
            {searchable && (
              <div className="flex items-center border-b border-border px-3 py-2">
                <Search className="mr-2 h-4 w-4 shrink-0 opacity-50" />
                <input
                  ref={searchRef}
                  value={search}
                  onChange={(e) => {
                    setSearch(e.target.value);
                    setHighlightIndex(0);
                  }}
                  placeholder="Search..."
                  className="flex h-6 w-full bg-transparent text-sm outline-none placeholder:text-muted-foreground"
                />
              </div>
            )}
            <div className="max-h-60 overflow-y-auto p-1">
              {filteredFlat.length === 0 && (
                <div className="py-6 text-center text-sm text-muted-foreground">
                  No results found.
                </div>
              )}
              {filteredGroups.map((group, gi) => (
                <div key={gi}>
                  {group.label && (
                    <div className="px-2 py-1.5 text-xs font-medium text-muted-foreground">
                      {group.label}
                    </div>
                  )}
                  {group.options.map((option) => {
                    const flatIdx = filteredFlat.indexOf(option);
                    return (
                      <div
                        key={option.value}
                        onClick={() => select(option.value)}
                        className={cn(
                          "relative flex cursor-pointer select-none items-center rounded-sm px-2 py-1.5 text-sm outline-none",
                          flatIdx === highlightIndex && "bg-accent text-accent-foreground",
                          option.value === value && "font-medium"
                        )}
                      >
                        {option.icon && <span className="mr-2">{option.icon}</span>}
                        <span className="flex-1">{option.label}</span>
                        {option.value === value && (
                          <Check className="h-4 w-4 shrink-0 text-primary" />
                        )}
                      </div>
                    );
                  })}
                </div>
              ))}
            </div>
          </div>,
          document.body
        )}
    </>
  );
}
```

- [ ] **Step 2: Verify it compiles**

```bash
cd edd-cloud-interface/frontend && npx tsc --noEmit
```

- [ ] **Step 3: Commit**

```bash
git add edd-cloud-interface/frontend/src/components/ui/dropdown-select.tsx
git commit -m "feat(ui): add DropdownSelect component with search, groups, and keyboard nav"
```

### Task 2: Replace native Select in CreateContainerForm

**Files:**
- Modify: `edd-cloud-interface/frontend/src/components/compute/CreateContainerForm.tsx`

- [ ] **Step 1: Replace instance type and image selects**

Read `CreateContainerForm.tsx` first. Replace the two `<Select>` usages:

**Instance type** — change from `<Select>` to `<DropdownSelect>` with groups:
```tsx
import { DropdownSelect } from "@/components/ui/dropdown-select";

// Replace the instance type <Select> with:
<DropdownSelect
  value={instanceType}
  onChange={(val) => setInstanceType(val)}
  groups={[
    { label: "ARM64", options: [
      { value: "nano", label: "Nano (ARM64, 0.5 CPU)" },
      { value: "micro", label: "Micro (ARM64, 1 CPU)" },
      { value: "mini", label: "Mini (ARM64, 2 CPU)" },
    ]},
    { label: "AMD64", options: [
      { value: "tiny", label: "Tiny (AMD64, 1 CPU)" },
      { value: "small", label: "Small (AMD64, 2 CPU)" },
      { value: "medium", label: "Medium (AMD64, 4 CPU)" },
    ]},
  ]}
/>

// Replace the image <Select> with:
<DropdownSelect
  value={selectedImage}
  onChange={(val) => setSelectedImage(val)}
  placeholder="Default (Debian Base)"
  searchable
  options={[
    { value: "", label: "Default (Debian Base)" },
    ...images.filter((i) => i.source === "registry").map((img) => ({
      value: img.image,
      label: img.name,
    })),
  ]}
/>
```

- [ ] **Step 2: Verify**

```bash
cd edd-cloud-interface/frontend && npx tsc --noEmit
```

- [ ] **Step 3: Commit**

```bash
git add edd-cloud-interface/frontend/src/components/compute/CreateContainerForm.tsx
git commit -m "feat(compute): replace native selects with DropdownSelect in container form"
```

---

## Chunk 2: Registry Session-Auth API

### Task 3: Add /api/ route handler and CORS to registry service

**Files:**
- Create: `edd-cloud-interface/services/registry/api.go`
- Modify: `edd-cloud-interface/services/registry/main.go` (add route + CORS)

- [ ] **Step 1: Create `api.go`**

Read `main.go`, `auth.go`, `db.go`, and `manifests.go` first to understand existing patterns.

Create `edd-cloud-interface/services/registry/api.go` with:

```go
package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// routeAPI dispatches /api/ requests. Uses manual path parsing because repo
// names contain slashes (same approach as routeV2).
func (s *server) routeAPI(w http.ResponseWriter, r *http.Request) {
	// CORS
	origin := r.Header.Get("Origin")
	if origin != "" && (strings.HasSuffix(origin, ".cloud.eddisonso.com") || origin == "https://cloud.eddisonso.com") {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, PUT, DELETE, OPTIONS")
	}
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api")

	// GET /api/repos
	if path == "/repos" && r.Method == http.MethodGet {
		s.apiListRepos(w, r)
		return
	}

	// Routes with repo name: /api/repos/{name}...
	if strings.HasPrefix(path, "/repos/") {
		rest := strings.TrimPrefix(path, "/repos/")

		// Check for /tags suffix
		if idx := strings.LastIndex(rest, "/tags/"); idx >= 0 {
			repoName := rest[:idx]
			tag := rest[idx+6:]
			if r.Method == http.MethodDelete && tag != "" {
				s.apiDeleteTag(w, r, repoName, tag)
				return
			}
		}

		if strings.HasSuffix(rest, "/tags") {
			repoName := strings.TrimSuffix(rest, "/tags")
			if r.Method == http.MethodGet {
				s.apiListTags(w, r, repoName)
				return
			}
		}

		if strings.HasSuffix(rest, "/visibility") {
			repoName := strings.TrimSuffix(rest, "/visibility")
			if r.Method == http.MethodPut {
				s.apiSetVisibility(w, r, repoName)
				return
			}
		}

		// GET /api/repos/{name} (repo detail)
		if r.Method == http.MethodGet {
			s.apiGetRepo(w, r, rest)
			return
		}
	}

	http.NotFound(w, r)
}

type apiRepo struct {
	Name       string    `json:"name"`
	Visibility int       `json:"visibility"`
	OwnerID    string    `json:"owner_id"`
	TagCount   int       `json:"tag_count"`
	TotalSize  int64     `json:"total_size"`
	LastPushed time.Time `json:"last_pushed"`
}

func (s *server) apiListRepos(w http.ResponseWriter, r *http.Request) {
	auth := s.authenticate(r)

	var rows *sql.Rows
	var err error

	if auth != nil && auth.UserID != "" {
		rows, err = s.db.QueryContext(r.Context(), `
			SELECT r.name, r.visibility, r.owner_id,
				COALESCE((SELECT COUNT(*) FROM tags WHERE repository_id = r.id), 0) as tag_count,
				COALESCE((SELECT SUM(rb.size) FROM repository_blobs rb WHERE rb.repository_id = r.id), 0) as total_size,
				COALESCE((SELECT MAX(t.updated_at) FROM tags t WHERE t.repository_id = r.id), r.created_at) as last_pushed
			FROM repositories r
			WHERE r.owner_id = $1 OR r.visibility > 0
			ORDER BY r.name`, auth.UserID)
	} else {
		rows, err = s.db.QueryContext(r.Context(), `
			SELECT r.name, r.visibility, r.owner_id,
				COALESCE((SELECT COUNT(*) FROM tags WHERE repository_id = r.id), 0) as tag_count,
				COALESCE((SELECT SUM(rb.size) FROM repository_blobs rb WHERE rb.repository_id = r.id), 0) as total_size,
				COALESCE((SELECT MAX(t.updated_at) FROM tags t WHERE t.repository_id = r.id), r.created_at) as last_pushed
			FROM repositories r
			WHERE r.visibility > 0
			ORDER BY r.name`)
	}
	if err != nil {
		slog.Error("apiListRepos: query", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var repos []apiRepo
	for rows.Next() {
		var repo apiRepo
		if err := rows.Scan(&repo.Name, &repo.Visibility, &repo.OwnerID, &repo.TagCount, &repo.TotalSize, &repo.LastPushed); err != nil {
			slog.Error("apiListRepos: scan", "err", err)
			continue
		}
		repos = append(repos, repo)
	}
	if repos == nil {
		repos = []apiRepo{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"repositories": repos})
}

func (s *server) apiGetRepo(w http.ResponseWriter, r *http.Request, repoName string) {
	auth := s.authenticate(r)

	repoID, ownerID, visibility, err := getRepoByName(r.Context(), s.db, repoName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Private repos: require auth + ownership
	if visibility == 0 && (auth == nil || auth.UserID != ownerID) {
		http.NotFound(w, r)
		return
	}

	var tagCount int
	var totalSize int64
	var lastPushed time.Time
	s.db.QueryRowContext(r.Context(), `
		SELECT
			COALESCE((SELECT COUNT(*) FROM tags WHERE repository_id = $1), 0),
			COALESCE((SELECT SUM(rb.size) FROM repository_blobs rb WHERE rb.repository_id = $1), 0),
			COALESCE((SELECT MAX(t.updated_at) FROM tags t WHERE t.repository_id = $1), NOW())
	`, repoID).Scan(&tagCount, &totalSize, &lastPushed)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(apiRepo{
		Name:       repoName,
		Visibility: visibility,
		OwnerID:    ownerID,
		TagCount:   tagCount,
		TotalSize:  totalSize,
		LastPushed: lastPushed,
	})
}

type apiTag struct {
	Name     string    `json:"name"`
	Digest   string    `json:"digest"`
	Size     int64     `json:"size"`
	PushedAt time.Time `json:"pushed_at"`
}

func (s *server) apiListTags(w http.ResponseWriter, r *http.Request, repoName string) {
	auth := s.authenticate(r)

	repoID, ownerID, visibility, err := getRepoByName(r.Context(), s.db, repoName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if visibility == 0 && (auth == nil || auth.UserID != ownerID) {
		http.NotFound(w, r)
		return
	}

	rows, err := s.db.QueryContext(r.Context(), `
		SELECT t.name, t.manifest_digest,
			COALESCE(m.size, 0),
			t.updated_at
		FROM tags t
		LEFT JOIN manifests m ON m.repository_id = t.repository_id AND m.digest = t.manifest_digest
		WHERE t.repository_id = $1
		ORDER BY t.updated_at DESC`, repoID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var tags []apiTag
	for rows.Next() {
		var tag apiTag
		if err := rows.Scan(&tag.Name, &tag.Digest, &tag.Size, &tag.PushedAt); err != nil {
			continue
		}
		tags = append(tags, tag)
	}
	if tags == nil {
		tags = []apiTag{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"name": repoName, "tags": tags})
}

func (s *server) apiSetVisibility(w http.ResponseWriter, r *http.Request, repoName string) {
	auth := s.authenticate(r)
	if auth == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	_, ownerID, _, err := getRepoByName(r.Context(), s.db, repoName)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if auth.UserID != ownerID {
		http.Error(w, "forbidden: not repo owner", http.StatusForbidden)
		return
	}

	var body struct {
		Visibility int `json:"visibility"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	_, err = s.db.ExecContext(r.Context(),
		`UPDATE repositories SET visibility = $1, updated_at = NOW() WHERE name = $2`,
		body.Visibility, repoName)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *server) apiDeleteTag(w http.ResponseWriter, r *http.Request, repoName, tag string) {
	auth := s.authenticate(r)
	if auth == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	repoID, ownerID, _, err := getRepoByName(r.Context(), s.db, repoName)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if auth.UserID != ownerID {
		http.Error(w, "forbidden: not repo owner", http.StatusForbidden)
		return
	}

	// Get digest before deleting tag
	digest, err := getTagDigest(r.Context(), s.db, repoID, tag)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Delete tag
	s.db.ExecContext(r.Context(),
		`DELETE FROM tags WHERE repository_id = $1 AND name = $2`, repoID, tag)

	// Delegate manifest cleanup to existing logic
	s.cleanupManifest(r.Context(), repoID, repoName, digest, "")

	w.WriteHeader(http.StatusAccepted)
}
```

Note: this file needs `"database/sql"` import — add it to the import block.

- [ ] **Step 2: Register /api/ route in main.go**

Read `main.go`, then add after the `/v2/` route:

```go
func (s *server) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/api/", s.routeAPI)
	mux.HandleFunc("/v2/", s.routeV2)
}
```

- [ ] **Step 3: Verify**

```bash
cd edd-cloud-interface/services/registry && go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add edd-cloud-interface/services/registry/
git commit -m "feat(registry): add session-auth /api/ endpoints for repo and tag browsing"
```

---

## Chunk 3: Registry Images Page (Frontend)

### Task 4: Add API helper and hook

**Files:**
- Modify: `edd-cloud-interface/frontend/src/lib/api.ts`
- Create: `edd-cloud-interface/frontend/src/hooks/useRegistry.ts`

- [ ] **Step 1: Add registry base URL builder**

In `api.ts`, add:

```typescript
export function buildRegistryBase(): string {
  return buildServiceBase("registry");
}
```

- [ ] **Step 2: Create useRegistry hook**

Create `edd-cloud-interface/frontend/src/hooks/useRegistry.ts`:

```typescript
import { useState, useCallback, useEffect } from "react";
import { buildRegistryBase, getAuthHeaders } from "@/lib/api";

export interface RepoInfo {
  name: string;
  visibility: number;
  owner_id: string;
  tag_count: number;
  total_size: number;
  last_pushed: string;
}

export interface TagInfo {
  name: string;
  digest: string;
  size: number;
  pushed_at: string;
}

export function useRegistry(userId?: string) {
  const [repos, setRepos] = useState<RepoInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string>("");

  const loadRepos = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const resp = await fetch(`${buildRegistryBase()}/api/repos`, {
        headers: getAuthHeaders(),
      });
      if (!resp.ok) throw new Error(`${resp.status}`);
      const data = await resp.json();
      setRepos(data.repositories || []);
    } catch (e: any) {
      setError(e.message || "Failed to load repositories");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    loadRepos();
  }, [loadRepos]);

  const loadTags = useCallback(async (repoName: string): Promise<TagInfo[]> => {
    const resp = await fetch(
      `${buildRegistryBase()}/api/repos/${repoName}/tags`,
      { headers: getAuthHeaders() }
    );
    if (!resp.ok) return [];
    const data = await resp.json();
    return data.tags || [];
  }, []);

  const setVisibility = useCallback(async (repoName: string, visibility: number) => {
    await fetch(`${buildRegistryBase()}/api/repos/${repoName}/visibility`, {
      method: "PUT",
      headers: { ...getAuthHeaders(), "Content-Type": "application/json" },
      body: JSON.stringify({ visibility }),
    });
    loadRepos();
  }, [loadRepos]);

  const deleteTag = useCallback(async (repoName: string, tag: string) => {
    await fetch(`${buildRegistryBase()}/api/repos/${repoName}/tags/${tag}`, {
      method: "DELETE",
      headers: getAuthHeaders(),
    });
  }, []);

  const myRepos = repos.filter((r) => r.owner_id === userId);
  const publicRepos = repos.filter((r) => r.visibility > 0 && r.owner_id !== userId);

  return { repos, myRepos, publicRepos, loading, error, loadRepos, loadTags, setVisibility, deleteTag };
}
```

- [ ] **Step 3: Verify**

```bash
cd edd-cloud-interface/frontend && npx tsc --noEmit
```

- [ ] **Step 4: Commit**

```bash
git add edd-cloud-interface/frontend/src/lib/api.ts edd-cloud-interface/frontend/src/hooks/useRegistry.ts
git commit -m "feat(registry): add useRegistry hook and registry API builder"
```

### Task 5: Create Registry page components

**Files:**
- Create: `edd-cloud-interface/frontend/src/components/registry/RepoList.tsx`
- Create: `edd-cloud-interface/frontend/src/components/registry/RepoDetail.tsx`
- Create: `edd-cloud-interface/frontend/src/pages/RegistryPage.tsx`

- [ ] **Step 1: Create RepoList component**

Read existing component patterns (e.g., `ContainerList.tsx`) for styling reference. Create `components/registry/RepoList.tsx`:

```tsx
import { Badge } from "@/components/ui/badge";
import { formatBytes, formatTimestamp } from "@/lib/utils";
import type { RepoInfo } from "@/hooks/useRegistry";

interface RepoListProps {
  repos: RepoInfo[];
  onSelect: (name: string) => void;
}

export function RepoList({ repos, onSelect }: RepoListProps) {
  if (repos.length === 0) {
    return (
      <div className="text-center py-12 text-muted-foreground">
        No repositories found.
      </div>
    );
  }

  return (
    <div className="space-y-2">
      {repos.map((repo) => (
        <div
          key={repo.name}
          onClick={() => onSelect(repo.name)}
          className="bg-card border border-border rounded-lg p-4 cursor-pointer hover:border-primary/50 transition-colors"
        >
          <div className="flex items-center justify-between">
            <div>
              <div className="font-medium">{repo.name}</div>
              <div className="text-sm text-muted-foreground mt-1">
                {repo.tag_count} tag{repo.tag_count !== 1 ? "s" : ""} · {formatBytes(repo.total_size)} · Last pushed {formatTimestamp(repo.last_pushed)}
              </div>
            </div>
            <Badge variant={repo.visibility > 0 ? "default" : "secondary"}>
              {repo.visibility > 0 ? "Public" : "Private"}
            </Badge>
          </div>
        </div>
      ))}
    </div>
  );
}
```

- [ ] **Step 2: Create RepoDetail component**

Create `components/registry/RepoDetail.tsx`:

```tsx
import { useState, useEffect } from "react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { CopyableText } from "@/components/ui/copyable-text";
import { ArrowLeft, Trash2, Eye, EyeOff } from "lucide-react";
import { formatBytes, formatTimestamp } from "@/lib/utils";
import type { TagInfo } from "@/hooks/useRegistry";

interface RepoDetailProps {
  repoName: string;
  ownerId: string;
  visibility: number;
  currentUserId?: string;
  onBack: () => void;
  onLoadTags: (name: string) => Promise<TagInfo[]>;
  onDeleteTag: (name: string, tag: string) => Promise<void>;
  onSetVisibility: (name: string, visibility: number) => Promise<void>;
}

export function RepoDetail({
  repoName, ownerId, visibility, currentUserId,
  onBack, onLoadTags, onDeleteTag, onSetVisibility,
}: RepoDetailProps) {
  const [tags, setTags] = useState<TagInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const isOwner = currentUserId === ownerId;

  useEffect(() => {
    setLoading(true);
    onLoadTags(repoName).then((t) => {
      setTags(t);
      setLoading(false);
    });
  }, [repoName, onLoadTags]);

  const handleDelete = async (tag: string) => {
    await onDeleteTag(repoName, tag);
    setTags((prev) => prev.filter((t) => t.name !== tag));
  };

  const toggleVisibility = async () => {
    const newVis = visibility > 0 ? 0 : 1;
    await onSetVisibility(repoName, newVis);
  };

  return (
    <div className="max-w-3xl">
      <div className="flex items-center gap-4 mb-6">
        <Button variant="outline" size="sm" onClick={onBack}>
          <ArrowLeft className="w-4 h-4 mr-2" />Back
        </Button>
        <h2 className="text-xl font-semibold">{repoName}</h2>
        <Badge variant={visibility > 0 ? "default" : "secondary"}>
          {visibility > 0 ? "Public" : "Private"}
        </Badge>
        {isOwner && (
          <Button variant="ghost" size="sm" onClick={toggleVisibility}>
            {visibility > 0 ? <EyeOff className="w-4 h-4 mr-1" /> : <Eye className="w-4 h-4 mr-1" />}
            {visibility > 0 ? "Make Private" : "Make Public"}
          </Button>
        )}
      </div>

      <div className="bg-card border border-border rounded-lg">
        <div className="px-5 py-4 border-b border-border">
          <h3 className="text-sm font-semibold">Tags</h3>
        </div>
        <div className="divide-y divide-border">
          {loading ? (
            <div className="p-5 text-muted-foreground">Loading...</div>
          ) : tags.length === 0 ? (
            <div className="p-5 text-muted-foreground">No tags.</div>
          ) : (
            tags.map((tag) => (
              <div key={tag.name} className="px-5 py-3 flex items-center justify-between">
                <div>
                  <div className="font-medium text-sm">{tag.name}</div>
                  <div className="text-xs text-muted-foreground mt-1">
                    <CopyableText
                      text={`docker pull registry.cloud.eddisonso.com/${repoName}:${tag.name}`}
                      displayText={tag.digest.slice(0, 19) + "..."}
                    />
                    {" · "}{formatBytes(tag.size)} · {formatTimestamp(tag.pushed_at)}
                  </div>
                </div>
                {isOwner && (
                  <Button variant="ghost" size="sm" onClick={() => handleDelete(tag.name)}>
                    <Trash2 className="w-4 h-4 text-destructive" />
                  </Button>
                )}
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  );
}
```

- [ ] **Step 3: Create RegistryPage**

Create `edd-cloud-interface/frontend/src/pages/RegistryPage.tsx`:

```tsx
import { useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { useAuth } from "@/contexts/AuthContext";
import { useRegistry } from "@/hooks/useRegistry";
import { RepoList } from "@/components/registry/RepoList";
import { RepoDetail } from "@/components/registry/RepoDetail";
import { PageHeader } from "@/components/ui/page-header";
import { Breadcrumb } from "@/components/ui/breadcrumb";
import { Package } from "lucide-react";

export function RegistryPage() {
  const location = useLocation();
  const navigate = useNavigate();
  const { userId } = useAuth();
  const { myRepos, publicRepos, loading, error, loadTags, setVisibility, deleteTag, repos } = useRegistry(userId);
  const [tab, setTab] = useState<"mine" | "public">("mine");

  // Parse repo name from URL: /storage/registry/test/echo -> "test/echo"
  const pathAfterRegistry = location.pathname.replace(/^\/storage\/registry\/?/, "");
  const selectedRepo = pathAfterRegistry || null;

  if (selectedRepo) {
    const repo = repos.find((r) => r.name === selectedRepo);
    return (
      <>
        <Breadcrumb items={[
          { label: "Storage", href: "/storage" },
          { label: "Registry", href: "/storage/registry" },
          { label: selectedRepo },
        ]} />
        <RepoDetail
          repoName={selectedRepo}
          ownerId={repo?.owner_id || ""}
          visibility={repo?.visibility || 0}
          currentUserId={userId}
          onBack={() => navigate("/storage/registry")}
          onLoadTags={loadTags}
          onDeleteTag={deleteTag}
          onSetVisibility={setVisibility}
        />
      </>
    );
  }

  return (
    <>
      <Breadcrumb items={[
        { label: "Storage", href: "/storage" },
        { label: "Registry" },
      ]} />
      <PageHeader
        title="Container Registry"
        description="Browse and manage container images"
        icon={<Package className="w-5 h-5" />}
      />
      {error && <div className="text-destructive text-sm mb-4">{error}</div>}

      <div className="flex gap-2 mb-4">
        <button
          onClick={() => setTab("mine")}
          className={`px-4 py-2 rounded-md text-sm font-medium transition-colors ${
            tab === "mine" ? "bg-primary text-primary-foreground" : "bg-muted text-muted-foreground hover:text-foreground"
          }`}
        >
          My Repositories
        </button>
        <button
          onClick={() => setTab("public")}
          className={`px-4 py-2 rounded-md text-sm font-medium transition-colors ${
            tab === "public" ? "bg-primary text-primary-foreground" : "bg-muted text-muted-foreground hover:text-foreground"
          }`}
        >
          Public
        </button>
      </div>

      {loading ? (
        <div className="text-muted-foreground">Loading...</div>
      ) : (
        <RepoList
          repos={tab === "mine" ? myRepos : publicRepos}
          onSelect={(name) => navigate(`/storage/registry/${name}`)}
        />
      )}
    </>
  );
}
```

- [ ] **Step 4: Add route and nav item**

In `App.tsx`, add the route (read first to find exact location):
```tsx
<Route path="/storage/registry/*" element={<RegistryPage />} />
```

In `constants.ts`, update the Storage nav item to have subItems:
```typescript
{
  id: "storage",
  label: "Storage",
  icon: HardDrive,
  path: "/storage",
  subItems: [
    { id: "files", label: "Files", icon: HardDrive, path: "/storage" },
    { id: "registry", label: "Registry", icon: Package, path: "/storage/registry" },
  ],
},
```

Import `Package` from `lucide-react` in constants.ts.

- [ ] **Step 5: Verify**

```bash
cd edd-cloud-interface/frontend && npx tsc --noEmit
```

- [ ] **Step 6: Commit**

```bash
git add edd-cloud-interface/frontend/
git commit -m "feat(registry): add registry images page with repo list, detail, and tag management"
```

---

## Chunk 4: Container Logs

### Task 6: Add logs WebSocket endpoint to compute backend

**Files:**
- Create: `edd-cloud-interface/services/compute/internal/api/logs.go`
- Modify: `edd-cloud-interface/services/compute/internal/api/handler.go` (add route)
- Modify: `edd-cloud-interface/services/compute/internal/k8s/client.go` (add GetPodLogs)

- [ ] **Step 1: Add GetPodLogs to K8s client**

Read `client.go` first. Add:

```go
// GetPodLogs returns a stream of log lines for the container's pod.
func (c *Client) GetPodLogs(ctx context.Context, namespace, podName string, follow bool, tailLines int64) (io.ReadCloser, error) {
	opts := &corev1.PodLogOptions{
		Follow:     follow,
		Timestamps: true,
		TailLines:  &tailLines,
	}
	return c.clientset.CoreV1().Pods(namespace).GetLogs(podName, opts).Stream(ctx)
}
```

Add `"io"` to the import block if not present.

- [ ] **Step 2: Create logs.go handler**

Read the existing terminal handler pattern (the `HandleTerminal` function) for WebSocket upgrade reference. Create `logs.go`:

```go
package api

import (
	"bufio"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gorilla/websocket"
)

func (h *Handler) HandleContainerLogs(w http.ResponseWriter, r *http.Request) {
	containerID := r.PathValue("id")
	if containerID == "" {
		writeError(w, "container ID required", http.StatusBadRequest)
		return
	}

	userID, _, ok := getUserFromContext(r.Context())
	if !ok {
		writeError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Look up container
	container, err := h.db.GetContainer(userID, containerID)
	if err != nil {
		writeError(w, "container not found", http.StatusNotFound)
		return
	}

	if container.Status != "running" {
		writeError(w, "container is not running", http.StatusBadRequest)
		return
	}

	// Get pod name
	podName, err := h.k8s.GetPodName(r.Context(), container.Namespace)
	if err != nil {
		writeError(w, "pod not found", http.StatusNotFound)
		return
	}

	tailLines := int64(100)
	if t := r.URL.Query().Get("tail"); t != "" {
		if n, err := strconv.ParseInt(t, 10, 64); err == nil && n > 0 {
			tailLines = n
		}
	}

	// Upgrade to WebSocket
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("logs: websocket upgrade failed", "err", err)
		return
	}
	defer conn.Close()

	// Stream logs
	ctx := r.Context()
	stream, err := h.k8s.GetPodLogs(ctx, container.Namespace, podName, true, tailLines)
	if err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte("error: "+err.Error()))
		return
	}
	defer stream.Close()

	scanner := bufio.NewScanner(stream)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		if err := conn.WriteMessage(websocket.TextMessage, scanner.Bytes()); err != nil {
			break
		}
	}
}
```

Note: check if `GetPodName` already exists on the K8s client. If not, add a method that lists pods by namespace label and returns the first running pod name. Read `client.go` to determine the exact pattern.

- [ ] **Step 3: Register route**

In `handler.go`, add:
```go
h.mux.HandleFunc("GET /compute/containers/{id}/logs", h.authMiddleware(h.scopeCheckContainer("read", h.HandleContainerLogs)))
```

- [ ] **Step 4: Verify**

```bash
cd edd-cloud-interface/services/compute && go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add edd-cloud-interface/services/compute/
git commit -m "feat(compute): add WebSocket container logs streaming endpoint"
```

### Task 7: Add container logs frontend

**Files:**
- Create: `edd-cloud-interface/frontend/src/hooks/useContainerLogs.ts`
- Create: `edd-cloud-interface/frontend/src/components/compute/ContainerLogs.tsx`
- Modify: `edd-cloud-interface/frontend/src/components/compute/ContainerDetail.tsx`

- [ ] **Step 1: Create useContainerLogs hook**

```typescript
import { useState, useRef, useCallback, useEffect } from "react";
import { buildComputeWsBase, getAuthToken } from "@/lib/api";

export function useContainerLogs(containerId: string, active: boolean) {
  const [logs, setLogs] = useState<string[]>([]);
  const [connected, setConnected] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);
  const bufferRef = useRef<string[]>([]);
  const maxLogs = 2000;

  const flush = useCallback(() => {
    if (bufferRef.current.length === 0) return;
    const batch = bufferRef.current;
    bufferRef.current = [];
    setLogs((prev) => {
      const next = [...prev, ...batch];
      return next.length > maxLogs ? next.slice(-maxLogs) : next;
    });
  }, []);

  useEffect(() => {
    if (!active || !containerId) return;

    const token = getAuthToken();
    const url = `${buildComputeWsBase()}/compute/containers/${containerId}/logs?token=${token}&tail=100`;
    const ws = new WebSocket(url);
    wsRef.current = ws;

    ws.onopen = () => setConnected(true);
    ws.onclose = () => setConnected(false);
    ws.onmessage = (e) => {
      bufferRef.current.push(e.data);
    };

    const interval = setInterval(flush, 200);

    return () => {
      clearInterval(interval);
      flush();
      ws.close();
      wsRef.current = null;
      setConnected(false);
    };
  }, [containerId, active, flush]);

  const clear = useCallback(() => setLogs([]), []);

  return { logs, connected, clear };
}
```

- [ ] **Step 2: Create ContainerLogs component**

```tsx
import { useRef, useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Download, Trash2 } from "lucide-react";
import { useContainerLogs } from "@/hooks/useContainerLogs";

interface ContainerLogsProps {
  containerId: string;
  active: boolean;
}

export function ContainerLogs({ containerId, active }: ContainerLogsProps) {
  const { logs, connected, clear } = useContainerLogs(containerId, active);
  const scrollRef = useRef<HTMLDivElement>(null);
  const [autoScroll, setAutoScroll] = useState(true);

  // Auto-scroll
  useEffect(() => {
    if (autoScroll && scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [logs, autoScroll]);

  // Detect user scroll
  const handleScroll = () => {
    if (!scrollRef.current) return;
    const { scrollTop, scrollHeight, clientHeight } = scrollRef.current;
    setAutoScroll(scrollHeight - scrollTop - clientHeight < 50);
  };

  const download = () => {
    const blob = new Blob([logs.join("\n")], { type: "text/plain" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `container-${containerId}-logs.txt`;
    a.click();
    URL.revokeObjectURL(url);
  };

  return (
    <div className="flex flex-col h-[500px]">
      <div className="flex items-center justify-between px-3 py-2 border-b border-border bg-card rounded-t-lg">
        <div className="flex items-center gap-2 text-sm">
          <span className={`w-2 h-2 rounded-full ${connected ? "bg-green-500" : "bg-muted"}`} />
          <span className="text-muted-foreground">{connected ? "Streaming" : "Disconnected"}</span>
        </div>
        <div className="flex gap-1">
          <Button variant="ghost" size="sm" onClick={clear}>
            <Trash2 className="w-3.5 h-3.5" />
          </Button>
          <Button variant="ghost" size="sm" onClick={download}>
            <Download className="w-3.5 h-3.5" />
          </Button>
        </div>
      </div>
      <div
        ref={scrollRef}
        onScroll={handleScroll}
        className="flex-1 overflow-y-auto bg-[#0d0d14] p-3 font-mono text-xs leading-relaxed rounded-b-lg"
      >
        {logs.length === 0 ? (
          <div className="text-muted-foreground">Waiting for logs...</div>
        ) : (
          logs.map((line, i) => {
            // K8s timestamp format: 2026-03-15T17:00:01.123Z <message>
            const spaceIdx = line.indexOf(" ");
            const ts = spaceIdx > 0 ? line.slice(0, spaceIdx) : "";
            const msg = spaceIdx > 0 ? line.slice(spaceIdx + 1) : line;
            return (
              <div key={i} className="whitespace-pre-wrap break-all">
                {ts && <span className="text-muted-foreground mr-2">{ts}</span>}
                <span>{msg}</span>
              </div>
            );
          })
        )}
        {!autoScroll && (
          <button
            onClick={() => {
              setAutoScroll(true);
              scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight });
            }}
            className="fixed bottom-4 right-4 bg-primary text-primary-foreground px-3 py-1.5 rounded-md text-xs shadow-lg"
          >
            Jump to bottom
          </button>
        )}
      </div>
    </div>
  );
}
```

- [ ] **Step 3: Add Logs tab to ContainerDetail**

Read `ContainerDetail.tsx` first. Add a tab bar at the top of the detail view, and conditionally render the logs component:

```tsx
import { ContainerLogs } from "./ContainerLogs";

// Add state for active tab
const [activeTab, setActiveTab] = useState<"info" | "logs">("info");

// Add tab bar after the header/back button section:
<div className="flex gap-2 mb-4">
  <button
    onClick={() => setActiveTab("info")}
    className={`px-4 py-2 rounded-md text-sm font-medium transition-colors ${
      activeTab === "info" ? "bg-primary text-primary-foreground" : "bg-muted text-muted-foreground"
    }`}
  >
    Info
  </button>
  <button
    onClick={() => setActiveTab("logs")}
    className={`px-4 py-2 rounded-md text-sm font-medium transition-colors ${
      activeTab === "logs" ? "bg-primary text-primary-foreground" : "bg-muted text-muted-foreground"
    }`}
  >
    Logs
  </button>
</div>

// Wrap existing info sections in a conditional:
{activeTab === "info" && (
  // ... existing info, access control, persistent storage sections
)}

{activeTab === "logs" && (
  <ContainerLogs containerId={container.id} active={activeTab === "logs"} />
)}
```

- [ ] **Step 4: Verify**

```bash
cd edd-cloud-interface/frontend && npx tsc --noEmit
```

- [ ] **Step 5: Commit**

```bash
git add edd-cloud-interface/frontend/ edd-cloud-interface/services/compute/
git commit -m "feat(compute): add real-time container logs streaming with WebSocket"
```

---

## Chunk 5: Compute ListImages Update + Cleanup

### Task 8: Update compute ListImages to use /api/repos

**Files:**
- Modify: `edd-cloud-interface/services/compute/internal/api/containers.go`

- [ ] **Step 1: Simplify ListImages**

Read the current `ListImages` function. Replace the `/v2/_catalog` + `/v2/{name}/tags/list` double call with a single call to `/api/repos`:

```go
func (h *Handler) ListImages(w http.ResponseWriter, r *http.Request) {
	images := []map[string]string{
		{"name": "Debian (Base)", "image": defaultImage, "source": "builtin"},
	}

	registryURL := os.Getenv("REGISTRY_URL")
	if registryURL == "" {
		registryURL = "http://edd-registry:80"
	}

	authHeader := r.Header.Get("Authorization")

	req, _ := http.NewRequestWithContext(r.Context(), "GET", registryURL+"/api/repos", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		// Fall back to just builtin images
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(images)
		return
	}
	defer resp.Body.Close()

	var data struct {
		Repositories []struct {
			Name     string `json:"name"`
			TagCount int    `json:"tag_count"`
		} `json:"repositories"`
	}
	if json.NewDecoder(resp.Body).Decode(&data) == nil {
		for _, repo := range data.Repositories {
			// Fetch tags for each repo
			tagReq, _ := http.NewRequestWithContext(r.Context(), "GET",
				fmt.Sprintf("%s/api/repos/%s/tags", registryURL, repo.Name), nil)
			if authHeader != "" {
				tagReq.Header.Set("Authorization", authHeader)
			}
			tagResp, err := http.DefaultClient.Do(tagReq)
			if err != nil || tagResp.StatusCode != http.StatusOK {
				if tagResp != nil { tagResp.Body.Close() }
				continue
			}
			var tagData struct {
				Tags []struct {
					Name string `json:"name"`
				} `json:"tags"`
			}
			json.NewDecoder(tagResp.Body).Decode(&tagData)
			tagResp.Body.Close()
			for _, tag := range tagData.Tags {
				images = append(images, map[string]string{
					"name":   fmt.Sprintf("%s:%s", repo.Name, tag.Name),
					"image":  fmt.Sprintf("registry.cloud.eddisonso.com/%s:%s", repo.Name, tag.Name),
					"source": "registry",
				})
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(images)
}
```

- [ ] **Step 2: Verify**

```bash
cd edd-cloud-interface/services/compute && go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add edd-cloud-interface/services/compute/
git commit -m "refactor(compute): use registry /api/repos for image listing"
```
