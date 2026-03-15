import { useState, useRef, useCallback, useEffect } from "react";
import { buildComputeWsBase, getAuthToken } from "@/lib/api";

export function useContainerLogs(containerId: string, active: boolean) {
  const [logs, setLogs] = useState<string[]>([]);
  const [connected, setConnected] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);
  const bufferRef = useRef<string[]>([]);
  const maxLogs = 2000;

  const flush = useCallback(() => {
    if (bufferRef.current.length === 0) return;
    const batch = bufferRef.current;
    bufferRef.current = [];
    setLogs((prev) => {
      const next = [...prev, ...batch];
      return next.length > maxLogs ? next.slice(-maxLogs) : next;
    });
  }, []);

  useEffect(() => {
    if (!active || !containerId) return;
    const token = getAuthToken();
    const url = `${buildComputeWsBase()}/compute/containers/${containerId}/logs?token=${token}&tail=100`;
    const ws = new WebSocket(url);
    wsRef.current = ws;
    ws.onopen = () => setConnected(true);
    ws.onclose = () => setConnected(false);
    ws.onmessage = (e) => { bufferRef.current.push(e.data); };
    const interval = setInterval(flush, 200);
    return () => {
      clearInterval(interval);
      flush();
      ws.close();
      wsRef.current = null;
      setConnected(false);
    };
  }, [containerId, active, flush]);

  const clear = useCallback(() => setLogs([]), []);
  return { logs, connected, clear };
}
