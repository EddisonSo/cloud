import { useState, useCallback, useEffect } from "react";
import { buildApiBase, buildWsBase } from "@/lib/api";

export function useContainers(user) {
  const [containers, setContainers] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [actions, setActions] = useState({});

  const loadContainers = useCallback(async () => {
    try {
      setLoading(true);
      setError("");
      const response = await fetch(`${buildApiBase()}/compute/containers`, {
        credentials: "include",
      });
      if (!response.ok) {
        if (response.status === 401) {
          setError("Sign in to manage containers");
          return;
        }
        throw new Error("Failed to load containers");
      }
      const payload = await response.json();
      setContainers(payload.containers || []);
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  }, []);

  const createContainer = useCallback(async (data) => {
    const response = await fetch(`${buildApiBase()}/compute/containers`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
      body: JSON.stringify(data),
    });
    if (!response.ok) {
      const message = await response.text();
      throw new Error(message || "Failed to create container");
    }
    await loadContainers();
    return response.json();
  }, [loadContainers]);

  const containerAction = useCallback(async (id, action) => {
    setActions((prev) => ({ ...prev, [id]: action }));
    try {
      const method = action === "deleting" ? "DELETE" : "POST";
      const endpoint =
        action === "deleting"
          ? `${buildApiBase()}/compute/containers/${id}`
          : `${buildApiBase()}/compute/containers/${id}/${action === "starting" ? "start" : "stop"}`;
      const response = await fetch(endpoint, { method, credentials: "include" });
      if (!response.ok) {
        const message = await response.text();
        throw new Error(message || `Failed to ${action.replace("ing", "")} container`);
      }
      await loadContainers();
    } finally {
      setActions((prev) => ({ ...prev, [id]: null }));
    }
  }, [loadContainers]);

  // WebSocket for real-time container updates
  useEffect(() => {
    if (!user) return;

    let ws = null;
    let reconnectTimeout = null;
    let isCleaningUp = false;

    const connect = () => {
      if (isCleaningUp) return;

      ws = new WebSocket(`${buildWsBase()}/compute/ws`);

      ws.onmessage = (event) => {
        try {
          const msg = JSON.parse(event.data);
          if (msg.type === "containers") {
            setContainers(msg.data || []);
          } else if (msg.type === "container_status") {
            const update = msg.data;
            setContainers((prev) =>
              prev.map((c) =>
                c.id === update.container_id
                  ? { ...c, status: update.status, external_ip: update.external_ip || c.external_ip }
                  : c
              )
            );
          }
        } catch (err) {
          console.error("Failed to parse compute WebSocket message:", err);
        }
      };

      ws.onclose = () => {
        if (!isCleaningUp) {
          reconnectTimeout = setTimeout(connect, 5000);
        }
      };
    };

    connect();

    return () => {
      isCleaningUp = true;
      if (reconnectTimeout) clearTimeout(reconnectTimeout);
      if (ws) ws.close();
    };
  }, [user]);

  return {
    containers,
    setContainers,
    loading,
    error,
    setError,
    actions,
    loadContainers,
    createContainer,
    containerAction,
  };
}
