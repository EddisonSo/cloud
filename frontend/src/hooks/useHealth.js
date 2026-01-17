import { useState, useEffect } from "react";
import { buildWsBase } from "@/lib/api";

export function useHealth(user, enabled = false) {
  const [health, setHealth] = useState({ cluster_ok: false, nodes: [] });
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [lastCheck, setLastCheck] = useState(null);

  useEffect(() => {
    if (!user || !enabled) return;

    let ws = null;
    let reconnectTimeout = null;

    const connect = () => {
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
          setHealth({ cluster_ok: true, nodes: payload.nodes || [] });
          setLastCheck(new Date());
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
        reconnectTimeout = setTimeout(connect, 5000);
      };
    };

    connect();

    return () => {
      if (reconnectTimeout) clearTimeout(reconnectTimeout);
      if (ws) ws.close();
    };
  }, [user, enabled]);

  return { health, loading, error, lastCheck };
}
