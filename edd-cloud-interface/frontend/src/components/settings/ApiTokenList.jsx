import { useState, useEffect, useCallback } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { useAuth } from "@/contexts/AuthContext";
import {
  buildAuthBase,
  buildComputeBase,
  buildStorageBase,
  getAuthHeaders,
  copyToClipboard,
} from "@/lib/api";
import {
  Plus,
  Trash2,
  Copy,
  AlertTriangle,
  KeyRound,
  ChevronDown,
  ChevronRight,
  Loader2,
} from "lucide-react";

const EXPIRY_OPTIONS = [
  { value: "30d", label: "30 days" },
  { value: "90d", label: "90 days" },
  { value: "365d", label: "1 year" },
  { value: "never", label: "No expiry" },
];

const CONTAINER_ACTIONS = ["create", "read", "update", "delete"];
const KEY_ACTIONS = ["create", "read", "delete"];
const NAMESPACE_ACTIONS = ["create", "read", "update", "delete"];
const FILE_ACTIONS = ["create", "read", "delete"];

function formatDate(unix) {
  if (!unix) return "Never";
  return new Date(unix * 1000).toLocaleDateString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}

function formatRelative(unix) {
  if (!unix) return "Never";
  const now = Date.now() / 1000;
  if (unix < now) return "Expired";
  const days = Math.ceil((unix - now) / 86400);
  if (days === 1) return "1 day";
  if (days < 30) return `${days} days`;
  const months = Math.round(days / 30);
  if (months < 12) return `${months} month${months > 1 ? "s" : ""}`;
  return `${Math.round(days / 365)} year${Math.round(days / 365) > 1 ? "s" : ""}`;
}

function scopeSummary(scopes) {
  const parts = [];
  for (const [scope, actions] of Object.entries(scopes)) {
    const segments = scope.split(".");
    // 4-segment: root.uid.resource.id -> "resource(id)"
    // 3-segment: root.uid.resource -> "resource"
    // 2-segment: root.uid -> root
    let label;
    if (segments.length === 4) {
      label = `${segments[2]}(${segments[3]})`;
    } else if (segments.length === 3) {
      label = segments[2];
    } else {
      label = segments[0];
    }
    parts.push(`${label}: ${actions.join(", ")}`);
  }
  return parts.join("; ");
}

