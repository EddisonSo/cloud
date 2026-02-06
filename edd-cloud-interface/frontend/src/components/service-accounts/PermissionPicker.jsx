import { useState, useEffect, useCallback } from "react";
import { Label } from "@/components/ui/label";
import {
  buildComputeBase,
  buildStorageBase,
  getAuthHeaders,
} from "@/lib/api";
import {
  ChevronDown,
  ChevronRight,
  Loader2,
} from "lucide-react";

export const EXPIRY_OPTIONS = [
  { value: "30d", label: "30 days" },
  { value: "90d", label: "90 days" },
  { value: "365d", label: "1 year" },
  { value: "never", label: "No expiry" },
];

export const CONTAINER_ACTIONS = ["create", "read", "update", "delete"];
export const KEY_ACTIONS = ["create", "read", "delete"];
export const NAMESPACE_ACTIONS = ["create", "read", "update", "delete"];
export const FILE_ACTIONS = ["create", "read", "delete"];

export function scopeSummary(scopes) {
  if (!scopes || Object.keys(scopes).length === 0) return "No permissions";
  const parts = [];
  for (const [scope, actions] of Object.entries(scopes)) {
    const segments = scope.split(".");
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

export function ActionPill({ action, selected, onClick }) {
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

export function SectionHeader({ label, expanded, onToggle, children }) {
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

export function ResourceRow({
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
        <p className="text-xs text-muted-foreground">{label}</p>
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

export function PermissionPicker({ userId, selectedScopes, setSelectedScopes }) {
  const [containers, setContainers] = useState(null);
  const [namespaces, setNamespaces] = useState(null);
  const [loadingContainers, setLoadingContainers] = useState(false);
  const [loadingNamespaces, setLoadingNamespaces] = useState(false);

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

  const broadContainersKey = `compute.${userId}.containers`;
  const broadKeysKey = `compute.${userId}.keys`;
  const broadNamespacesKey = `storage.${userId}.namespaces`;
  const broadFilesKey = `storage.${userId}.files`;
  const containerKey = (id) => `compute.${userId}.containers.${id}`;
  const nsNamespacesKey = (nsName) => `storage.${userId}.namespaces.${nsName}`;
  const nsFilesKey = (nsName) => `storage.${userId}.files.${nsName}`;

  return (
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
            <ResourceRow
              label="All Containers"
              scopeKey={broadContainersKey}
              actions={CONTAINER_ACTIONS}
              selectedScopes={selectedScopes}
              onToggle={toggleAction}
              onToggleAll={setAllActions}
            />
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
  );
}
