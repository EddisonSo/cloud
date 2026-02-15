import { useState, useMemo, useEffect, useCallback } from "react";
import { Breadcrumb } from "@/components/ui/breadcrumb";
import { PageHeader } from "@/components/ui/page-header";
import { Skeleton } from "@/components/ui/skeleton";
import { Select } from "@/components/ui/select";
import { StatusChip } from "@/components/ui/status-chip";
import { Progress } from "@/components/ui/progress";
import { useHealth } from "@/hooks";
import { useAuth } from "@/contexts/AuthContext";
import { formatBytes } from "@/lib/formatters";
import { HistoricalMetricsView, LogsView } from "@/components/observability";
import type { Pod } from "@/types";

interface SortHeaderProps {
  children: React.ReactNode;
  column: string;
  sortColumn: string | null;
  sortDir: string;
  onSort: (column: string) => void;
  className?: string;
}

function SortHeader({ children, column, sortColumn, sortDir, onSort, className = "" }: SortHeaderProps) {
  const isActive = sortColumn === column;
  return (
    <button
      onClick={() => onSort(column)}
      className={`flex items-center justify-center gap-1 hover:text-foreground transition-colors ${className}`}
    >
      {children}
      <span className="text-[10px]">
        {isActive ? (sortDir === "asc" ? "▲" : "▼") : ""}
      </span>
    </button>
  );
}

const tabs = [
  { id: "realtime", label: "Real-time" },
  { id: "historical", label: "Historical" },
  { id: "logs", label: "Logs" },
];

function getTabFromHash() {
  const hash = window.location.hash.slice(1);
  return tabs.find((t) => t.id === hash)?.id || "realtime";
}