export function ApiTokenList() {
  const { userId } = useAuth();
  const [tokens, setTokens] = useState([]);
  const [loading, setLoading] = useState(true);
  const [showCreate, setShowCreate] = useState(false);
  const [createdToken, setCreatedToken] = useState(null);

  const loadTokens = async () => {
    try {
      const res = await fetch(`${buildAuthBase()}/api/tokens`, {
        headers: getAuthHeaders(),
      });
      if (res.ok) {
        const data = await res.json();
        setTokens(data || []);
      }
    } catch (err) {
      console.warn("Failed to load tokens:", err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadTokens();
  }, []);

  const handleDelete = async (id) => {
    try {
      const res = await fetch(`${buildAuthBase()}/api/tokens/${id}`, {
        method: "DELETE",
        headers: getAuthHeaders(),
      });
      if (res.ok) {
        setTokens((prev) => prev.filter((t) => t.id !== id));
      }
    } catch (err) {
      console.warn("Failed to delete token:", err);
    }
  };

  const handleCreated = (token, meta) => {
    setCreatedToken(token);
    setShowCreate(false);
    loadTokens();
  };

  if (loading) {
    return (
      <Card>
        <CardContent className="py-8 text-center text-muted-foreground">
          Loading tokens...
        </CardContent>
      </Card>
    );
  }

  return (
    <div className="space-y-4">
      {/* Created token display */}
      {createdToken && (
        <Card className="border-primary/50">
          <CardContent className="py-4">
            <div className="flex items-start gap-3">
              <AlertTriangle className="w-5 h-5 text-primary mt-0.5 flex-shrink-0" />
              <div className="flex-1 min-w-0">
                <p className="text-sm font-medium mb-2">
                  Your new API token. Copy it now — it won't be shown again.
                </p>
                <div className="flex items-center gap-2">
                  <code className="flex-1 text-xs bg-secondary px-3 py-2 rounded font-mono break-all select-all">
                    {createdToken}
                  </code>
                  <Button
                    variant="outline"
                    size="icon"
                    onClick={() => copyToClipboard(createdToken)}
                  >
                    <Copy className="w-4 h-4" />
                  </Button>
                </div>
              </div>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setCreatedToken(null)}
              >
                Dismiss
              </Button>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Create form */}
      {showCreate ? (
        <CreateTokenForm
          userId={userId}
          onCreated={handleCreated}
          onCancel={() => setShowCreate(false)}
        />
      ) : (
        <div className="flex justify-end">
          <Button onClick={() => setShowCreate(true)}>
            <Plus className="w-4 h-4 mr-2" />
            Create token
          </Button>
        </div>
      )}

      {/* Token list */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Active tokens</CardTitle>
        </CardHeader>
        <CardContent>
          {tokens.length === 0 ? (
            <div className="text-center py-8 text-muted-foreground">
              <KeyRound className="w-8 h-8 mx-auto mb-2 opacity-50" />
              <p>No API tokens yet</p>
              <p className="text-xs mt-1">
                Create a token for programmatic access to compute and storage APIs.
              </p>
            </div>
          ) : (
            <div className="space-y-3">
              {tokens.map((token) => (
                <div
                  key={token.id}
                  className="flex items-center justify-between p-3 rounded-md border border-border bg-secondary/30"
                >
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2 mb-1">
                      <span className="font-medium text-sm">{token.name}</span>
                      {token.expires_at > 0 && token.expires_at < Date.now() / 1000 && (
                        <Badge variant="destructive" className="text-xs">Expired</Badge>
                      )}
                    </div>
                    <div className="text-xs text-muted-foreground space-y-0.5">
                      <p>{scopeSummary(token.scopes)}</p>
                      <p>
                        Created {formatDate(token.created_at)}
                        {token.expires_at > 0 && ` · Expires in ${formatRelative(token.expires_at)}`}
                        {token.last_used_at > 0 && ` · Last used ${formatDate(token.last_used_at)}`}
                      </p>
                    </div>
                  </div>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="text-muted-foreground hover:text-destructive"
                    onClick={() => handleDelete(token.id)}
                  >
                    <Trash2 className="w-4 h-4" />
                  </Button>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

// Action toggle pill
function ActionPill({ action, selected, onClick }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`px-2 py-0.5 text-xs rounded border transition-colors ${
        selected
          ? "bg-primary text-primary-foreground border-primary"
          : "bg-secondary border-border text-muted-foreground hover:text-foreground"
      }`}
    >
      {action}
    </button>
  );
}

// Collapsible section header
function SectionHeader({ label, expanded, onToggle, children }) {
  return (
    <div>
      <button
        type="button"
        onClick={onToggle}
        className="flex items-center gap-1.5 text-sm font-medium w-full text-left mb-2"
      >
        {expanded ? (
          <ChevronDown className="w-3.5 h-3.5" />
        ) : (
          <ChevronRight className="w-3.5 h-3.5" />
        )}
        {label}
      </button>
      {expanded && <div className="pl-5 space-y-3">{children}</div>}
    </div>
  );
}

function CreateTokenForm({ userId, onCreated, onCancel }) {
  const [name, setName] = useState("");
  const [expiresIn, setExpiresIn] = useState("90d");
  const [selectedScopes, setSelectedScopes] = useState({});
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState("");

  // Lazy-loaded resources
  const [containers, setContainers] = useState(null); // null = not loaded
  const [namespaces, setNamespaces] = useState(null);
  const [loadingContainers, setLoadingContainers] = useState(false);
  const [loadingNamespaces, setLoadingNamespaces] = useState(false);

  // Expanded sections
  const [computeExpanded, setComputeExpanded] = useState(false);
  const [storageExpanded, setStorageExpanded] = useState(false);
  const [specificContainersExpanded, setSpecificContainersExpanded] = useState(false);
  const [specificNamespacesExpanded, setSpecificNamespacesExpanded] = useState(false);

  const fetchContainers = useCallback(async () => {
    if (containers !== null) return;
    setLoadingContainers(true);
    try {
      const res = await fetch(`${buildComputeBase()}/compute/containers`, {
        headers: getAuthHeaders(),
      });
      if (res.ok) {
        const data = await res.json();
        setContainers(data || []);
      } else {
        setContainers([]);
      }
    } catch {
      setContainers([]);
    } finally {
      setLoadingContainers(false);
    }
  }, [containers]);

  const fetchNamespaces = useCallback(async () => {
    if (namespaces !== null) return;
    setLoadingNamespaces(true);
    try {
      const res = await fetch(`${buildStorageBase()}/storage/namespaces`, {
        headers: getAuthHeaders(),
      });
      if (res.ok) {
        const data = await res.json();
        setNamespaces(data || []);
      } else {
        setNamespaces([]);
      }
    } catch {
      setNamespaces([]);
    } finally {
      setLoadingNamespaces(false);
    }
  }, [namespaces]);

  // Lazy load on expand
  useEffect(() => {
    if (specificContainersExpanded) fetchContainers();
  }, [specificContainersExpanded, fetchContainers]);

  useEffect(() => {
    if (specificNamespacesExpanded) fetchNamespaces();
  }, [specificNamespacesExpanded, fetchNamespaces]);

  const toggleAction = (scopeKey, action) => {
    setSelectedScopes((prev) => {
      const current = prev[scopeKey] || [];
      if (current.includes(action)) {
        const next = current.filter((a) => a !== action);
        if (next.length === 0) {
          const { [scopeKey]: _, ...rest } = prev;
          return rest;
        }
        return { ...prev, [scopeKey]: next };
      }
      return { ...prev, [scopeKey]: [...current, action] };
    });
  };

  const setAllActions = (scopeKey, actions) => {
    setSelectedScopes((prev) => {
      const current = prev[scopeKey] || [];
      if (current.length === actions.length) {
        const { [scopeKey]: _, ...rest } = prev;
        return rest;
      }
      return { ...prev, [scopeKey]: [...actions] };
    });
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    setError("");

    if (!name.trim()) {
      setError("Token name is required");
      return;
    }
    if (Object.keys(selectedScopes).length === 0) {
      setError("Select at least one permission");
      return;
    }

    setCreating(true);
    try {
      const res = await fetch(`${buildAuthBase()}/api/tokens`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          ...getAuthHeaders(),
        },
        body: JSON.stringify({
          name: name.trim(),
          scopes: selectedScopes,
          expires_in: expiresIn,
        }),
      });

      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error(data.error || "Failed to create token");
      }

      const data = await res.json();
      onCreated(data.token, data);
    } catch (err) {
      setError(err.message);
    } finally {
      setCreating(false);
    }
  };

  // Scope key helpers
  const broadContainersKey = `compute.${userId}.containers`;
  const broadKeysKey = `compute.${userId}.keys`;
  const broadNamespacesKey = `storage.${userId}.namespaces`;
  const broadFilesKey = `storage.${userId}.files`;
  const containerKey = (id) => `compute.${userId}.containers.${id}`;
  const nsNamespacesKey = (nsName) => `storage.${userId}.namespaces.${nsName}`;
  const nsFilesKey = (nsName) => `storage.${userId}.files.${nsName}`;

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Create API token</CardTitle>
      </CardHeader>
      <CardContent>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="token-name">Name</Label>
              <Input
                id="token-name"
                placeholder="e.g., CI/CD pipeline"
                value={name}
                onChange={(e) => setName(e.target.value)}
                autoFocus
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="token-expiry">Expiry</Label>
              <select
                id="token-expiry"
                value={expiresIn}
                onChange={(e) => setExpiresIn(e.target.value)}
                className="w-full h-9 rounded-md border border-border bg-background px-3 text-sm"
              >
                {EXPIRY_OPTIONS.map((opt) => (
                  <option key={opt.value} value={opt.value}>
                    {opt.label}
                  </option>
                ))}
              </select>
            </div>
          </div>

          <div className="space-y-3">
            <Label>Permissions</Label>
            <div className="grid grid-cols-2 gap-4">
              {/* Compute section */}
              <div className="border border-border rounded-md p-3 space-y-3">
                <SectionHeader
                  label="Compute"
                  expanded={computeExpanded}
                  onToggle={() => setComputeExpanded(!computeExpanded)}
                >
                  {/* All Containers */}
                  <ResourceRow
                    label="All Containers"
                    scopeKey={broadContainersKey}
                    actions={CONTAINER_ACTIONS}
                    selectedScopes={selectedScopes}
                    onToggle={toggleAction}
                    onToggleAll={setAllActions}
                  />

                  {/* Specific Containers (lazy) */}
                  <SectionHeader
                    label="Specific Containers"
                    expanded={specificContainersExpanded}
                    onToggle={() => setSpecificContainersExpanded(!specificContainersExpanded)}
                  >
                    {loadingContainers && (
                      <div className="flex items-center gap-2 text-xs text-muted-foreground py-1">
                        <Loader2 className="w-3 h-3 animate-spin" />
                        Loading containers...
                      </div>
                    )}
                    {containers !== null && containers.length === 0 && !loadingContainers && (
                      <p className="text-xs text-muted-foreground">No containers found</p>
                    )}
                    {containers?.map((c) => (
                      <ResourceRow
                        key={c.id}
                        label={c.name || c.id.slice(0, 8)}
                        scopeKey={containerKey(c.id)}
                        actions={CONTAINER_ACTIONS}
                        selectedScopes={selectedScopes}
                        onToggle={toggleAction}
                        onToggleAll={setAllActions}
                      />
                    ))}
                  </SectionHeader>

                  {/* SSH Keys */}
                  <ResourceRow
                    label="SSH Keys"
                    scopeKey={broadKeysKey}
                    actions={KEY_ACTIONS}
                    selectedScopes={selectedScopes}
                    onToggle={toggleAction}
                    onToggleAll={setAllActions}
                  />
                </SectionHeader>
              </div>

              {/* Storage section */}
              <div className="border border-border rounded-md p-3 space-y-3">
                <SectionHeader
                  label="Storage"
                  expanded={storageExpanded}
                  onToggle={() => setStorageExpanded(!storageExpanded)}
                >
                  {/* All Namespaces + Files */}
                  <ResourceRow
                    label="All Namespaces"
                    scopeKey={broadNamespacesKey}
                    actions={NAMESPACE_ACTIONS}
                    selectedScopes={selectedScopes}
                    onToggle={toggleAction}
                    onToggleAll={setAllActions}
                  />
                  <ResourceRow
                    label="All Files"
                    scopeKey={broadFilesKey}
                    actions={FILE_ACTIONS}
                    selectedScopes={selectedScopes}
                    onToggle={toggleAction}
                    onToggleAll={setAllActions}
                  />

                  {/* Specific Namespaces (lazy) */}
                  <SectionHeader
                    label="Specific Namespaces"
                    expanded={specificNamespacesExpanded}
                    onToggle={() => setSpecificNamespacesExpanded(!specificNamespacesExpanded)}
                  >
                    {loadingNamespaces && (
                      <div className="flex items-center gap-2 text-xs text-muted-foreground py-1">
                        <Loader2 className="w-3 h-3 animate-spin" />
                        Loading namespaces...
                      </div>
                    )}
                    {namespaces !== null && namespaces.length === 0 && !loadingNamespaces && (
                      <p className="text-xs text-muted-foreground">No namespaces found</p>
                    )}
                    {namespaces?.map((ns) => (
                      <div key={ns.name} className="space-y-1.5">
                        <p className="text-xs font-medium text-foreground">{ns.name}</p>
                        <ResourceRow
                          label="Namespace"
                          scopeKey={nsNamespacesKey(ns.name)}
                          actions={NAMESPACE_ACTIONS}
                          selectedScopes={selectedScopes}
                          onToggle={toggleAction}
                          onToggleAll={setAllActions}

                        />
                        <ResourceRow
                          label="Files"
                          scopeKey={nsFilesKey(ns.name)}
                          actions={FILE_ACTIONS}
                          selectedScopes={selectedScopes}
                          onToggle={toggleAction}
                          onToggleAll={setAllActions}

                        />
                      </div>
                    ))}
                  </SectionHeader>
                </SectionHeader>
              </div>
            </div>
          </div>

          {error && <p className="text-sm text-destructive">{error}</p>}

          <div className="flex gap-2 justify-end">
            <Button type="button" variant="outline" onClick={onCancel}>
              Cancel
            </Button>
            <Button type="submit" disabled={creating}>
              {creating ? "Creating..." : "Create token"}
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  );
}

// A row with a label and action pills
function ResourceRow({
  label,
  scopeKey,
  actions,
  selectedScopes,
  onToggle,
  onToggleAll,
}) {
  const selected = selectedScopes[scopeKey] || [];
  return (
    <div>
      <div className="flex items-center justify-between mb-1">
        <p className="text-xs text-muted-foreground">
          {label}
        </p>
        <button
          type="button"
          onClick={() => onToggleAll(scopeKey, actions)}
          className="text-[10px] text-muted-foreground hover:text-foreground"
        >
          {selected.length === actions.length ? "none" : "all"}
        </button>
      </div>
      <div className="flex flex-wrap gap-1.5">
        {actions.map((action) => (
          <ActionPill
            key={action}
            action={action}
            selected={selected.includes(action)}
            onClick={() => onToggle(scopeKey, action)}
          />
        ))}
      </div>
    </div>
  );
}
