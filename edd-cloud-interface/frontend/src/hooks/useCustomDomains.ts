import { useState, useCallback, useEffect, useRef } from "react";
import { buildServiceBase, getAuthHeaders } from "@/lib/api";
import type { CustomDomain, CreateCustomDomainData, CloudflareStatus } from "@/types";

export function useCustomDomains(user: string | null) {
  const [domains, setDomains] = useState<CustomDomain[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [cloudflare, setCloudflare] = useState<CloudflareStatus | null>(null);
  const abortRef = useRef<AbortController | null>(null);

  // Abort in-flight request on unmount
  useEffect(() => {
    return () => {
      abortRef.current?.abort();
    };
  }, []);

  const base = () => buildServiceBase("networking");

  const loadDomains = useCallback(async (): Promise<CustomDomain[]> => {
    abortRef.current?.abort();
    abortRef.current = new AbortController();
    try {
      setLoading(true);
      setError("");
      const res = await fetch(`${base()}/api/domains`, {
        headers: getAuthHeaders(),
        signal: abortRef.current.signal,
      });
      if (!res.ok) {
        if (res.status === 401) {
          setError("Sign in to manage domains");
          return [];
        }
        throw new Error("Failed to load domains");
      }
      const payload = await res.json();
      const list: CustomDomain[] = payload.domains || [];
      setDomains(list);
      return list;
    } catch (err) {
      if ((err as Error).name === "AbortError") return [];
      setError((err as Error).message);
      return [];
    } finally {
      setLoading(false);
    }
  }, []);

  const createDomain = useCallback(
    async (data: CreateCustomDomainData): Promise<CustomDomain> => {
      const res = await fetch(`${base()}/api/domains`, {
        method: "POST",
        headers: { "Content-Type": "application/json", ...getAuthHeaders() },
        body: JSON.stringify(data),
      });
      if (!res.ok) {
        throw new Error((await res.text()) || "Failed to create domain");
      }
      const created: CustomDomain = await res.json();
      await loadDomains();
      return created;
    },
    [loadDomains]
  );

  const verifyDomain = useCallback(
    async (id: string): Promise<{ status: string; detail?: string }> => {
      const res = await fetch(`${base()}/api/domains/${id}/verify`, {
        method: "POST",
        headers: getAuthHeaders(),
      });
      if (!res.ok) {
        throw new Error("Failed to verify domain");
      }
      const result = await res.json();
      await loadDomains();
      return result;
    },
    [loadDomains]
  );

  const deleteDomain = useCallback(
    async (id: string): Promise<void> => {
      const res = await fetch(`${base()}/api/domains/${id}`, {
        method: "DELETE",
        headers: getAuthHeaders(),
      });
      if (!res.ok) {
        throw new Error("Failed to delete domain");
      }
      await loadDomains();
    },
    [loadDomains]
  );

  const loadCloudflare = useCallback(async () => {
    const res = await fetch(`${base()}/api/cloudflare-token`, { headers: getAuthHeaders() });
    if (res.ok) setCloudflare(await res.json());
  }, []);

  const saveCloudflareToken = useCallback(async (token: string): Promise<CloudflareStatus> => {
    const res = await fetch(`${base()}/api/cloudflare-token`, {
      method: "PUT",
      headers: { "Content-Type": "application/json", ...getAuthHeaders() },
      body: JSON.stringify({ token }),
    });
    if (!res.ok) throw new Error((await res.text()) || "Failed to save token");
    const status = (await res.json()) as CloudflareStatus;
    setCloudflare(status);
    return status;
  }, []);

  const deleteCloudflareToken = useCallback(async () => {
    const res = await fetch(`${base()}/api/cloudflare-token`, { method: "DELETE", headers: getAuthHeaders() });
    if (!res.ok) throw new Error("Failed to disconnect");
    setCloudflare({ configured: false });
  }, []);

  // Load domains and cloudflare status whenever the authenticated user changes
  useEffect(() => {
    if (user) {
      loadDomains();
      loadCloudflare();
    } else {
      setDomains([]);
      setCloudflare(null);
    }
  }, [user, loadDomains, loadCloudflare]);

  return {
    domains,
    loading,
    error,
    setError,
    loadDomains,
    createDomain,
    verifyDomain,
    deleteDomain,
    cloudflare,
    loadCloudflare,
    saveCloudflareToken,
    deleteCloudflareToken,
  };
}