export function HealthPage() {
  const { user, userId } = useAuth();
  const { health, podMetrics, loading, error, updateFrequency, setUpdateFrequency } = useHealth(user, true);
  const [showPercent, setShowPercent] = useState(false);
  const [nodeSort, setNodeSort] = useState<{ column: string | null; dir: "asc" | "desc" }>({ column: null, dir: "desc" });
  const [podSort, setPodSort] = useState<{ column: string | null; dir: "asc" | "desc" }>({ column: null, dir: "desc" });
  const [activeTab, setActiveTabState] = useState(getTabFromHash);

  // Sync tab with URL hash for browser back/forward navigation
  const setActiveTab = useCallback((tabId: string) => {
    setActiveTabState(tabId);
    const newHash = tabId === "realtime" ? "" : `#${tabId}`;
    if (window.location.hash !== newHash) {
      window.history.pushState(null, "", newHash || window.location.pathname);
    }
  }, []);

  useEffect(() => {
    const handlePopState = () => {
      setActiveTabState(getTabFromHash());
    };
    window.addEventListener("popstate", handlePopState);
    return () => window.removeEventListener("popstate", handlePopState);
  }, []);


  function parseKiBytes(str: string): number {
    if (!str) return 0;
    const num = parseInt(str.replace(/[^0-9]/g, ""), 10);
    if (str.includes("Ki")) return num * 1024;
    if (str.includes("Mi")) return num * 1024 * 1024;
    if (str.includes("Gi")) return num * 1024 * 1024 * 1024;
    return num;
  }

  const handleNodeSort = (column: string) => {
    setNodeSort((prev) => ({
      column,
      dir: prev.column === column && prev.dir === "desc" ? "asc" : "desc",
    }));
  };

  const handlePodSort = (column: string) => {
    setPodSort((prev) => ({
      column,
      dir: prev.column === column && prev.dir === "desc" ? "asc" : "desc",
    }));
  };

  const sortedNodes = useMemo(() => {
    if (!nodeSort.column) return health.nodes;
    return [...health.nodes].sort((a, b) => {
      let aVal: string | number, bVal: string | number;
      switch (nodeSort.column) {
        case "name":
          aVal = a.name || "";
          bVal = b.name || "";
          break;
        case "status":
          aVal = (a.conditions || []).every((c) => c.status === "False") ? 1 : 0;
          bVal = (b.conditions || []).every((c) => c.status === "False") ? 1 : 0;
          break;
        case "cpu":
          aVal = a.cpu_percent || 0;
          bVal = b.cpu_percent || 0;
          break;
        case "memory":
          aVal = a.memory_percent || 0;
          bVal = b.memory_percent || 0;
          break;
        case "disk":
          aVal = a.disk_percent || 0;
          bVal = b.disk_percent || 0;
          break;
        default:
          return 0;
      }
      if (typeof aVal === "string") {
        return nodeSort.dir === "asc" ? aVal.localeCompare(bVal as string) : (bVal as string).localeCompare(aVal);
      }
      return nodeSort.dir === "asc" ? (aVal as number) - (bVal as number) : (bVal as number) - (aVal as number);
    });
  }, [health.nodes, nodeSort]);

  // Filter pods to only show user's own compute containers
  const filteredPods = useMemo(() => {
    if (!podMetrics.pods) return [];
    return podMetrics.pods.filter((pod) => {
      const ns = pod.namespace || "core";
      // Show all pods in core namespace (core services)
      if (ns === "core") return true;
      // For compute-* namespaces, only show user's own containers
      // Namespace format: compute-{user_id}-{container_id}
      if (ns.startsWith("compute-") && userId) {
        return ns.startsWith(`compute-${userId}-`);
      }
      // Hide other users' compute containers
      return false;
    });
  }, [podMetrics.pods, userId]);

  const sortPods = (pods: Pod[]) => {
    // Default to sorting by name lexicographically
    if (!podSort.column) {
      return [...pods].sort((a, b) => (a.name || "").localeCompare(b.name || ""));
    }
    return [...pods].sort((a, b) => {
      let aVal: string | number, bVal: string | number;
      switch (podSort.column) {
        case "name":
          aVal = a.name || "";
          bVal = b.name || "";
          break;
        case "cpu":
          aVal = a.cpu_usage || 0;
          bVal = b.cpu_usage || 0;
          break;
        case "memory":
          aVal = a.memory_usage || 0;
          bVal = b.memory_usage || 0;
          break;
        case "disk":
          aVal = a.disk_usage || 0;
          bVal = b.disk_usage || 0;
          break;
        default:
          return 0;
      }
      if (typeof aVal === "string") {
        return podSort.dir === "asc" ? aVal.localeCompare(bVal as string) : (bVal as string).localeCompare(aVal);
      }
      return podSort.dir === "asc" ? (aVal as number) - (bVal as number) : (bVal as number) - (aVal as number);
    });
  };

  return (
    <div>
      <Breadcrumb items={[{ label: "Health" }]} />
      <PageHeader title="Health Monitor" description="Live telemetry for master connectivity and chunkserver status." />

      {/* Tab Navigation */}
      <div className="border-b border-border mb-6">
        <nav className="flex gap-6">
          {tabs.map((tab) => (
            <button
              key={tab.id}
              onClick={() => setActiveTab(tab.id)}
              className={`pb-3 text-sm font-medium transition-colors relative ${
                activeTab === tab.id
                  ? "text-foreground"
                  : "text-muted-foreground hover:text-foreground"
              }`}
            >
              {tab.label}
              {activeTab === tab.id && (
                <span className="absolute bottom-0 left-0 right-0 h-0.5 bg-primary" />
              )}
            </button>
          ))}
        </nav>
      </div>

      {activeTab === "historical" && <HistoricalMetricsView />}

      {activeTab === "logs" && <LogsView />}

      {activeTab === "realtime" && (
        <>
          {/* Update Frequency */}
          <div className="flex items-center gap-2 mb-4">
            <span className="text-sm text-muted-foreground">Update frequency:</span>
            <Select
              value={updateFrequency}
              onChange={(e) => setUpdateFrequency(Number(e.target.value))}
              className="w-32"
            >
              <option value={0}>Real-time</option>
              <option value={500}>0.5s</option>
              <option value={1000}>1s</option>
              <option value={5000}>5s</option>
              <option value={30000}>30s</option>
              <option value={60000}>1 min</option>
            </Select>
          </div>

          {/* Node Table */}
      <div className="bg-card border border-border rounded-lg">
        <div className="px-5 py-4 border-b border-border flex items-center justify-between">
          <h2 className="text-sm font-semibold">Cluster Nodes</h2>
          <button
            onClick={() => setShowPercent(!showPercent)}
            className="text-xs text-muted-foreground hover:text-foreground transition-colors"
          >
            {showPercent ? "Show absolute" : "Show percent"}
          </button>
        </div>
        <div className="p-5">
          {loading ? (
            <div className="space-y-2">
              <div className="grid grid-cols-[1.35fr_1fr_1.35fr_1.35fr_1fr] gap-4 px-4 py-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                <div className="text-center">Node</div>
                <div className="text-center">Status</div>
                <div className="text-center">CPU</div>
                <div className="text-center">Memory</div>
                <div className="text-center">Disk</div>
              </div>
              {[...Array(4)].map((_, i) => (
                <div key={i} className="grid grid-cols-[1.35fr_1fr_1.35fr_1.35fr_1fr] gap-4 px-4 py-3 bg-secondary rounded-md items-center">
                  <Skeleton className="h-5 w-20 mx-auto" />
                  <Skeleton className="h-5 w-16 mx-auto" />
                  <Skeleton className="h-2 w-full" />
                  <Skeleton className="h-2 w-full" />
                  <Skeleton className="h-2 w-full" />
                </div>
              ))}
            </div>
          ) : error ? (
            <p className="text-destructive py-8 text-center">{error}</p>
          ) : health.nodes.length === 0 ? (
            <p className="text-muted-foreground py-8 text-center">
              {user ? "No nodes found" : "Log in to view cluster health"}
            </p>
          ) : (
            <div className="space-y-2">
              {/* Header */}
              <div className="grid grid-cols-[1.35fr_1fr_1.35fr_1.35fr_1fr] gap-4 px-4 py-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                <SortHeader column="name" sortColumn={nodeSort.column} sortDir={nodeSort.dir} onSort={handleNodeSort}>
                  Node
                </SortHeader>
                <SortHeader column="status" sortColumn={nodeSort.column} sortDir={nodeSort.dir} onSort={handleNodeSort}>
                  Status
                </SortHeader>
                <SortHeader column="cpu" sortColumn={nodeSort.column} sortDir={nodeSort.dir} onSort={handleNodeSort}>
                  CPU
                </SortHeader>
                <SortHeader column="memory" sortColumn={nodeSort.column} sortDir={nodeSort.dir} onSort={handleNodeSort}>
                  Memory
                </SortHeader>
                <SortHeader column="disk" sortColumn={nodeSort.column} sortDir={nodeSort.dir} onSort={handleNodeSort}>
                  Disk
                </SortHeader>
              </div>
              {/* Rows */}
              {sortedNodes.map((node, idx) => {
                const conditions = node.conditions || [];
                const isHealthy = conditions.every((c) => c.status === "False");

                // Parse CPU values (cpu_usage is like "500m", cpu_capacity is like "4")
                const cpuUsageStr = node.cpu_usage || "0";
                const cpuCapStr = node.cpu_capacity || "0";
                const cpuUsageMillis = cpuUsageStr.includes("n")
                  ? parseInt(cpuUsageStr) / 1000000
                  : cpuUsageStr.includes("m")
                  ? parseInt(cpuUsageStr)
                  : parseInt(cpuUsageStr) * 1000;
                const cpuCapMillis = parseInt(cpuCapStr) * 1000;

                // Parse memory values
                const memUsage = parseKiBytes(node.memory_usage || "0");
                const memCap = parseKiBytes(node.memory_capacity || "0");

                return (
                  <div
                    key={node.name || idx}
                    className="grid grid-cols-[1.35fr_1fr_1.35fr_1.35fr_1fr] gap-4 px-4 py-3 bg-secondary rounded-md items-center"
                  >
                    <div className="font-medium truncate text-center" title={node.name}>
                      {node.name}
                    </div>
                    <div className="flex items-center justify-center gap-2">
                      <StatusChip status={isHealthy ? "healthy" : "warning"} />
                    </div>
                    <div
                      className="space-y-1 text-center cursor-pointer"
                      onClick={() => setShowPercent(!showPercent)}
                    >
                      <Progress value={node.cpu_percent || 0} className="h-2" />
                      <span className="text-xs text-muted-foreground">
                        {showPercent
                          ? `${(node.cpu_percent || 0).toFixed(1)}%`
                          : `${cpuUsageMillis.toFixed(0)}m / ${cpuCapMillis.toFixed(0)}m`}
                      </span>
                    </div>
                    <div
                      className="space-y-1 text-center cursor-pointer"
                      onClick={() => setShowPercent(!showPercent)}
                    >
                      <Progress value={node.memory_percent || 0} className="h-2" />
                      <span className="text-xs text-muted-foreground">
                        {showPercent
                          ? `${(node.memory_percent || 0).toFixed(1)}%`
                          : `${formatBytes(memUsage)} / ${formatBytes(memCap)}`}
                      </span>
                    </div>
                    <div
                      className="space-y-1 text-center cursor-pointer"
                      onClick={() => setShowPercent(!showPercent)}
                    >
                      <Progress value={node.disk_percent || 0} className="h-2" />
                      <span className="text-xs text-muted-foreground">
                        {showPercent
                          ? `${(node.disk_percent || 0).toFixed(1)}%`
                          : `${formatBytes(node.disk_usage || 0)} / ${formatBytes(node.disk_capacity || 0)}`}
                      </span>
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </div>
      </div>

      {/* Pod Metrics Table */}
      <div className="bg-card border border-border rounded-lg mt-6">
        <div className="px-5 py-4 border-b border-border flex items-center justify-between">
          <h2 className="text-sm font-semibold">Pod Metrics</h2>
          <button
            onClick={() => setShowPercent(!showPercent)}
            className="text-xs text-muted-foreground hover:text-foreground transition-colors"
          >
            {showPercent ? "Show absolute" : "Show percent"}
          </button>
        </div>
        <div className="p-5">
          {loading ? (
            <div className="space-y-2">
              <div className="grid grid-cols-[2fr_1.2fr_1.2fr_1fr] gap-4 px-4 py-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                <div>Pod</div>
                <div className="text-center">CPU</div>
                <div className="text-center">Memory</div>
                <div className="text-center">Disk</div>
              </div>
              {[...Array(5)].map((_, i) => (
                <div key={i} className="grid grid-cols-[2fr_1.2fr_1.2fr_1fr] gap-4 px-4 py-3 bg-secondary rounded-md items-center">
                  <Skeleton className="h-5 w-32" />
                  <Skeleton className="h-2 w-full" />
                  <Skeleton className="h-2 w-full" />
                  <Skeleton className="h-2 w-full" />
                </div>
              ))}
            </div>
          ) : filteredPods.length === 0 ? (
            <p className="text-muted-foreground py-8 text-center">
              {user ? "No pods found" : "Log in to view pod metrics"}
            </p>
          ) : (
            <div className="space-y-4">
              {/* Group pods by node, sorted alphabetically */}
              {Object.entries(
                filteredPods.reduce<Record<string, Pod[]>>((acc, pod) => {
                  const node = pod.node || "Unknown";
                  if (!acc[node]) acc[node] = [];
                  acc[node].push(pod);
                  return acc;
                }, {})
              ).sort(([a], [b]) => a.localeCompare(b)).map(([nodeName, pods]) => (
                <div key={nodeName} className="space-y-2">
                  {/* Node Header */}
                  <div className="px-4 py-3 bg-primary/10 border-l-4 border-primary rounded-md">
                    <span className="text-base font-bold">{nodeName}</span>
                    <span className="text-sm text-muted-foreground ml-2">({pods.length} pods)</span>
                  </div>
                  {/* Column Header */}
                  <div className="grid grid-cols-[2fr_1.2fr_1.2fr_1fr] gap-4 px-4 py-1 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                    <SortHeader column="name" sortColumn={podSort.column} sortDir={podSort.dir} onSort={handlePodSort} className="justify-start">
                      Pod
                    </SortHeader>
                    <SortHeader column="cpu" sortColumn={podSort.column} sortDir={podSort.dir} onSort={handlePodSort}>
                      CPU
                    </SortHeader>
                    <SortHeader column="memory" sortColumn={podSort.column} sortDir={podSort.dir} onSort={handlePodSort}>
                      Memory
                    </SortHeader>
                    <SortHeader column="disk" sortColumn={podSort.column} sortDir={podSort.dir} onSort={handlePodSort}>
                      Disk
                    </SortHeader>
                  </div>
                  {/* Pod Rows */}
                  {sortPods(pods).map((pod, idx) => {
                    const cpuUsageMillis = (pod.cpu_usage || 0) / 1000000;
                    const cpuCapMillis = (pod.cpu_capacity || 0) / 1000000;
                    const cpuPercent = cpuCapMillis > 0 ? (cpuUsageMillis / cpuCapMillis) * 100 : 0;

                    const memUsage = pod.memory_usage || 0;
                    const memCap = pod.memory_capacity || 0;
                    const memPercent = memCap > 0 ? (memUsage / memCap) * 100 : 0;

                    const diskUsage = pod.disk_usage || 0;
                    const diskCap = pod.disk_capacity || 0;
                    const diskPercent = diskCap > 0 ? (diskUsage / diskCap) * 100 : 0;

                    return (
                      <div
                        key={pod.name || idx}
                        className="grid grid-cols-[2fr_1.2fr_1.2fr_1fr] gap-4 px-4 py-3 bg-secondary rounded-md items-center"
                      >
                        <div className="font-semibold text-foreground truncate" title={pod.name}>
                          {pod.name}
                        </div>
                        <div
                          className="space-y-1 text-center cursor-pointer"
                          onClick={() => setShowPercent(!showPercent)}
                        >
                          <Progress value={cpuPercent} className="h-2" />
                          <span className="text-xs text-muted-foreground">
                            {showPercent
                              ? `${cpuPercent.toFixed(1)}%`
                              : `${cpuUsageMillis.toFixed(0)}m / ${cpuCapMillis.toFixed(0)}m`}
                          </span>
                        </div>
                        <div
                          className="space-y-1 text-center cursor-pointer"
                          onClick={() => setShowPercent(!showPercent)}
                        >
                          <Progress value={memPercent} className="h-2" />
                          <span className="text-xs text-muted-foreground">
                            {showPercent
                              ? `${memPercent.toFixed(1)}%`
                              : `${formatBytes(memUsage)} / ${formatBytes(memCap)}`}
                          </span>
                        </div>
                        <div
                          className="space-y-1 text-center cursor-pointer"
                          onClick={() => setShowPercent(!showPercent)}
                        >
                          <Progress value={diskPercent} className="h-2" />
                          <span className="text-xs text-muted-foreground">
                            {showPercent
                              ? `${diskPercent.toFixed(1)}%`
                              : `${formatBytes(diskUsage)} / ${formatBytes(diskCap)}`}
                          </span>
                        </div>
                      </div>
                    );
                  })}
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
        </>
      )}
    </div>
  );
}
