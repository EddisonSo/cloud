import { useState, useCallback } from "react";
import { buildApiBase, getAuthHeaders } from "@/lib/api";
import { DEFAULT_NAMESPACE } from "@/lib/constants";
import { registerCacheClear } from "@/lib/cache";

// Module-level cache that persists across component mounts
let cachedNamespaces = null;
let namespacesLoaded = false;

// Register cache clear function
registerCacheClear(() => {
  cachedNamespaces = null;
  namespacesLoaded = false;
});

export function useNamespaces() {
  const [namespaces, setNamespaces] = useState(cachedNamespaces || []);
  const [activeNamespace, setActiveNamespace] = useState("");
  const [loading, setLoading] = useState(false);

  const normalizeNamespace = (value) => (value && value.trim() ? value.trim() : DEFAULT_NAMESPACE);

  const loadNamespaces = useCallback(async (forceRefresh = false) => {
    // Skip if already loaded and not forcing refresh
    if (namespacesLoaded && !forceRefresh) {
      return cachedNamespaces;
    }
    try {
      setLoading(true);
      const response = await fetch(`${buildApiBase()}/storage/namespaces`, {
        headers: getAuthHeaders(),
      });
      if (!response.ok) throw new Error("Failed to load namespaces");
      const payload = await response.json();
      const sorted = payload
        .map((item) => ({
          name: item.name,
          count: item.count ?? 0,
          hidden: item.hidden ?? false,
        }))
        .sort((a, b) => a.name.localeCompare(b.name));
      setNamespaces(sorted);
      cachedNamespaces = sorted;
      namespacesLoaded = true;
      return sorted;
    } catch (err) {
      console.warn(err.message);
      return [];
    } finally {
      setLoading(false);
    }
  }, []);

  const createNamespace = useCallback(async (name, hidden = false) => {
    const normalizedName = normalizeNamespace(name);
    const response = await fetch(`${buildApiBase()}/storage/namespaces`, {
      method: "POST",
      headers: { "Content-Type": "application/json", ...getAuthHeaders() },
      body: JSON.stringify({ name: normalizedName, hidden }),
    });
    if (!response.ok) {
      if (response.status === 409) throw new Error("Namespace already exists.");
      const message = await response.text();
      throw new Error(message || "Failed to create namespace");
    }
    await loadNamespaces(true);
    return response.json();
  }, [loadNamespaces]);

  const deleteNamespace = useCallback(async (name) => {
    const response = await fetch(
      `${buildApiBase()}/storage/namespaces/${encodeURIComponent(name)}`,
      { method: "DELETE", headers: getAuthHeaders() }
    );
    if (!response.ok) {
      const message = await response.text();
      throw new Error(message || "Failed to delete namespace");
    }
    await loadNamespaces(true);
  }, [loadNamespaces]);

  const toggleNamespaceHidden = useCallback(async (name, hidden) => {
    const response = await fetch(
      `${buildApiBase()}/storage/namespaces/${encodeURIComponent(name)}`,
      {
        method: "PUT",
        headers: { "Content-Type": "application/json", ...getAuthHeaders() },
        body: JSON.stringify({ hidden }),
      }
    );
    if (!response.ok) throw new Error("Failed to update namespace");
    await loadNamespaces(true);
  }, [loadNamespaces]);

  return {
    namespaces,
    activeNamespace,
    setActiveNamespace,
    loading,
    normalizeNamespace,
    loadNamespaces,
    createNamespace,
    deleteNamespace,
    toggleNamespaceHidden,
  };
}
