import { useState, useMemo, useEffect, useCallback } from "react";
import { Breadcrumb } from "@/components/ui/breadcrumb";
import { PageHeader } from "@/components/ui/page-header";
import { Skeleton } from "@/components/ui/skeleton";
import { Select } from "@/components/ui/select";
import { StatusChip } from "@/components/ui/status-chip";
import { Progress } from "@/components/ui/progress";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { useHealth } from "@/hooks";
import { useAuth } from "@/contexts/AuthContext";
import { formatBytes } from "@/lib/formatters";
import { HistoricalMetricsView, LogsView } from "@/components/observability";
import type { Pod } from "@/types";

/* ── Sort header: microlabel button with sort indicator ───────────────── */

interface SortHeaderProps {
  children: React.ReactNode;
  column: string;
  sortColumn: string | null;
  sortDir: string;
  onSort: (column: string) => void;
  className?: string;
}

function SortHeader({
  children,
  column,
  sortColumn,
  sortDir,
  onSort,
  className = "",
}: SortHeaderProps) {
  const isActive = sortColumn === column;
  return (
    <button
      onClick={() => onSort(column)}
      className={`microlabel flex items-center gap-1 hover:text-muted-foreground transition-colors ${className}`}
    >
      {children}
      {isActive && (
        <span className="text-[9px] text-primary">
          {sortDir === "asc" ? "▲" : "▼"}
        </span>
      )}
    </button>
  );
}

/* ── Tab definitions ──────────────────────────────────────────────────── */

// Historical tab queries /api/metrics/nodes which is admin-only.
// Non-admins only see realtime (pod metrics) and logs.
const ALL_TABS = [
  { id: "realtime",   label: "Real-time",  adminOnly: false },
  { id: "historical", label: "Historical", adminOnly: true  },
  { id: "logs",       label: "Logs",       adminOnly: false },
];

function getTabFromHash() {
  const hash = window.location.hash.slice(1);
  return ALL_TABS.find((t) => t.id === hash)?.id || "realtime";
}

/* ── Page ─────────────────────────────────────────────────────────────── */

