import { useState, useEffect, useRef } from "react";
import { buildHealthBase, getAuthToken } from "@/lib/api";

export function useHealth(user, enabled = false) {
  const [health, setHealth] = useState({ cluster_ok: false, nodes: [] });
  const [podMetrics, setPodMetrics] = useState({ pods: [] });
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [lastCheck, setLastCheck] = useState(null);
  const [updateFrequency, setUpdateFrequency] = useState(0);

  const latestClusterRef = useRef(null);
  const latestPodsRef = useRef(null);

  // Combined health SSE (cluster-info + pod-metrics in single connection)
  useEffect(() => {
    if (!user || !enabled) return;

    let eventSource = null;
    let reconnectTimeout = null;
    let updateInterval = null;
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

      eventSource.onmessage = (event) => {
        try {
          const message = JSON.parse(event.data);

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
        eventSource.close();
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
