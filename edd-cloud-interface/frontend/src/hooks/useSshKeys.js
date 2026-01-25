import { useState, useCallback, useRef, useEffect } from "react";
import { buildComputeBase, getAuthHeaders } from "@/lib/api";
import { registerCacheClear } from "@/lib/cache";

// Module-level cache that persists across component mounts
let cachedSshKeys = null;
let sshKeysLoaded = false;

// Register cache clear function
registerCacheClear(() => {
  cachedSshKeys = null;
  sshKeysLoaded = false;
});

export function useSshKeys() {
  const [sshKeys, setSshKeys] = useState(cachedSshKeys || []);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const abortControllerRef = useRef(null);

  // Cleanup abort controller on unmount
  useEffect(() => {
    return () => {
      if (abortControllerRef.current) {
        abortControllerRef.current.abort();
      }
    };
  }, []);

  const loadSshKeys = useCallback(async (forceRefresh = false) => {
    // Skip if already loaded and not forcing refresh
    if (sshKeysLoaded && !forceRefresh) {
      return cachedSshKeys;
    }

    // Abort any in-flight request
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
    }
    abortControllerRef.current = new AbortController();

    try {
      setLoading(true);
      const response = await fetch(`${buildComputeBase()}/compute/ssh-keys`, {
        headers: getAuthHeaders(),
        signal: abortControllerRef.current.signal,
      });
      if (!response.ok) return [];
      const payload = await response.json();
      const list = payload.ssh_keys || [];
      setSshKeys(list);
      cachedSshKeys = list;
      sshKeysLoaded = true;
      return list;
    } catch (err) {
      if (err.name === "AbortError") return [];
      console.warn("Failed to load SSH keys:", err.message);
      return [];
    } finally {
      setLoading(false);
    }
  }, []);

  const addSshKey = useCallback(async (name, publicKey) => {
    const response = await fetch(`${buildComputeBase()}/compute/ssh-keys`, {
      method: "POST",
      headers: { "Content-Type": "application/json", ...getAuthHeaders() },
      body: JSON.stringify({ name, public_key: publicKey }),
    });
    if (!response.ok) {
      const message = await response.text();
      throw new Error(message || "Failed to add SSH key");
    }
    await loadSshKeys(true);
    return response.json();
  }, [loadSshKeys]);

  const deleteSshKey = useCallback(async (id) => {
    const response = await fetch(`${buildComputeBase()}/compute/ssh-keys/${id}`, {
      method: "DELETE",
      headers: getAuthHeaders(),
    });
    if (!response.ok) throw new Error("Failed to delete SSH key");
    await loadSshKeys(true);
  }, [loadSshKeys]);

  return {
    sshKeys,
    loading,
    error,
    setError,
    loadSshKeys,
    addSshKey,
    deleteSshKey,
  };
}
