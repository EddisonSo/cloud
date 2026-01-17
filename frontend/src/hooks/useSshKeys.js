import { useState, useCallback } from "react";
import { buildApiBase } from "@/lib/api";

export function useSshKeys() {
  const [sshKeys, setSshKeys] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const loadSshKeys = useCallback(async () => {
    try {
      setLoading(true);
      const response = await fetch(`${buildApiBase()}/compute/ssh-keys`, {
        credentials: "include",
      });
      if (!response.ok) return;
      const payload = await response.json();
      setSshKeys(payload.ssh_keys || []);
    } catch (err) {
      console.warn("Failed to load SSH keys:", err.message);
    } finally {
      setLoading(false);
    }
  }, []);

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
    await loadSshKeys();
    return response.json();
  }, [loadSshKeys]);

  const deleteSshKey = useCallback(async (id) => {
    const response = await fetch(`${buildApiBase()}/compute/ssh-keys/${id}`, {
      method: "DELETE",
      credentials: "include",
    });
    if (!response.ok) throw new Error("Failed to delete SSH key");
    await loadSshKeys();
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
