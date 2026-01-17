import { useState, useEffect, useRef } from "react";
import { buildWsBase } from "@/lib/api";

export function useHealth(user, enabled = false) {
  const [health, setHealth] = useState({ cluster_ok: false, nodes: [] });
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [lastCheck, setLastCheck] = useState(null);
  const [updateFrequency, setUpdateFrequency] = useState(0);

  const latestDataRef = useRef(null);

  useEffect(() => {
    if (!user || !enabled) return;

    let ws = null;
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

      ws = new WebSocket(`${buildWsBase()}/ws/cluster-info`);

      ws.onopen = () => {
        setLoading(false);
        setError("");
      };

      ws.onmessage = (event) => {
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

      ws.onerror = () => {
        setError("WebSocket error");
        setHealth({ cluster_ok: false, nodes: [] });
      };

      ws.onclose = () => {
        setLoading(false);
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
      if (ws) ws.close();
    };
  }, [user, enabled, updateFrequency]);

  return { health, loading, error, lastCheck, updateFrequency, setUpdateFrequency };
}
