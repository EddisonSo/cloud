import { useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { useMetricsHistory } from "@/hooks";
import { MultiMetricsChart } from "./MetricsChart";
import { TimeRangeSelector } from "./TimeRangeSelector";

export function HistoricalMetricsView(): React.ReactElement {
  const [timeRange, setTimeRange] = useState<string>("1h");
  const [selectedNode, setSelectedNode] = useState<string | null>(null);
  const { data, loading, error, refetch } = useMetricsHistory({ timeRange });

  if (loading && !data) {
    return (
      <div className="space-y-4">
        {/* Head skeleton */}
        <div className="flex items-center justify-between">
          <Skeleton className="h-3 w-36" />
          <Skeleton className="h-3 w-24" />
        </div>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {[...Array(4)].map((_, i) => (
            <Card key={i}>
              <CardHeader>
                <Skeleton className="h-3 w-20" />
              </CardHeader>
              <CardContent>
                <Skeleton className="w-full aspect-[5/2] max-h-[600px]" />
              </CardContent>
            </Card>
          ))}
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="text-center py-12">
        <p className="font-mono text-xs text-destructive mb-4">{error}</p>
        <button
          onClick={refetch}
          className="font-mono text-[10.5px] uppercase tracking-[0.12em] text-primary hover:text-primary/80 transition-colors"
        >
          Retry
        </button>
      </div>
    );
  }

  const series = (data as { series?: Record<string, unknown[]> })?.series || {};
  const nodeNames = Object.keys(series).sort();

  if (nodeNames.length === 0) {
    return (
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <span className="microlabel">Historical Metrics</span>
          <TimeRangeSelector value={timeRange} onChange={setTimeRange} />
        </div>
        <div className="text-center py-12 font-mono text-xs text-muted-foreground">
          No historical data available yet. Data will accumulate over time.
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {/* Section head: microlabel + time-range selector + refresh */}
      <div className="flex items-center justify-between flex-wrap gap-4">
        <span className="microlabel">Historical Metrics</span>
        <div className="flex items-center gap-6">
          <TimeRangeSelector value={timeRange} onChange={setTimeRange} />
          <button
            onClick={refetch}
            className="font-mono text-[10.5px] uppercase tracking-[0.12em] text-faint hover:text-muted-foreground transition-colors"
          >
            Refresh
          </button>
        </div>
      </div>

      {selectedNode ? (
        <div className="space-y-4">
          {/* Drill-down breadcrumb */}
          <div className="flex items-center gap-2 font-mono text-[10.5px] uppercase tracking-[0.12em]">
            <button
              onClick={() => setSelectedNode(null)}
              className="text-primary hover:text-primary/80 transition-colors"
            >
              All Nodes
            </button>
            <span className="text-faint">/</span>
            <span className="text-muted-foreground">{selectedNode}</span>
          </div>
          <Card>
            <CardHeader className="pb-0">
              {/* CardTitle already microlabel-styled */}
              <CardTitle>{selectedNode}</CardTitle>
            </CardHeader>
            <CardContent className="pt-4">
              <MultiMetricsChart
                data={(series[selectedNode] as unknown[]) || []}
                title=""
              />
            </CardContent>
          </Card>
        </div>
      ) : (
        <div className="space-y-4">
          {nodeNames.map((nodeName) => (
            <Card
              key={nodeName}
              className="cursor-pointer hover:border-primary/60 transition-colors"
              onClick={() => setSelectedNode(nodeName)}
            >
              <CardHeader className="pb-0">
                {/* CardTitle renders as microlabel (mono 10.5px uppercase) */}
                <CardTitle>{nodeName}</CardTitle>
              </CardHeader>
              <CardContent className="pt-4">
                <MultiMetricsChart
                  data={(series[nodeName] as unknown[]) || []}
                  title=""
                />
              </CardContent>
            </Card>
          ))}
        </div>
      )}

      {/* Meta strip: resolution + range */}
      <p className="font-mono text-[10px] uppercase tracking-[0.12em] text-faint">
        Resolution:{" "}
        <span className="text-muted-foreground">
          {(data as { resolution?: string })?.resolution || "raw"}
        </span>
        {"  "}Range:{" "}
        <span className="text-muted-foreground">
          {(data as { start?: string })?.start
            ? new Date((data as { start: string }).start).toLocaleString()
            : "—"}
        </span>
        {" — "}
        <span className="text-muted-foreground">
          {(data as { end?: string })?.end
            ? new Date((data as { end: string }).end).toLocaleString()
            : "—"}
        </span>
      </p>
    </div>
  );
}
