import { Header } from "@/components/layout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { StatusBadge, StatusDot } from "@/components/common";
import { TAB_COPY } from "@/lib/constants";
import { useHealth } from "@/hooks";
import { useAuth } from "@/contexts/AuthContext";
import { formatBytes } from "@/lib/formatters";

export function HealthPage() {
  const copy = TAB_COPY.health;
  const { user } = useAuth();
  const { health, loading, error, lastCheck } = useHealth(user, true);

  const healthyNodes = health.nodes.filter((n) => n.status === "ok").length;
  const totalNodes = health.nodes.length;

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
            <div className="flex items-center gap-2">
              <StatusDot status={health.cluster_ok ? "ok" : "down"} />
              <span className="text-2xl font-semibold">
                {health.cluster_ok ? "Healthy" : "Degraded"}
              </span>
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="pt-6">
            <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground mb-1">
              Nodes Online
            </p>
            <span className="text-2xl font-semibold">
              {healthyNodes} / {totalNodes}
            </span>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="pt-6">
            <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground mb-1">
              Total Storage
            </p>
            <span className="text-2xl font-semibold">
              {formatBytes(health.nodes.reduce((sum, n) => sum + (n.total_space || 0), 0))}
            </span>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="pt-6">
            <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground mb-1">
              Last Updated
            </p>
            <span className="text-lg font-medium text-muted-foreground">
              {lastCheck ? lastCheck.toLocaleTimeString() : "—"}
            </span>
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
            <p className="text-muted-foreground py-8 text-center">Loading health data...</p>
          ) : error ? (
            <p className="text-destructive py-8 text-center">{error}</p>
          ) : health.nodes.length === 0 ? (
            <p className="text-muted-foreground py-8 text-center">No nodes found</p>
          ) : (
            <div className="space-y-2">
              {/* Header */}
              <div className="grid grid-cols-5 gap-4 px-4 py-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                <div>Node</div>
                <div>Status</div>
                <div>Used Space</div>
                <div>Free Space</div>
                <div>Chunks</div>
              </div>
              {/* Rows */}
              {health.nodes.map((node, idx) => (
                <div
                  key={node.address || idx}
                  className="grid grid-cols-5 gap-4 px-4 py-3 bg-secondary rounded-md items-center"
                >
                  <div className="font-medium truncate" title={node.address}>
                    {node.address}
                  </div>
                  <div>
                    <StatusBadge status={node.status} />
                  </div>
                  <div className="text-sm text-muted-foreground">
                    {formatBytes(node.used_space)}
                  </div>
                  <div className="text-sm text-muted-foreground">
                    {formatBytes(node.free_space)}
                  </div>
                  <div className="text-sm text-muted-foreground">
                    {node.chunk_count?.toLocaleString() || 0}
                  </div>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
