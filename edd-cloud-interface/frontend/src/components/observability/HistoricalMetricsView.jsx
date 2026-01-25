import { useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { useMetricsHistory } from "@/hooks";
import { MultiMetricsChart } from "./MetricsChart";
import { TimeRangeSelector } from "./TimeRangeSelector";

export function HistoricalMetricsView() {
  const [timeRange, setTimeRange] = useState("1h");
  const [selectedNode, setSelectedNode] = useState(null);
  const { data, loading, error, refetch } = useMetricsHistory({ timeRange });

  if (loading && !data) {
    return (
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <Skeleton className="h-8 w-48" />
          <Skeleton className="h-8 w-32" />
        </div>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {[...Array(4)].map((_, i) => (
            <Card key={i}>
              <CardHeader>
                <Skeleton className="h-5 w-24" />
              </CardHeader>
              <CardContent>
                <Skeleton className="h-[400px] w-full" />
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
        <p className="text-destructive mb-4">{error}</p>
        <button
          onClick={refetch}
          className="text-sm text-primary hover:underline"
        >
          Retry
        </button>
      </div>
    );
  }

  const series = data?.series || {};
  const nodeNames = Object.keys(series).sort();

  if (nodeNames.length === 0) {
    return (
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <h3 className="text-lg font-semibold">Historical Metrics</h3>
          <TimeRangeSelector value={timeRange} onChange={setTimeRange} />
        </div>
        <div className="text-center py-12 text-muted-foreground">
          No historical data available yet. Data will accumulate over time.
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between flex-wrap gap-4">
        <h3 className="text-lg font-semibold">Historical Metrics</h3>
        <div className="flex items-center gap-4">
          <TimeRangeSelector value={timeRange} onChange={setTimeRange} />
          <button
            onClick={refetch}
            className="text-sm text-muted-foreground hover:text-foreground transition-colors"
          >
            Refresh
          </button>
        </div>
      </div>

      {selectedNode ? (
        <div className="space-y-4">
          <div className="flex items-center gap-2">
            <button
              onClick={() => setSelectedNode(null)}
              className="text-sm text-primary hover:underline"
            >
              All Nodes
            </button>
            <span className="text-muted-foreground">/</span>
            <span className="font-medium">{selectedNode}</span>
          </div>
          <Card>
            <CardHeader>
              <CardTitle className="text-base">{selectedNode}</CardTitle>
            </CardHeader>
            <CardContent>
              <MultiMetricsChart
                data={series[selectedNode] || []}
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
              className="cursor-pointer hover:border-primary/50 transition-colors"
              onClick={() => setSelectedNode(nodeName)}
            >
              <CardHeader className="pb-2">
                <CardTitle className="text-base">{nodeName}</CardTitle>
              </CardHeader>
              <CardContent>
                <MultiMetricsChart
                  data={series[nodeName] || []}
                  title=""
                />
              </CardContent>
            </Card>
          ))}
        </div>
      )}

      <p className="text-xs text-muted-foreground">
        Resolution: {data?.resolution || "raw"} |
        Range: {data?.start ? new Date(data.start).toLocaleString() : "—"} - {data?.end ? new Date(data.end).toLocaleString() : "—"}
      </p>
    </div>
  );
}
