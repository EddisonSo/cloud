import { useState, useCallback, useRef } from "react";
import { buildApiBase } from "@/lib/api";

export function useSshKeys() {
  const [sshKeys, setSshKeys] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const loadedRef = useRef(false);

  const loadSshKeys = useCallback(async (forceRefresh = false) => {
    // Skip if already loaded and not forcing refresh
    if (loadedRef.current && !forceRefresh) {
      return sshKeys;
    }
    try {
      setLoading(true);
      const response = await fetch(`${buildApiBase()}/compute/ssh-keys`, {
        credentials: "include",
      });
      if (!response.ok) return [];
      const payload = await response.json();
      setSshKeys(payload.ssh_keys || []);
      loadedRef.current = true;
      return payload.ssh_keys || [];
    } catch (err) {
      console.warn("Failed to load SSH keys:", err.message);
      return [];
    } finally {
      setLoading(false);
    }
  }, [sshKeys]);

  const addSshKey = useCallback(async (name, publicKey) => {
    const response = await fetch(`${buildApiBase()}/compute/ssh-keys`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
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
    const response = await fetch(`${buildApiBase()}/compute/ssh-keys/${id}`, {
      method: "DELETE",
      credentials: "include",
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
