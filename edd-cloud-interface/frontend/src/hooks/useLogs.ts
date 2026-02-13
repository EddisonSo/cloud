import { useState, useEffect, useRef, useCallback } from "react";
import { buildHealthBase } from "@/lib/api";
import type { LogEntry, LogLevel } from "@/types";

export function useLogs(user: string | null, enabled: boolean = false) {
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [connected, setConnected] = useState<boolean>(false);
  const [error, setError] = useState<string>("");
  const [sourceFilter, setSourceFilter] = useState<string>("");
  const [levelFilter, setLevelFilter] = useState<string>("DEBUG");
  const [sources, setSources] = useState<string[]>(["edd-auth", "edd-compute", "edd-gateway", "edd-storage"]);
  const [autoScroll, setAutoScroll] = useState<boolean>(true);
  const [updateFrequency, setUpdateFrequency] = useState<number>(0);
  const [lastLogTime, setLastLogTime] = useState<Date | null>(null);

  const autoScrollRef = useRef<boolean>(true);
  const containerRef = useRef<HTMLDivElement>(null);
  const bufferRef = useRef<LogEntry[]>([]);
  const lastFlushRef = useRef<number>(0);

  const maxLogs = 1000;

  const clearLogs = useCallback(() => setLogs([]), []);

  const logLevelColor = (level: LogLevel): string => {
    switch (level) {
      case 0: return "debug";
      case 1: return "info";
      case 2: return "warn";
      case 3: return "error";
      default: return "info";
    }
  };

  const logLevelName = (level: LogLevel): string => {
    switch (level) {
      case 0: return "DEBUG";
      case 1: return "INFO";
      case 2: return "WARN";
      case 3: return "ERROR";
      default: return "INFO";
    }
  };

  useEffect(() => {
    if (!user || !enabled) return;

    let ws: WebSocket | null = null;
    let reconnectTimeout: ReturnType<typeof setTimeout> | null = null;
    let flushInterval: ReturnType<typeof setInterval> | null = null;
    let isCleaningUp = false;

    setLogs([]);
    bufferRef.current = [];

    const flushBuffer = () => {
      if (bufferRef.current.length === 0) return;
      const toFlush = bufferRef.current;
      bufferRef.current = [];
      lastFlushRef.current = Date.now();
      setLogs((prev) => {
        const next = [...prev, ...toFlush];
        return next.length > maxLogs ? next.slice(-maxLogs) : next;
      });
    };

    const connect = () => {
      if (isCleaningUp) return;
      setError("");

      const params = new URLSearchParams();
      if (sourceFilter) params.set("source", sourceFilter);
      if (levelFilter && levelFilter !== "DEBUG") params.set("level", levelFilter);
      const protocol = window.location.protocol === "https:" ? "wss" : "ws";
      const host = buildHealthBase().replace(/^https?:\/\//, "");
      const wsUrl = `${protocol}://${host}/ws/logs${params.toString() ? "?" + params.toString() : ""}`;

      ws = new WebSocket(wsUrl);

      ws.onopen = () => {
        setConnected(true);
        setError("");
      };

      ws.onmessage = (event: MessageEvent) => {
        try {
          const entry: LogEntry = JSON.parse(event.data);
          setLastLogTime(new Date());

          if (updateFrequency === 0) {
            setLogs((prev) => {
              const next = [...prev, entry];
              return next.length > maxLogs ? next.slice(-maxLogs) : next;
            });
          } else {
            bufferRef.current.push(entry);
          }
        } catch (err) {
          console.error("Failed to parse log entry:", err);
        }
      };

      ws.onclose = () => {
        setConnected(false);
        if (!isCleaningUp) {
          reconnectTimeout = setTimeout(connect, 5000);
        }
      };

      ws.onerror = () => {
        setError("Connection error");
      };
    };

    connect();

    if (updateFrequency > 0) {
      flushInterval = setInterval(flushBuffer, updateFrequency);
    }

    return () => {
      isCleaningUp = true;
      if (reconnectTimeout) clearTimeout(reconnectTimeout);
      if (flushInterval) clearInterval(flushInterval);
      if (ws) ws.close();
      flushBuffer();
    };
  }, [user, enabled, sourceFilter, levelFilter, updateFrequency]);

  // Auto-scroll
  useEffect(() => {
    if (autoScrollRef.current && containerRef.current) {
      containerRef.current.scrollTop = containerRef.current.scrollHeight;
    }
  }, [logs]);

  // Sync autoScroll ref
  useEffect(() => {
    autoScrollRef.current = autoScroll;
  }, [autoScroll]);

  return {
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
    lastLogTime,
    containerRef,
    clearLogs,
    logLevelColor,
    logLevelName,
  };
}
