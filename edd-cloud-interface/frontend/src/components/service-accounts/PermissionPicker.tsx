import { useState, useEffect, useCallback } from "react";
import { Label } from "@/components/ui/label";
import {
  buildComputeBase,
  buildStorageBase,
  buildNetworkingBase,
  getAuthHeaders,
} from "@/lib/api";
import {
  ChevronDown,
  ChevronRight,
  Loader2,
} from "lucide-react";

interface ExpiryOption {
  value: string;
  label: string;
}

export const EXPIRY_OPTIONS: ExpiryOption[] = [
  { value: "30d", label: "30 days" },
  { value: "90d", label: "90 days" },
  { value: "365d", label: "1 year" },
  { value: "never", label: "No expiry" },
];

export const CONTAINER_ACTIONS: string[] = ["create", "read", "update", "delete", "start", "stop"];
export const KEY_ACTIONS: string[] = ["create", "read", "delete"];
export const NAMESPACE_ACTIONS: string[] = ["create", "read", "update", "delete"];
export const FILE_ACTIONS: string[] = ["create", "read", "delete"];
export const REGISTRY_ACTIONS: string[] = ["push", "pull", "delete"];
export const NETWORKING_ACTIONS: string[] = ["create", "read", "delete"];

export function scopeSummary(scopes: Record<string, string[]>): string {
  if (!scopes || Object.keys(scopes).length === 0) return "No permissions";
  const parts: string[] = [];
  for (const [scope, actions] of Object.entries(scopes)) {
    const segments = scope.split(".");
    let label: string;
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

interface ActionPillProps {
  action: string;
  selected: boolean;
  onClick: () => void;
}

export function ActionPill({ action, selected, onClick }: ActionPillProps): React.ReactElement {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`px-2 py-0.5 font-mono text-[11px] border transition-colors ${
        selected
          ? "bg-primary text-primary-foreground border-primary"
          : "bg-secondary border-border text-muted-foreground hover:text-foreground"
      }`}
    >
      {action}
    </button>
  );
}

interface SectionHeaderProps {
  label: string;
  expanded: boolean;
  onToggle: () => void;
  children?: React.ReactNode;
}

