import { useState, useEffect, useRef } from "react";
import { buildApiBase } from "@/lib/api";

export function useHealth(user, enabled = false) {
  const [health, setHealth] = useState({ cluster_ok: false, nodes: [] });
  const [podMetrics, setPodMetrics] = useState({ pods: [] });
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [lastCheck, setLastCheck] = useState(null);
  const [updateFrequency, setUpdateFrequency] = useState(0);

  const latestDataRef = useRef(null);

  // Cluster info via SSE
  useEffect(() => {
    if (!user || !enabled) return;

    let eventSource = null;
    let reconnectTimeout = null;
    let updateInterval = null;
    let isCleaningUp = false;

    const applyUpdate = () => {
      if (latestDataRef.current) {
        setHealth({ cluster_ok: true, nodes: latestDataRef.current.nodes || [] });
        setLastCheck(new Date());
      }
    };

    const connect = () => {
      if (isCleaningUp) return;
      setLoading(true);
      setError("");

      eventSource = new EventSource(`${buildApiBase()}/sse/cluster-info`);

      eventSource.onopen = () => {
        setLoading(false);
        setError("");
      };

      eventSource.onmessage = (event) => {
        try {
          const payload = JSON.parse(event.data);
          latestDataRef.current = payload;

          if (updateFrequency === 0) {
            setHealth({ cluster_ok: true, nodes: payload.nodes || [] });
            setLastCheck(new Date());
          }
        } catch (err) {
          console.error("Failed to parse cluster info:", err);
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

  // Pod metrics via SSE
  useEffect(() => {
    if (!user || !enabled) return;

    let eventSource = null;
    let reconnectTimeout = null;
    let isCleaningUp = false;

    const connect = () => {
      if (isCleaningUp) return;

      eventSource = new EventSource(`${buildApiBase()}/sse/pod-metrics`);

      eventSource.onmessage = (event) => {
        try {
          const payload = JSON.parse(event.data);
          setPodMetrics(payload);
        } catch (err) {
          console.error("Failed to parse pod metrics:", err);
        }
      };

      eventSource.onerror = () => {
        console.error("Pod metrics connection error");
        eventSource.close();
        if (!isCleaningUp) {
          reconnectTimeout = setTimeout(connect, 5000);
        }
      };
    };

    connect();

    return () => {
      isCleaningUp = true;
      if (reconnectTimeout) clearTimeout(reconnectTimeout);
      if (eventSource) eventSource.close();
    };
  }, [user, enabled]);

  return { health, podMetrics, loading, error, lastCheck, updateFrequency, setUpdateFrequency };
}
