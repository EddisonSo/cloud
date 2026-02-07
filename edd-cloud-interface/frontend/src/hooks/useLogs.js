import { useState, useEffect, useRef, useCallback } from "react";
import { buildHealthBase } from "@/lib/api";

export function useLogs(user, enabled = false) {
  const [logs, setLogs] = useState([]);
  const [connected, setConnected] = useState(false);
  const [error, setError] = useState("");
  const [sourceFilter, setSourceFilter] = useState("");
  const [levelFilter, setLevelFilter] = useState("DEBUG");
  const [sources, setSources] = useState(["edd-auth", "edd-compute", "edd-gateway", "edd-storage"]);
  const [autoScroll, setAutoScroll] = useState(true);
  const [updateFrequency, setUpdateFrequency] = useState(0);
  const [lastLogTime, setLastLogTime] = useState(null);

  const autoScrollRef = useRef(true);
  const containerRef = useRef(null);
  const bufferRef = useRef([]);
  const lastFlushRef = useRef(0);

  const maxLogs = 1000;

  const clearLogs = useCallback(() => setLogs([]), []);

  const logLevelColor = (level) => {
    switch (level) {
      case 0: return "debug";
      case 1: return "info";
      case 2: return "warn";
      case 3: return "error";
      default: return "info";
    }
  };

  const logLevelName = (level) => {
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

    let eventSource = null;
    let reconnectTimeout = null;
    let flushInterval = null;
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
      const sseUrl = `${buildHealthBase()}/sse/logs${params.toString() ? "?" + params.toString() : ""}`;

      eventSource = new EventSource(sseUrl);

      eventSource.onopen = () => {
        setConnected(true);
        setError("");
      };

      eventSource.onmessage = (event) => {
        try {
          const entry = JSON.parse(event.data);
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

      eventSource.onerror = () => {
        setError("Connection error");
        setConnected(false);
        eventSource.close();
        if (!isCleaningUp) {
          reconnectTimeout = setTimeout(connect, 5000);
        }
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
      if (eventSource) eventSource.close();
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