export function SectionHeader({ label, expanded, onToggle, children }: SectionHeaderProps): React.ReactElement {
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

interface ResourceRowProps {
  label: string;
  scopeKey: string;
  actions: string[];
  selectedScopes: Record<string, string[]>;
  onToggle: (scopeKey: string, action: string) => void;
  onToggleAll: (scopeKey: string, actions: string[]) => void;
}

export function ResourceRow({
  label,
  scopeKey,
  actions,
  selectedScopes,
  onToggle,
  onToggleAll,
}: ResourceRowProps): React.ReactElement {
  const selected = selectedScopes[scopeKey] || [];
  return (
    <div>
      <div className="flex items-center justify-between mb-1">
        <p className="font-mono text-[10px] uppercase tracking-[0.12em] text-faint">{label}</p>
        <button
          type="button"
          onClick={() => onToggleAll(scopeKey, actions)}
          className="font-mono text-[10px] text-faint hover:text-foreground"
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

interface ContainerItem {
  id: string;
  name?: string;
}

interface NamespaceItem {
  name: string;
}

interface DomainConnItem {
  id: string;
  zones: string[];
}

interface DomainMappingItem {
  id: string;
  domain: string;
}

interface PermissionPickerProps {
  userId: string | null | undefined;
  selectedScopes: Record<string, string[]>;
  setSelectedScopes: React.Dispatch<React.SetStateAction<Record<string, string[]>>>;
}

export function PermissionPicker({ userId, selectedScopes, setSelectedScopes }: PermissionPickerProps): React.ReactElement {
  const [containers, setContainers] = useState<ContainerItem[] | null>(null);
  const [namespaces, setNamespaces] = useState<NamespaceItem[] | null>(null);
  const [domainConns, setDomainConns] = useState<DomainConnItem[] | null>(null);
  const [domainMappings, setDomainMappings] = useState<DomainMappingItem[] | null>(null);
  const [loadingContainers, setLoadingContainers] = useState<boolean>(false);
  const [loadingNamespaces, setLoadingNamespaces] = useState<boolean>(false);
  const [loadingDomainConns, setLoadingDomainConns] = useState<boolean>(false);
  const [loadingDomainMappings, setLoadingDomainMappings] = useState<boolean>(false);

  const [computeExpanded, setComputeExpanded] = useState<boolean>(false);
  const [storageExpanded, setStorageExpanded] = useState<boolean>(false);
  const [networkingExpanded, setNetworkingExpanded] = useState<boolean>(false);
  const [specificContainersExpanded, setSpecificContainersExpanded] = useState<boolean>(false);
  const [specificNamespacesExpanded, setSpecificNamespacesExpanded] = useState<boolean>(false);
  const [specificDomainsExpanded, setSpecificDomainsExpanded] = useState<boolean>(false);
  const [specificMappingsExpanded, setSpecificMappingsExpanded] = useState<boolean>(false);

  const fetchContainers = useCallback(async (): Promise<void> => {
    if (containers !== null) return;
    setLoadingContainers(true);
    try {
      const res = await fetch(`${buildComputeBase()}/compute/containers`, {
        headers: getAuthHeaders(),
      });
      if (res.ok) {
        const data: { containers?: ContainerItem[] } = await res.json();
        setContainers(data.containers || []);
      } else {
        setContainers([]);
      }
    } catch {
      setContainers([]);
    } finally {
      setLoadingContainers(false);
    }
  }, [containers]);

  const fetchNamespaces = useCallback(async (): Promise<void> => {
    if (namespaces !== null) return;
    setLoadingNamespaces(true);
    try {
      const res = await fetch(`${buildStorageBase()}/storage/namespaces`, {
        headers: getAuthHeaders(),
      });
      if (res.ok) {
        const data: NamespaceItem[] = await res.json();
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

  const fetchDomainConns = useCallback(async (): Promise<void> => {
    if (domainConns !== null) return;
    setLoadingDomainConns(true);
    try {
      const res = await fetch(`${buildNetworkingBase()}/api/domains`, {
        headers: getAuthHeaders(),
      });
      if (res.ok) {
        const data: { connections?: DomainConnItem[] } = await res.json();
        setDomainConns(data.connections || []);
      } else {
        setDomainConns([]);
      }
    } catch {
      setDomainConns([]);
    } finally {
      setLoadingDomainConns(false);
    }
  }, [domainConns]);

  const fetchDomainMappings = useCallback(async (): Promise<void> => {
    if (domainMappings !== null) return;
    setLoadingDomainMappings(true);
    try {
      const res = await fetch(`${buildNetworkingBase()}/api/domain-mappings`, {
        headers: getAuthHeaders(),
      });
      if (res.ok) {
        const data: { domains?: DomainMappingItem[] } = await res.json();
        setDomainMappings(data.domains || []);
      } else {
        setDomainMappings([]);
      }
    } catch {
      setDomainMappings([]);
    } finally {
      setLoadingDomainMappings(false);
    }
  }, [domainMappings]);

  useEffect(() => {
    if (specificContainersExpanded) fetchContainers();
  }, [specificContainersExpanded, fetchContainers]);

  useEffect(() => {
    if (specificNamespacesExpanded) fetchNamespaces();
  }, [specificNamespacesExpanded, fetchNamespaces]);

  useEffect(() => {
    if (specificDomainsExpanded) fetchDomainConns();
  }, [specificDomainsExpanded, fetchDomainConns]);

  useEffect(() => {
    if (specificMappingsExpanded) fetchDomainMappings();
  }, [specificMappingsExpanded, fetchDomainMappings]);

  const toggleAction = (scopeKey: string, action: string): void => {
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

  const setAllActions = (scopeKey: string, actions: string[]): void => {
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
  const broadRegistryKey = `storage.${userId}.registry`;
  const broadDomainsKey = `networking.${userId}.domains`;
  const broadDomainMappingsKey = `networking.${userId}.domain-mappings`;
  const domainKey = (id: string): string => `networking.${userId}.domains.${id}`;
  const mappingKey = (id: string): string => `networking.${userId}.domain-mappings.${id}`;
  const containerKey = (id: string): string => `compute.${userId}.containers.${id}`;
  const nsNamespacesKey = (nsName: string): string => `storage.${userId}.namespaces.${nsName}`;
  const nsFilesKey = (nsName: string): string => `storage.${userId}.files.${nsName}`;

  return (
    <div className="space-y-3">
      <Label>Permissions</Label>
      <div className="space-y-4">
        {/* Compute section */}
        <div className="border border-border p-3 space-y-3">
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
                <div className="flex items-center gap-2 font-mono text-[11px] text-muted-foreground py-1">
                  <Loader2 className="w-3 h-3 animate-spin" />
                  Loading containers...
                </div>
              )}
              {containers !== null && containers.length === 0 && !loadingContainers && (
                <p className="font-mono text-[11px] text-muted-foreground">No containers found</p>
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
        <div className="border border-border p-3 space-y-3">
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
            <ResourceRow
              label="Registry"
              scopeKey={broadRegistryKey}
              actions={REGISTRY_ACTIONS}
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
                <div className="flex items-center gap-2 font-mono text-[11px] text-muted-foreground py-1">
                  <Loader2 className="w-3 h-3 animate-spin" />
                  Loading namespaces...
                </div>
              )}
              {namespaces !== null && namespaces.length === 0 && !loadingNamespaces && (
                <p className="font-mono text-[11px] text-muted-foreground">No namespaces found</p>
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

        {/* Networking section */}
        <div className="border border-border p-3 space-y-3">
          <SectionHeader
            label="Networking"
            expanded={networkingExpanded}
            onToggle={() => setNetworkingExpanded(!networkingExpanded)}
          >
            <ResourceRow
              label="All Domains"
              scopeKey={broadDomainsKey}
              actions={NETWORKING_ACTIONS}
              selectedScopes={selectedScopes}
              onToggle={toggleAction}
              onToggleAll={setAllActions}
            />
            <SectionHeader
              label="Specific Domains"
              expanded={specificDomainsExpanded}
              onToggle={() => setSpecificDomainsExpanded(!specificDomainsExpanded)}
            >
              {loadingDomainConns && (
                <div className="flex items-center gap-2 font-mono text-[11px] text-muted-foreground py-1">
                  <Loader2 className="w-3 h-3 animate-spin" />
                  Loading domains...
                </div>
              )}
              {domainConns !== null && domainConns.length === 0 && !loadingDomainConns && (
                <p className="font-mono text-[11px] text-muted-foreground">No domains found</p>
              )}
              {domainConns?.map((c) => (
                <ResourceRow
                  key={c.id}
                  label={c.zones.length > 0 ? c.zones.join(", ") : c.id.slice(0, 8)}
                  scopeKey={domainKey(c.id)}
                  actions={NETWORKING_ACTIONS}
                  selectedScopes={selectedScopes}
                  onToggle={toggleAction}
                  onToggleAll={setAllActions}
                />
              ))}
            </SectionHeader>
            <ResourceRow
              label="All Domain Mappings"
              scopeKey={broadDomainMappingsKey}
              actions={NETWORKING_ACTIONS}
              selectedScopes={selectedScopes}
              onToggle={toggleAction}
              onToggleAll={setAllActions}
            />
            <SectionHeader
              label="Specific Domain Mappings"
              expanded={specificMappingsExpanded}
              onToggle={() => setSpecificMappingsExpanded(!specificMappingsExpanded)}
            >
              {loadingDomainMappings && (
                <div className="flex items-center gap-2 font-mono text-[11px] text-muted-foreground py-1">
                  <Loader2 className="w-3 h-3 animate-spin" />
                  Loading domain mappings...
                </div>
              )}
              {domainMappings !== null && domainMappings.length === 0 && !loadingDomainMappings && (
                <p className="font-mono text-[11px] text-muted-foreground">No domain mappings found</p>
              )}
              {domainMappings?.map((m) => (
                <ResourceRow
                  key={m.id}
                  label={m.domain}
                  scopeKey={mappingKey(m.id)}
                  actions={NETWORKING_ACTIONS}
                  selectedScopes={selectedScopes}
                  onToggle={toggleAction}
                  onToggleAll={setAllActions}
                />
              ))}
            </SectionHeader>
          </SectionHeader>
        </div>
      </div>
    </div>
  );
}
