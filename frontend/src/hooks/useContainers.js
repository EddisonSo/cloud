import { useState, useCallback, useEffect } from "react";
import { buildApiBase, buildWsBase, getAuthHeaders, getAuthToken } from "@/lib/api";
import { registerCacheClear } from "@/lib/cache";

// Module-level cache that persists across component mounts
let cachedContainers = null;
let containersLoaded = false;

// Register cache clear function
registerCacheClear(() => {
  cachedContainers = null;
  containersLoaded = false;
});

export function useContainers(user) {
  const [containers, setContainers] = useState(cachedContainers || []);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [actions, setActions] = useState({});

  const loadContainers = useCallback(async (forceRefresh = false) => {
    // Skip if already loaded and not forcing refresh
    if (containersLoaded && !forceRefresh) {
      return cachedContainers;
    }
    try {
      setLoading(true);
      setError("");
      const response = await fetch(`${buildApiBase()}/compute/containers`, {
        headers: getAuthHeaders(),
      });
      if (!response.ok) {
        if (response.status === 401) {
          setError("Sign in to manage containers");
          return;
        }
        throw new Error("Failed to load containers");
      }
      const payload = await response.json();
      const list = payload.containers || [];
      setContainers(list);
      cachedContainers = list;
      containersLoaded = true;
      return list;
    } catch (err) {
      setError(err.message);
      return [];
    } finally {
      setLoading(false);
    }
  }, []);

  const createContainer = useCallback(async (data) => {
    const response = await fetch(`${buildApiBase()}/compute/containers`, {
      method: "POST",
      headers: { "Content-Type": "application/json", ...getAuthHeaders() },
      body: JSON.stringify(data),
    });
    if (!response.ok) {
      const message = await response.text();
      throw new Error(message || "Failed to create container");
    }
    await loadContainers(true);
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
      const response = await fetch(endpoint, { method, headers: getAuthHeaders() });
      if (!response.ok) {
        const message = await response.text();
        throw new Error(message || `Failed to ${action.replace("ing", "")} container`);
      }
      await loadContainers(true);
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

      // Include token in WebSocket URL as query parameter
      const token = getAuthToken();
      const wsUrl = token
        ? `${buildWsBase()}/compute/ws?token=${encodeURIComponent(token)}`
        : `${buildWsBase()}/compute/ws`;
      ws = new WebSocket(wsUrl);

      ws.onmessage = (event) => {
        try {
          const msg = JSON.parse(event.data);
          if (msg.type === "containers") {
            const list = msg.data || [];
            setContainers(list);
            cachedContainers = list;
            containersLoaded = true;
          } else if (msg.type === "container_status") {
            const update = msg.data;
            setContainers((prev) => {
              const updated = prev.map((c) =>
                c.id === update.container_id
                  ? { ...c, status: update.status, external_ip: update.external_ip || c.external_ip }
                  : c
              );
              cachedContainers = updated;
              return updated;
            });
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
