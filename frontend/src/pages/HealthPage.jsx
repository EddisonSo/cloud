import { Header } from "@/components/layout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { StatusDot } from "@/components/common";
import { Progress } from "@/components/ui/progress";
import { TAB_COPY } from "@/lib/constants";
import { useHealth } from "@/hooks";
import { useAuth } from "@/contexts/AuthContext";
import { formatBytes } from "@/lib/formatters";

export function HealthPage() {
  const copy = TAB_COPY.health;
  const { user } = useAuth();
  const { health, loading, error, lastCheck } = useHealth(user, true);

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

  return (
    <div>
      <Header eyebrow={copy.eyebrow} title={copy.title} description={copy.lead} />

      {/* Summary Cards */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4 mb-6">
        <Card>
          <CardContent className="pt-6">
            <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground mb-1">
              Cluster Status
            </p>
            {loading ? (
              <Skeleton className="h-8 w-24" />
            ) : (
              <div className="flex items-center gap-2">
                <StatusDot status={health.cluster_ok ? "ok" : "down"} />
                <span className="text-2xl font-semibold">
                  {health.cluster_ok ? "Healthy" : "Degraded"}
                </span>
              </div>
            )}
          </CardContent>
        </Card>
        <Card>
          <CardContent className="pt-6">
            <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground mb-1">
              Nodes Online
            </p>
            {loading ? (
              <Skeleton className="h-8 w-16" />
            ) : (
              <span className="text-2xl font-semibold">
                {healthyNodes} / {totalNodes}
              </span>
            )}
          </CardContent>
        </Card>
        <Card>
          <CardContent className="pt-6">
            <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground mb-1">
              Total Memory
            </p>
            {loading ? (
              <Skeleton className="h-8 w-20" />
            ) : (
              <span className="text-2xl font-semibold">
                {formatBytes(totalMemory)}
              </span>
            )}
          </CardContent>
        </Card>
        <Card>
          <CardContent className="pt-6">
            <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground mb-1">
              Last Updated
            </p>
            {loading ? (
              <Skeleton className="h-7 w-24" />
            ) : (
              <span className="text-lg font-medium text-muted-foreground">
                {lastCheck ? lastCheck.toLocaleTimeString() : "—"}
              </span>
            )}
          </CardContent>
        </Card>
      </div>

      {/* Node Table */}
      <Card>
        <CardHeader>
          <CardTitle>Cluster Nodes</CardTitle>
        </CardHeader>
        <CardContent>
          {loading ? (
            <div className="space-y-2">
              <div className="grid grid-cols-5 gap-4 px-4 py-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                <div>Node</div>
                <div>Status</div>
                <div>CPU</div>
                <div>Memory</div>
                <div>Disk</div>
              </div>
              {[...Array(4)].map((_, i) => (
                <div key={i} className="grid grid-cols-5 gap-4 px-4 py-3 bg-secondary rounded-md items-center">
                  <Skeleton className="h-5 w-20" />
                  <Skeleton className="h-5 w-16" />
                  <Skeleton className="h-2 w-full" />
                  <Skeleton className="h-2 w-full" />
                  <Skeleton className="h-5 w-16" />
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
              <div className="grid grid-cols-5 gap-4 px-4 py-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                <div>Node</div>
                <div>Status</div>
                <div>CPU</div>
                <div>Memory</div>
                <div>Disk</div>
              </div>
              {/* Rows */}
              {health.nodes.map((node, idx) => {
                const conditions = node.conditions || [];
                const isHealthy = conditions.every((c) => c.status === "False");
                return (
                  <div
                    key={node.name || idx}
                    className="grid grid-cols-5 gap-4 px-4 py-3 bg-secondary rounded-md items-center"
                  >
                    <div className="font-medium truncate" title={node.name}>
                      {node.name}
                    </div>
                    <div className="flex items-center gap-2">
                      <StatusDot status={isHealthy ? "ok" : "warning"} />
                      <span className="text-sm">{isHealthy ? "Healthy" : "Pressure"}</span>
                    </div>
                    <div className="space-y-1">
                      <Progress value={node.cpu_percent || 0} className="h-2" />
                      <span className="text-xs text-muted-foreground">
                        {(node.cpu_percent || 0).toFixed(1)}%
                      </span>
                    </div>
                    <div className="space-y-1">
                      <Progress value={node.memory_percent || 0} className="h-2" />
                      <span className="text-xs text-muted-foreground">
                        {(node.memory_percent || 0).toFixed(1)}%
                      </span>
                    </div>
                    <div className="text-sm text-muted-foreground">
                      {formatBytes(node.disk_capacity || 0)}
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
