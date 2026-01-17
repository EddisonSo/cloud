import { Header } from "@/components/layout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Select } from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";
import { ScrollArea } from "@/components/ui/scroll-area";
import { TAB_COPY } from "@/lib/constants";
import { useLogs } from "@/hooks";
import { useAuth } from "@/contexts/AuthContext";
import { cn } from "@/lib/utils";
import { Trash2 } from "lucide-react";

export function LogsPage() {
  const copy = TAB_COPY.logs;
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

  const formatLogTime = (timestamp) => {
    if (!timestamp) return "";
    const date = new Date(timestamp * 1000);
    return date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });
  };

  const levelColors = {
    debug: "bg-muted text-muted-foreground",
    info: "bg-blue-500/20 text-blue-400",
    warn: "bg-yellow-500/20 text-yellow-400",
    error: "bg-red-500/20 text-red-400",
  };

  return (
    <div>
      <Header eyebrow={copy.eyebrow} title={copy.title} description={copy.lead} />

      <Card className="flex flex-col h-[calc(100vh-200px)] min-h-[400px]">
        <CardHeader className="flex-shrink-0 flex flex-row items-center justify-between space-y-0 pb-4">
          <CardTitle className="flex items-center gap-3">
            Cluster Logs
            <Badge variant={connected ? "success" : "destructive"} className="text-xs">
              {connected ? "Connected" : "Disconnected"}
            </Badge>
          </CardTitle>
          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm" onClick={clearLogs}>
              <Trash2 className="w-4 h-4 mr-1" />
              Clear
            </Button>
          </div>
        </CardHeader>
        <CardContent className="flex-1 flex flex-col min-h-0 pt-0">
          {/* Filters */}
          <div className="flex flex-wrap items-center gap-4 pb-4 border-b border-border mb-4">
            <div className="flex items-center gap-2">
              <span className="text-sm text-muted-foreground">Source:</span>
              <Select
                value={sourceFilter}
                onChange={(e) => setSourceFilter(e.target.value)}
                className="w-40"
              >
                <option value="">All</option>
                {sources.map((s) => (
                  <option key={s} value={s}>{s}</option>
                ))}
              </Select>
            </div>
            <div className="flex items-center gap-2">
              <span className="text-sm text-muted-foreground">Level:</span>
              <Select
                value={levelFilter}
                onChange={(e) => setLevelFilter(e.target.value)}
                className="w-32"
              >
                <option value="DEBUG">Debug+</option>
                <option value="INFO">Info+</option>
                <option value="WARN">Warn+</option>
                <option value="ERROR">Error</option>
              </Select>
            </div>
            <div className="flex items-center gap-2">
              <span className="text-sm text-muted-foreground">Update:</span>
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
            <label className="flex items-center gap-2 text-sm cursor-pointer">
              <input
                type="checkbox"
                checked={autoScroll}
                onChange={(e) => setAutoScroll(e.target.checked)}
                className="w-4 h-4 accent-primary"
              />
              Auto-scroll
            </label>
          </div>

          {/* Logs Container */}
          {error ? (
            <p className="text-destructive py-8 text-center">{error}</p>
          ) : (
            <ScrollArea
              ref={containerRef}
              className="flex-1 bg-background border border-border rounded-md font-mono text-xs"
            >
              <div className="p-2 space-y-0.5">
                {logs.length === 0 ? (
                  <p className="text-muted-foreground text-center py-8">Waiting for logs...</p>
                ) : (
                  logs.map((entry, idx) => (
                    <div
                      key={idx}
                      className="grid grid-cols-[auto_auto_auto_1fr] gap-3 px-2 py-1 rounded hover:bg-accent/50 items-baseline"
                    >
                      <span className="text-muted-foreground text-[11px] whitespace-nowrap">
                        {formatLogTime(entry.timestamp)}
                      </span>
                      <span
                        className={cn(
                          "px-1.5 py-0.5 rounded text-[10px] font-semibold uppercase whitespace-nowrap",
                          levelColors[logLevelColor(entry.level)]
                        )}
                      >
                        {logLevelName(entry.level)}
                      </span>
                      <span className="text-purple-400 font-medium whitespace-nowrap">
                        {entry.source}
                      </span>
                      <span className="text-foreground break-words">
                        {entry.message}
                      </span>
                    </div>
                  ))
                )}
              </div>
            </ScrollArea>
          )}

          {/* Footer */}
          <div className="flex justify-between items-center pt-3 text-xs text-muted-foreground">
            <span>{logs.length} log entries</span>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
