import React, { useRef, useEffect, useState, useCallback } from "react";
import { useContainerLogs } from "@/hooks/useContainerLogs";
import { Button } from "@/components/ui/button";
import { Download, Trash2, ArrowDown } from "lucide-react";

interface ContainerLogsProps {
  containerId: string;
  active: boolean;
}

function parseLogLine(line: string): { timestamp: string; message: string } {
  // K8s log lines start with an RFC3339Nano timestamp followed by a space
  const spaceIdx = line.indexOf(" ");
  if (spaceIdx > 0) {
    const ts = line.slice(0, spaceIdx);
    // Basic check: looks like a timestamp (contains T and -)
    if (ts.includes("T") && ts.includes("-")) {
      return { timestamp: ts, message: line.slice(spaceIdx + 1) };
    }
  }
  return { timestamp: "", message: line };
}

export function ContainerLogs({ containerId, active }: ContainerLogsProps) {
  const { logs, connected, clear } = useContainerLogs(containerId, active);
  const bottomRef = useRef<HTMLDivElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const [autoScroll, setAutoScroll] = useState(true);

  const handleScroll = useCallback(() => {
    const el = containerRef.current;
    if (!el) return;
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 40;
    setAutoScroll(atBottom);
  }, []);

  useEffect(() => {
    if (autoScroll) {
      bottomRef.current?.scrollIntoView({ behavior: "instant" });
    }
  }, [logs, autoScroll]);

  const jumpToBottom = useCallback(() => {
    setAutoScroll(true);
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, []);

  const handleDownload = useCallback(() => {
    const content = logs.join("\n");
    const blob = new Blob([content], { type: "text/plain" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `container-${containerId}-logs.txt`;
    a.click();
    URL.revokeObjectURL(url);
  }, [logs, containerId]);

  return (
    <div className="bg-card border border-border rounded-lg">
      {/* Toolbar */}
      <div className="flex items-center justify-between px-4 py-3 border-b border-border">
        <div className="flex items-center gap-2">
          <span
            className={`inline-block w-2 h-2 rounded-full ${connected ? "bg-green-500" : "bg-muted-foreground"}`}
          />
          <span className="text-xs text-muted-foreground">
            {connected ? "Connected" : "Disconnected"}
          </span>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="ghost" size="sm" onClick={clear} title="Clear logs">
            <Trash2 className="w-4 h-4 mr-1" />
            Clear
          </Button>
          <Button
            variant="ghost"
            size="sm"
            onClick={handleDownload}
            disabled={logs.length === 0}
            title="Download logs"
          >
            <Download className="w-4 h-4 mr-1" />
            Download
          </Button>
        </div>
      </div>

      {/* Log output */}
      <div
        ref={containerRef}
        onScroll={handleScroll}
        className="relative overflow-y-auto font-mono text-xs bg-[#0d0d14] rounded-b-lg"
        style={{ height: "calc(100vh - 280px)", minHeight: "300px" }}
      >
        {logs.length === 0 ? (
          <div className="flex items-center justify-center h-full text-muted-foreground text-sm">
            {connected ? "Waiting for logs..." : "Connect to view logs"}
          </div>
        ) : (
          <div className="p-3 space-y-0.5">
            {logs.map((line, i) => {
              const { timestamp, message } = parseLogLine(line);
              return (
                <div key={i} className="flex gap-2 leading-5">
                  {timestamp && (
                    <span className="shrink-0 text-muted-foreground select-none">
                      {timestamp.length > 23 ? timestamp.slice(0, 23) : timestamp}
                    </span>
                  )}
                  <span className="text-green-300 break-all whitespace-pre-wrap">{message}</span>
                </div>
              );
            })}
            <div ref={bottomRef} />
          </div>
        )}

        {/* Jump to bottom button */}
        {!autoScroll && logs.length > 0 && (
          <button
            onClick={jumpToBottom}
            className="absolute bottom-4 right-4 flex items-center gap-1 bg-secondary text-foreground border border-border px-3 py-1.5 rounded-md text-xs shadow-lg hover:bg-muted transition-colors"
          >
            <ArrowDown className="w-3 h-3" />
            Jump to bottom
          </button>
        )}
      </div>
    </div>
  );
}
