import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Select } from "@/components/ui/select";
import { ScrollArea } from "@/components/ui/scroll-area";
import { useLogs } from "@/hooks";
import { useAuth } from "@/contexts/AuthContext";
import { Trash2 } from "lucide-react";

/*
 * Map log level strings to Badge variants that conform to design tokens.
 * debug → secondary (dim border, muted text)
 * info  → default  (ice border + text)
 * warn  → warning  (amber)
 * error → destructive (red)
 */
type BadgeVariant = "default" | "secondary" | "destructive" | "outline" | "success" | "warning";

function levelVariant(levelColor: string): BadgeVariant {
  switch (levelColor) {
    case "info":  return "default";
    case "warn":  return "warning";
    case "error": return "destructive";
    default:      return "secondary"; // debug
  }
}

export function LogsView(): React.ReactElement {
  const { user } = useAuth();
  const {
    logs,
    connected,
    error,
    sourceFilter,
    setSourceFilter,
    levelFilter,
    setLevelFilter,
    sources,
    autoScroll,
    setAutoScroll,
    updateFrequency,
    setUpdateFrequency,
    containerRef,
    clearLogs,
    logLevelColor,
    logLevelName,
  } = useLogs(user, true);

  const formatLogTime = (timestamp: number): string => {
    if (!timestamp) return "";
    const date = new Date(timestamp * 1000);
    return date.toLocaleTimeString([], {
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
    });
  };

  return (
    <Card className="flex flex-col h-[calc(100vh-280px)] min-h-[400px]">
      {/* Card head: microlabel title + connection badge + clear button */}
      <CardHeader className="flex-shrink-0 flex flex-row items-center justify-between space-y-0 pb-4">
        <CardTitle className="flex items-center gap-3">
          Cluster Logs
          <Badge variant={connected ? "success" : "destructive"}>
            {connected ? "Connected" : "Disconnected"}
          </Badge>
        </CardTitle>
        <Button variant="outline" size="sm" onClick={clearLogs}>
          <Trash2 className="w-3.5 h-3.5 mr-1.5" />
          Clear
        </Button>
      </CardHeader>

      <CardContent className="flex-1 flex flex-col min-h-0 pt-0">
        {/* Filters — microlabel labels */}
        <div className="flex flex-wrap items-center gap-4 pb-4 border-b border-border mb-4">
          <div className="flex items-center gap-2">
            <span className="microlabel">Source</span>
            <Select
              value={sourceFilter}
              onChange={(e: React.ChangeEvent<HTMLSelectElement>) =>
                setSourceFilter(e.target.value)
              }
              className="w-40"
            >
              <option value="">All</option>
              {sources.map((s) => (
                <option key={s} value={s}>
                  {s}
                </option>
              ))}
            </Select>
          </div>

          <div className="flex items-center gap-2">
            <span className="microlabel">Level</span>
            <Select
              value={levelFilter}
              onChange={(e: React.ChangeEvent<HTMLSelectElement>) =>
                setLevelFilter(e.target.value)
              }
              className="w-32"
            >
              <option value="DEBUG">Debug+</option>
              <option value="INFO">Info+</option>
              <option value="WARN">Warn+</option>
              <option value="ERROR">Error</option>
            </Select>
          </div>

          <div className="flex items-center gap-2">
            <span className="microlabel">Update</span>
            <Select
              value={updateFrequency}
              onChange={(e: React.ChangeEvent<HTMLSelectElement>) =>
                setUpdateFrequency(Number(e.target.value))
              }
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

          <label className="flex items-center gap-2 cursor-pointer">
            <input
              type="checkbox"
              checked={autoScroll}
              onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
                setAutoScroll(e.target.checked)
              }
              className="w-4 h-4 accent-primary"
            />
            <span className="microlabel">Auto-scroll</span>
          </label>
        </div>

        {/* Log output — flat bordered, bg-background */}
        {error ? (
          <p className="font-mono text-xs text-destructive py-8 text-center">
            {error}
          </p>
        ) : (
          <ScrollArea
            ref={containerRef}
            className="flex-1 bg-background border border-border font-mono text-xs"
          >
            <div className="p-2 space-y-0.5">
              {logs.length === 0 ? (
                <p className="text-muted-foreground text-center py-8 font-mono text-xs">
                  Waiting for logs…
                </p>
              ) : (
                logs.map((entry, idx) => (
                  <div
                    key={idx}
                    className="grid grid-cols-[auto_auto_auto_1fr] gap-3 px-2 py-1 hover:bg-popover items-baseline"
                  >
                    {/* Timestamp */}
                    <span className="text-faint text-[11px] whitespace-nowrap tabular-nums">
                      {formatLogTime(entry.timestamp as unknown as number)}
                    </span>

                    {/* Level badge — design-token variant */}
                    <Badge
                      variant={levelVariant(logLevelColor(entry.level))}
                      className="px-1.5 py-0.5 text-[10px] whitespace-nowrap leading-none"
                    >
                      {logLevelName(entry.level)}
                    </Badge>

                    {/* Source — ice accent */}
                    <span className="text-primary whitespace-nowrap">
                      {entry.source}
                    </span>

                    {/* Message */}
                    <span className="text-foreground break-words">
                      {entry.message}
                    </span>
                  </div>
                ))
              )}
            </div>
          </ScrollArea>
        )}

        {/* Footer strip */}
        <div className="flex justify-between items-center pt-3 font-mono text-[10px] uppercase tracking-[0.12em] text-faint">
          <span>
            {logs.length} <span className="text-muted-foreground">{logs.length === 1 ? "entry" : "entries"}</span>
          </span>
        </div>
      </CardContent>
    </Card>
  );
}
