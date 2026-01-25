import { useState, useMemo } from "react";
import { Header } from "@/components/layout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton, TextSkeleton } from "@/components/ui/skeleton";
import { Select } from "@/components/ui/select";
import { StatusDot } from "@/components/common";
import { Progress } from "@/components/ui/progress";
import { TAB_COPY } from "@/lib/constants";
import { useHealth } from "@/hooks";
import { useAuth } from "@/contexts/AuthContext";
import { formatBytes } from "@/lib/formatters";
import { HistoricalMetricsView, ServiceDependencyGraph } from "@/components/observability";

function SortHeader({ children, column, sortColumn, sortDir, onSort, className = "" }) {
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
  { id: "servicemap", label: "Service Map" },
];

export function HealthPage() {
  const copy = TAB_COPY.health;
  const { user } = useAuth();
  const { health, podMetrics, loading, error, lastCheck, updateFrequency, setUpdateFrequency } = useHealth(user, true);
  const [showPercent, setShowPercent] = useState(false);
  const [nodeSort, setNodeSort] = useState({ column: null, dir: "desc" });
  const [podSort, setPodSort] = useState({ column: null, dir: "desc" });
  const [activeTab, setActiveTab] = useState("realtime");

  const totalNodes = health.nodes.length;
  const healthyNodes = health.nodes.filter((n) => {
    const conditions = n.conditions || [];
    return conditions.every((c) => c.status === "False");
  }).length;

  const totalDisk = health.nodes.reduce((sum, n) => sum + (n.disk_capacity || 0), 0);
  const totalMemory = health.nodes.reduce((sum, n) => {
    const cap = n.memory_capacity || "0";
    return sum + parseKiBytes(cap);
  }, 0);

  function parseKiBytes(str) {
    if (!str) return 0;
    const num = parseInt(str.replace(/[^0-9]/g, ""), 10);
    if (str.includes("Ki")) return num * 1024;
    if (str.includes("Mi")) return num * 1024 * 1024;
    if (str.includes("Gi")) return num * 1024 * 1024 * 1024;
    return num;
  }

  const handleNodeSort = (column) => {
    setNodeSort((prev) => ({
      column,
      dir: prev.column === column && prev.dir === "desc" ? "asc" : "desc",
    }));
  };

  const handlePodSort = (column) => {
    setPodSort((prev) => ({
      column,
      dir: prev.column === column && prev.dir === "desc" ? "asc" : "desc",
    }));
  };

  const sortedNodes = useMemo(() => {
    if (!nodeSort.column) return health.nodes;
    return [...health.nodes].sort((a, b) => {
      let aVal, bVal;
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
        return nodeSort.dir === "asc" ? aVal.localeCompare(bVal) : bVal.localeCompare(aVal);
      }
      return nodeSort.dir === "asc" ? aVal - bVal : bVal - aVal;
    });
  }, [health.nodes, nodeSort]);

  // Filter pods to only show user's own compute containers
  const filteredPods = useMemo(() => {
    if (!podMetrics.pods) return [];
    return podMetrics.pods.filter((pod) => {
      const ns = pod.namespace || "default";
      // Show all pods in default namespace (core services)
      if (ns === "default") return true;
      // For compute-* namespaces, only show user's own containers
      // Namespace format: compute-{user_id}-{container_id}
      if (ns.startsWith("compute-") && user?.id) {
        return ns.startsWith(`compute-${user.id}-`);
      }
      // Hide other users' compute containers
      return false;
    });
  }, [podMetrics.pods, user?.id]);

  const sortPods = (pods) => {
    // Default to sorting by name lexicographically
    if (!podSort.column) {
      return [...pods].sort((a, b) => (a.name || "").localeCompare(b.name || ""));
    }
    return [...pods].sort((a, b) => {
      let aVal, bVal;
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
        return podSort.dir === "asc" ? aVal.localeCompare(bVal) : bVal.localeCompare(aVal);
      }
      return podSort.dir === "asc" ? aVal - bVal : bVal - aVal;
    });
  };

  return (
    <div>
      <Header eyebrow={copy.eyebrow} title={copy.title} description={copy.lead} />

      {/* Summary Cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
        <Card className="min-w-0">
          <CardContent className="pt-6">
            <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground mb-1 truncate">
              Cluster Status
            </p>
            {loading ? (
              <div className="flex items-center gap-2">
                <StatusDot status="ok" />
                <TextSkeleton text="Healthy" className="text-2xl font-semibold" />
              </div>
            ) : (
              <div className="flex items-center gap-2">
                <StatusDot status={health.cluster_ok ? "ok" : "down"} />
                <span className="text-2xl font-semibold truncate">
                  {health.cluster_ok ? "Healthy" : "Degraded"}
                </span>
              </div>
            )}
          </CardContent>
        </Card>
        <Card className="min-w-0">
          <CardContent className="pt-6">
            <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground mb-1 truncate">
              Nodes Online
            </p>
            {loading ? (
              <TextSkeleton text="0 / 0" className="text-2xl font-semibold" />
            ) : (
              <span className="text-2xl font-semibold">
                {healthyNodes} / {totalNodes}
              </span>
            )}
          </CardContent>
        </Card>
        <Card className="min-w-0">
          <CardContent className="pt-6">
            <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground mb-1 truncate">
              Total Memory
            </p>
            {loading ? (
              <TextSkeleton text="0.00 GB" className="text-2xl font-semibold" />
            ) : (
              <span className="text-2xl font-semibold truncate block">
                {formatBytes(totalMemory)}
              </span>
            )}
          </CardContent>
        </Card>
        <Card className="min-w-0">
          <CardContent className="pt-6">
            <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground mb-1 truncate">
              Last Updated
            </p>
            {loading ? (
              <TextSkeleton text="00:00:00" className="text-lg font-medium text-muted-foreground" />
            ) : (
              <span className="text-lg font-medium text-muted-foreground truncate block">
                {lastCheck ? lastCheck.toLocaleTimeString() : "—"}
              </span>
            )}
          </CardContent>
        </Card>
      </div>

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

      {activeTab === "servicemap" && <ServiceDependencyGraph healthData={health} />}

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
      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <CardTitle>Cluster Nodes</CardTitle>
          <button
            onClick={() => setShowPercent(!showPercent)}
            className="text-xs text-muted-foreground hover:text-foreground transition-colors"
          >
            {showPercent ? "Show absolute" : "Show percent"}
          </button>
        </CardHeader>
        <CardContent>
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
                      <StatusDot status={isHealthy ? "ok" : "warning"} />
                      <span className="text-sm">{isHealthy ? "Healthy" : "Pressure"}</span>
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
        </CardContent>
      </Card>

      {/* Pod Metrics Table */}
      <Card className="mt-6">
        <CardHeader className="flex flex-row items-center justify-between">
          <CardTitle>Pod Metrics</CardTitle>
          <button
            onClick={() => setShowPercent(!showPercent)}
            className="text-xs text-muted-foreground hover:text-foreground transition-colors"
          >
            {showPercent ? "Show absolute" : "Show percent"}
          </button>
        </CardHeader>
        <CardContent>
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
                filteredPods.reduce((acc, pod) => {
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
        </CardContent>
      </Card>
        </>
      )}
    </div>
  );
}