export function HealthPage() {
  const { user, userId, isAdmin } = useAuth();
  const { health, podMetrics, loading, error, updateFrequency, setUpdateFrequency } =
    useHealth(user, true);

  const [showPercent, setShowPercent] = useState(false);
  const [nodeSort, setNodeSort] = useState<{ column: string | null; dir: "asc" | "desc" }>({
    column: null,
    dir: "desc",
  });
  const [podSort, setPodSort] = useState<{ column: string | null; dir: "asc" | "desc" }>({
    column: null,
    dir: "desc",
  });
  const [activeTab, setActiveTabState] = useState(getTabFromHash);

  /* Sync tab with URL hash for browser back/forward navigation */
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
        return nodeSort.dir === "asc"
          ? aVal.localeCompare(bVal as string)
          : (bVal as string).localeCompare(aVal);
      }
      return nodeSort.dir === "asc"
        ? (aVal as number) - (bVal as number)
        : (bVal as number) - (aVal as number);
    });
  }, [health.nodes, nodeSort]);

  /* Filter pods — admins see all; non-admins see only their own compute namespaces */
  const filteredPods = useMemo(() => {
    if (!podMetrics.pods) return [];
    return podMetrics.pods.filter((pod) => {
      const ns = pod.namespace || "";
      // Admins see everything (core + all compute namespaces)
      if (isAdmin) return true;
      // Non-admins: only their own compute-{userID}-* namespaces
      if (ns.startsWith("compute-") && userId) {
        return ns.startsWith(`compute-${userId}-`);
      }
      return false;
    });
  }, [podMetrics.pods, userId, isAdmin]);

  const sortPods = (pods: Pod[]) => {
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
        return podSort.dir === "asc"
          ? aVal.localeCompare(bVal as string)
          : (bVal as string).localeCompare(aVal);
      }
      return podSort.dir === "asc"
        ? (aVal as number) - (bVal as number)
        : (bVal as number) - (aVal as number);
    });
  };

  /* Ambient counts for the status strip */
  const healthyNodeCount = useMemo(
    () =>
      health.nodes.filter((n) =>
        (n.conditions || []).every((c) => c.status === "False")
      ).length,
    [health.nodes]
  );

  return (
    <div>
      <Breadcrumb items={[{ label: "Health" }]} />
      <PageHeader
        title="Health Monitor"
        description="Live telemetry for master connectivity and chunkserver status."
      />

      {/* ── Bare views tab switcher ──────────────────────────────────────── */}
      <nav className="flex items-baseline gap-6 pb-3 border-b border-border mb-6">
        {ALL_TABS.filter((tab) => !tab.adminOnly || isAdmin).map((tab) => (
          <button
            key={tab.id}
            onClick={() => setActiveTab(tab.id)}
            className={`font-mono text-[10.5px] uppercase tracking-[0.12em] transition-colors ${
              activeTab === tab.id
                ? "text-primary"
                : "text-faint hover:text-muted-foreground"
            }`}
          >
            {activeTab === tab.id && (
              <span className="mr-1" aria-hidden="true">›</span>
            )}
            {tab.label}
          </button>
        ))}
      </nav>

      {activeTab === "historical" && isAdmin && <HistoricalMetricsView />}

      {activeTab === "logs" && <LogsView />}

      {activeTab === "realtime" && (
        <>
          {/* Update frequency control */}
          <div className="flex items-center gap-3 mb-4">
            <span className="microlabel">Update Frequency</span>
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

          {/* ── Ambient status strip ──────────────────────────────────── */}
          {!loading && !error && (isAdmin ? health.nodes.length > 0 : filteredPods.length > 0) && (
            <div className="flex flex-wrap gap-x-6 gap-y-1 font-mono text-[10.5px] uppercase tracking-[0.12em] text-faint py-2.5 border-y border-line mb-5">
              {isAdmin && (
                <>
                  <span>
                    Nodes{" "}
                    <span className="text-muted-foreground">{health.nodes.length}</span>
                  </span>
                  <span>
                    Healthy{" "}
                    <span className="text-muted-foreground">{healthyNodeCount}</span>
                  </span>
                </>
              )}
              <span>
                Pods{" "}
                <span className="text-muted-foreground">{filteredPods.length}</span>
              </span>
            </div>
          )}

          {/* ── Cluster Nodes table — admin only ─────────────────────── */}
          {isAdmin && <Card>
            <CardHeader className="flex-row items-center justify-between space-y-0 pb-0">
              <CardTitle>Cluster Nodes</CardTitle>
              <button
                onClick={() => setShowPercent(!showPercent)}
                className="font-mono text-[10px] uppercase tracking-[0.12em] text-faint hover:text-muted-foreground transition-colors"
              >
                {showPercent ? "Show Absolute" : "Show Percent"}
              </button>
            </CardHeader>

            <CardContent className="pt-4">
              {loading ? (
                <div>
                  {/* Loading header */}
                  <div className="hidden md:grid grid-cols-[1.35fr_1fr_1.35fr_1.35fr_1fr] gap-4 px-4 py-2 border-b border-line">
                    {["Node", "Status", "CPU", "Memory", "Disk"].map((h) => (
                      <div key={h} className="microlabel text-center">{h}</div>
                    ))}
                  </div>
                  <div className="divide-y divide-line">
                    {[...Array(4)].map((_, i) => (
                      <div
                        key={i}
                        className="flex flex-col gap-2 px-4 py-3 md:grid md:grid-cols-[1.35fr_1fr_1.35fr_1.35fr_1fr] md:gap-4 md:items-center"
                      >
                        <Skeleton className="h-3.5 w-20 mx-auto" />
                        <Skeleton className="h-3.5 w-16 mx-auto" />
                        <Skeleton className="h-1.5 w-full" />
                        <Skeleton className="h-1.5 w-full" />
                        <Skeleton className="h-1.5 w-full" />
                      </div>
                    ))}
                  </div>
                </div>
              ) : error ? (
                <p className="font-mono text-xs text-destructive py-8 text-center">
                  {error}
                </p>
              ) : health.nodes.length === 0 ? (
                <p className="font-mono text-xs text-muted-foreground py-8 text-center">
                  {user ? "No nodes found" : "Log in to view cluster health"}
                </p>
              ) : (
                <div>
                  {/* Column headers — hidden on mobile */}
                  <div className="hidden md:grid grid-cols-[1.35fr_1fr_1.35fr_1.35fr_1fr] gap-4 px-4 py-2 border-b border-line">
                    <SortHeader
                      column="name"
                      sortColumn={nodeSort.column}
                      sortDir={nodeSort.dir}
                      onSort={handleNodeSort}
                      className="justify-center"
                    >
                      Node
                    </SortHeader>
                    <SortHeader
                      column="status"
                      sortColumn={nodeSort.column}
                      sortDir={nodeSort.dir}
                      onSort={handleNodeSort}
                      className="justify-center"
                    >
                      Status
                    </SortHeader>
                    <SortHeader
                      column="cpu"
                      sortColumn={nodeSort.column}
                      sortDir={nodeSort.dir}
                      onSort={handleNodeSort}
                      className="justify-center"
                    >
                      CPU
                    </SortHeader>
                    <SortHeader
                      column="memory"
                      sortColumn={nodeSort.column}
                      sortDir={nodeSort.dir}
                      onSort={handleNodeSort}
                      className="justify-center"
                    >
                      Memory
                    </SortHeader>
                    <SortHeader
                      column="disk"
                      sortColumn={nodeSort.column}
                      sortDir={nodeSort.dir}
                      onSort={handleNodeSort}
                      className="justify-center"
                    >
                      Disk
                    </SortHeader>
                  </div>

                  {/* Rows — flat, hairline dividers, hover popover wash */}
                  <div className="divide-y divide-line">
                    {sortedNodes.map((node, idx) => {
                      const conditions = node.conditions || [];
                      const isHealthy = conditions.every((c) => c.status === "False");

                      const cpuUsageStr = node.cpu_usage || "0";
                      const cpuCapStr = node.cpu_capacity || "0";
                      const cpuUsageMillis = cpuUsageStr.includes("n")
                        ? parseInt(cpuUsageStr) / 1000000
                        : cpuUsageStr.includes("m")
                        ? parseInt(cpuUsageStr)
                        : parseInt(cpuUsageStr) * 1000;
                      const cpuCapMillis = parseInt(cpuCapStr) * 1000;

                      const memUsage = parseKiBytes(node.memory_usage || "0");
                      const memCap = parseKiBytes(node.memory_capacity || "0");

                      return (
                        <div
                          key={node.name || idx}
                          className="flex flex-col gap-2 px-4 py-3 hover:bg-popover transition-colors md:grid md:grid-cols-[1.35fr_1fr_1.35fr_1.35fr_1fr] md:gap-4 md:items-center"
                        >
                          {/* Name + mobile status */}
                          <div className="flex items-center justify-between md:block md:text-center">
                            <span
                              className="font-mono text-[12.5px] text-muted-foreground truncate"
                              title={node.name}
                            >
                              {node.name}
                            </span>
                            <span className="md:hidden">
                              <StatusChip status={isHealthy ? "healthy" : "warning"} />
                            </span>
                          </div>

                          {/* Status — desktop only */}
                          <div className="hidden md:flex items-center justify-center">
                            <StatusChip status={isHealthy ? "healthy" : "warning"} />
                          </div>

                          {/* CPU */}
                          <div
                            className="space-y-1 text-center cursor-pointer"
                            onClick={() => setShowPercent(!showPercent)}
                          >
                            <span className="microlabel md:hidden">CPU</span>
                            <Progress value={node.cpu_percent || 0} className="h-1.5" />
                            <span className="font-mono text-[11px] text-muted-foreground tabular-nums">
                              {showPercent
                                ? `${(node.cpu_percent || 0).toFixed(1)}%`
                                : `${cpuUsageMillis.toFixed(0)}m / ${cpuCapMillis.toFixed(0)}m`}
                            </span>
                          </div>

                          {/* Memory */}
                          <div
                            className="space-y-1 text-center cursor-pointer"
                            onClick={() => setShowPercent(!showPercent)}
                          >
                            <span className="microlabel md:hidden">Memory</span>
                            <Progress value={node.memory_percent || 0} className="h-1.5" />
                            <span className="font-mono text-[11px] text-muted-foreground tabular-nums">
                              {showPercent
                                ? `${(node.memory_percent || 0).toFixed(1)}%`
                                : `${formatBytes(memUsage)} / ${formatBytes(memCap)}`}
                            </span>
                          </div>

                          {/* Disk */}
                          <div
                            className="space-y-1 text-center cursor-pointer"
                            onClick={() => setShowPercent(!showPercent)}
                          >
                            <span className="microlabel md:hidden">Disk</span>
                            <Progress value={node.disk_percent || 0} className="h-1.5" />
                            <span className="font-mono text-[11px] text-muted-foreground tabular-nums">
                              {showPercent
                                ? `${(node.disk_percent || 0).toFixed(1)}%`
                                : `${formatBytes(node.disk_usage || 0)} / ${formatBytes(node.disk_capacity || 0)}`}
                            </span>
                          </div>
                        </div>
                      );
                    })}
                  </div>
                </div>
              )}
            </CardContent>
          </Card>}

          {/* ── Pod Metrics table ────────────────────────────────────────── */}
          <Card className="mt-5">
            <CardHeader className="flex-row items-center justify-between space-y-0 pb-0">
              <CardTitle>Pod Metrics</CardTitle>
              <button
                onClick={() => setShowPercent(!showPercent)}
                className="font-mono text-[10px] uppercase tracking-[0.12em] text-faint hover:text-muted-foreground transition-colors"
              >
                {showPercent ? "Show Absolute" : "Show Percent"}
              </button>
            </CardHeader>

            <CardContent className="pt-4">
              {loading ? (
                <div>
                  <div className="hidden md:grid grid-cols-[2fr_1.2fr_1.2fr_1fr] gap-4 px-4 py-2 border-b border-line">
                    {["Pod", "CPU", "Memory", "Disk"].map((h) => (
                      <div key={h} className="microlabel">{h}</div>
                    ))}
                  </div>
                  <div className="divide-y divide-line">
                    {[...Array(5)].map((_, i) => (
                      <div
                        key={i}
                        className="flex flex-col gap-2 px-4 py-3 md:grid md:grid-cols-[2fr_1.2fr_1.2fr_1fr] md:gap-4 md:items-center"
                      >
                        <Skeleton className="h-3.5 w-32" />
                        <Skeleton className="h-1.5 w-full" />
                        <Skeleton className="h-1.5 w-full" />
                        <Skeleton className="h-1.5 w-full" />
                      </div>
                    ))}
                  </div>
                </div>
              ) : filteredPods.length === 0 ? (
                <p className="font-mono text-xs text-muted-foreground py-8 text-center">
                  {user ? "No pods found" : "Log in to view pod metrics"}
                </p>
              ) : (
                <div className="space-y-5">
                  {/* Group pods by node, sorted alphabetically */}
                  {Object.entries(
                    filteredPods.reduce<Record<string, Pod[]>>((acc, pod) => {
                      const node = pod.node || "Unknown";
                      if (!acc[node]) acc[node] = [];
                      acc[node].push(pod);
                      return acc;
                    }, {})
                  )
                    .sort(([a], [b]) => a.localeCompare(b))
                    .map(([nodeName, pods]) => (
                      <div key={nodeName}>
                        {/* Node group header — ice left-tick, flat */}
                        <div className="pl-3 border-l-2 border-primary mb-2 flex items-baseline gap-3">
                          <span className="font-mono text-[12.5px] text-foreground">
                            {nodeName}
                          </span>
                          <span className="microlabel">{pods.length} pods</span>
                        </div>

                        {/* Column header — hidden on mobile */}
                        <div className="hidden md:grid grid-cols-[2fr_1.2fr_1.2fr_1fr] gap-4 px-4 py-2 border-b border-line mb-0">
                          <SortHeader
                            column="name"
                            sortColumn={podSort.column}
                            sortDir={podSort.dir}
                            onSort={handlePodSort}
                            className="justify-start"
                          >
                            Pod
                          </SortHeader>
                          <SortHeader
                            column="cpu"
                            sortColumn={podSort.column}
                            sortDir={podSort.dir}
                            onSort={handlePodSort}
                            className="justify-center"
                          >
                            CPU
                          </SortHeader>
                          <SortHeader
                            column="memory"
                            sortColumn={podSort.column}
                            sortDir={podSort.dir}
                            onSort={handlePodSort}
                            className="justify-center"
                          >
                            Memory
                          </SortHeader>
                          <SortHeader
                            column="disk"
                            sortColumn={podSort.column}
                            sortDir={podSort.dir}
                            onSort={handlePodSort}
                            className="justify-center"
                          >
                            Disk
                          </SortHeader>
                        </div>

                        {/* Pod rows */}
                        <div className="divide-y divide-line">
                          {sortPods(pods).map((pod, idx) => {
                            const cpuUsageMillis = (pod.cpu_usage || 0) / 1000000;
                            const cpuCapMillis = (pod.cpu_capacity || 0) / 1000000;
                            const cpuPercent =
                              cpuCapMillis > 0
                                ? (cpuUsageMillis / cpuCapMillis) * 100
                                : 0;

                            const memUsage = pod.memory_usage || 0;
                            const memCap = pod.memory_capacity || 0;
                            const memPercent =
                              memCap > 0 ? (memUsage / memCap) * 100 : 0;

                            const diskUsage = pod.disk_usage || 0;
                            const diskCap = pod.disk_capacity || 0;
                            const diskPercent =
                              diskCap > 0 ? (diskUsage / diskCap) * 100 : 0;

                            return (
                              <div
                                key={pod.name || idx}
                                className="flex flex-col gap-2 px-4 py-3 hover:bg-popover transition-colors md:grid md:grid-cols-[2fr_1.2fr_1.2fr_1fr] md:gap-4 md:items-center"
                              >
                                {/* Pod name */}
                                <span
                                  className="font-mono text-[12.5px] text-muted-foreground truncate"
                                  title={pod.name}
                                >
                                  {pod.name}
                                </span>

                                {/* CPU */}
                                <div
                                  className="space-y-1 text-center cursor-pointer"
                                  onClick={() => setShowPercent(!showPercent)}
                                >
                                  <span className="microlabel md:hidden">CPU</span>
                                  <Progress value={cpuPercent} className="h-1.5" />
                                  <span className="font-mono text-[11px] text-muted-foreground tabular-nums">
                                    {showPercent
                                      ? `${cpuPercent.toFixed(1)}%`
                                      : `${cpuUsageMillis.toFixed(0)}m / ${cpuCapMillis.toFixed(0)}m`}
                                  </span>
                                </div>

                                {/* Memory */}
                                <div
                                  className="space-y-1 text-center cursor-pointer"
                                  onClick={() => setShowPercent(!showPercent)}
                                >
                                  <span className="microlabel md:hidden">Memory</span>
                                  <Progress value={memPercent} className="h-1.5" />
                                  <span className="font-mono text-[11px] text-muted-foreground tabular-nums">
                                    {showPercent
                                      ? `${memPercent.toFixed(1)}%`
                                      : `${formatBytes(memUsage)} / ${formatBytes(memCap)}`}
                                  </span>
                                </div>

                                {/* Disk */}
                                <div
                                  className="space-y-1 text-center cursor-pointer"
                                  onClick={() => setShowPercent(!showPercent)}
                                >
                                  <span className="microlabel md:hidden">Disk</span>
                                  <Progress value={diskPercent} className="h-1.5" />
                                  <span className="font-mono text-[11px] text-muted-foreground tabular-nums">
                                    {showPercent
                                      ? `${diskPercent.toFixed(1)}%`
                                      : `${formatBytes(diskUsage)} / ${formatBytes(diskCap)}`}
                                  </span>
                                </div>
                              </div>
                            );
                          })}
                        </div>
                      </div>
                    ))}
                </div>
              )}
            </CardContent>
          </Card>
        </>
      )}
    </div>
  );
}
