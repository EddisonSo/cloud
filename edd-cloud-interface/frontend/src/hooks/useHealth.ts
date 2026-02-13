import { useState, useEffect, useRef } from "react";
import { buildHealthBase, getAuthToken } from "@/lib/api";
import type { HealthState, PodMetrics, ClusterNode, HealthSseMessage } from "@/types";

export function useHealth(user: string | null, enabled: boolean = false) {
  const [health, setHealth] = useState<HealthState>({ cluster_ok: false, nodes: [] });
  const [podMetrics, setPodMetrics] = useState<PodMetrics>({ pods: [] });
  const [loading, setLoading] = useState<boolean>(false);
  const [error, setError] = useState<string>("");
  const [lastCheck, setLastCheck] = useState<Date | null>(null);
  const [updateFrequency, setUpdateFrequency] = useState<number>(0);

  const latestClusterRef = useRef<{ nodes: ClusterNode[] } | null>(null);
  const latestPodsRef = useRef<PodMetrics | null>(null);

  // Combined health SSE (cluster-info + pod-metrics in single connection)
  useEffect(() => {
    if (!user || !enabled) return;

    let eventSource: EventSource | null = null;
    let reconnectTimeout: ReturnType<typeof setTimeout> | null = null;
    let updateInterval: ReturnType<typeof setInterval> | null = null;
    let isCleaningUp = false;

    const applyUpdate = () => {
      if (latestClusterRef.current) {
        setHealth({ cluster_ok: true, nodes: latestClusterRef.current.nodes || [] });
        setLastCheck(new Date());
      }
      if (latestPodsRef.current) {
        setPodMetrics(latestPodsRef.current);
      }
    };

    const connect = () => {
      if (isCleaningUp) return;
      setLoading(true);
      setError("");

      const token = getAuthToken();
      const sseUrl = token
        ? `${buildHealthBase()}/sse/health?token=${encodeURIComponent(token)}`
        : `${buildHealthBase()}/sse/health`;
      eventSource = new EventSource(sseUrl);

      eventSource.onopen = () => {
        setLoading(false);
        setError("");
      };

      eventSource.onmessage = (event: MessageEvent) => {
        try {
          const message: HealthSseMessage = JSON.parse(event.data);

          if (message.type === "cluster") {
            latestClusterRef.current = message.payload;
            if (updateFrequency === 0) {
              setHealth({ cluster_ok: true, nodes: message.payload.nodes || [] });
              setLastCheck(new Date());
            }
          } else if (message.type === "pods") {
            latestPodsRef.current = message.payload;
            if (updateFrequency === 0) {
              setPodMetrics(message.payload);
            }
          }
        } catch (err) {
          console.error("Failed to parse health data:", err);
        }
      };

      eventSource.onerror = () => {
        setError("Connection error");
        setHealth({ cluster_ok: false, nodes: [] });
        eventSource!.close();
        if (!isCleaningUp) {
          reconnectTimeout = setTimeout(connect, 5000);
        }
      };
    };

    connect();

    if (updateFrequency > 0) {
      updateInterval = setInterval(applyUpdate, updateFrequency);
    }

    return () => {
      isCleaningUp = true;
      if (reconnectTimeout) clearTimeout(reconnectTimeout);
      if (updateInterval) clearInterval(updateInterval);
      if (eventSource) eventSource.close();
    };
  }, [user, enabled, updateFrequency]);

  return { health, podMetrics, loading, error, lastCheck, updateFrequency, setUpdateFrequency };
}
