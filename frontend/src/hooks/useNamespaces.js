import { useState, useCallback, useRef, useEffect } from "react";
import { buildStorageBase, getAuthHeaders } from "@/lib/api";
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
  const abortControllerRef = useRef(null);

  // Cleanup abort controller on unmount
  useEffect(() => {
    return () => {
      if (abortControllerRef.current) {
        abortControllerRef.current.abort();
      }
    };
  }, []);

  const normalizeNamespace = (value) => (value && value.trim() ? value.trim() : DEFAULT_NAMESPACE);

  const loadNamespaces = useCallback(async (forceRefresh = false) => {
    // Skip if already loaded and not forcing refresh
    if (namespacesLoaded && !forceRefresh) {
      return cachedNamespaces;
    }

    // Abort any in-flight request
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
    }
    abortControllerRef.current = new AbortController();

    try {
      setLoading(true);
      const response = await fetch(`${buildStorageBase()}/storage/namespaces`, {
        headers: getAuthHeaders(),
        signal: abortControllerRef.current.signal,
      });
      if (!response.ok) throw new Error("Failed to load namespaces");
      const payload = await response.json();
      const sorted = payload
        .map((item) => ({
          name: item.name,
          count: item.count ?? 0,
          hidden: item.hidden ?? false,
          visibility: item.visibility ?? 2, // Default to public
        }))
        .sort((a, b) => a.name.localeCompare(b.name));
      setNamespaces(sorted);
      cachedNamespaces = sorted;
      namespacesLoaded = true;
      return sorted;
    } catch (err) {
      if (err.name === "AbortError") return [];
      console.warn(err.message);
      return [];
    } finally {
      setLoading(false);
    }
  }, []);

  // visibility: 0=private, 1=visible (unlisted), 2=public
  const createNamespace = useCallback(async (name, visibility = 2) => {
    const normalizedName = normalizeNamespace(name);
    const response = await fetch(`${buildStorageBase()}/storage/namespaces`, {
      method: "POST",
      headers: { "Content-Type": "application/json", ...getAuthHeaders() },
      body: JSON.stringify({ name: normalizedName, visibility }),
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
      `${buildStorageBase()}/storage/namespaces/${encodeURIComponent(name)}`,
      { method: "DELETE", headers: getAuthHeaders() }
    );
    if (!response.ok) {
      const message = await response.text();
      throw new Error(message || "Failed to delete namespace");
    }
    await loadNamespaces(true);
  }, [loadNamespaces]);

  // visibility: 0=private, 1=visible (unlisted), 2=public
  const updateNamespaceVisibility = useCallback(async (name, visibility) => {
    const response = await fetch(
      `${buildStorageBase()}/storage/namespaces/${encodeURIComponent(name)}`,
      {
        method: "PUT",
        headers: { "Content-Type": "application/json", ...getAuthHeaders() },
        body: JSON.stringify({ visibility }),
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
    updateNamespaceVisibility,
  };
}
