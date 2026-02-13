import { useState, useCallback, useRef, useEffect } from "react";
import { buildComputeBase, getAuthHeaders } from "@/lib/api";
import type { Container, IngressRule } from "@/types";

export function useContainerAccess() {
  const [container, setContainer] = useState<Container | null>(null);
  const [loading, setLoading] = useState<boolean>(false);
  const [sshEnabled, setSshEnabled] = useState<boolean>(false);
  const [savingSSH, setSavingSSH] = useState<boolean>(false);
  const [ingressRules, setIngressRules] = useState<IngressRule[]>([]);
  const [addingRule, setAddingRule] = useState<boolean>(false);
  const [mountPaths, setMountPaths] = useState<string[]>([]);
  const [savingMounts, setSavingMounts] = useState<boolean>(false);
  const abortControllerRef = useRef<AbortController | null>(null);

  // Cleanup abort controller on unmount
  useEffect(() => {
    return () => {
      if (abortControllerRef.current) {
        abortControllerRef.current.abort();
      }
    };
  }, []);

  const openAccess = useCallback(async (containerData: Container) => {
    setContainer(containerData);
    setLoading(true);
    setSshEnabled(containerData.ssh_enabled || false);
    setIngressRules([]);
    setMountPaths([]);

    // Abort any in-flight request
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
    }
    abortControllerRef.current = new AbortController();

    try {
      const [sshRes, ingressRes, mountsRes] = await Promise.all([
        fetch(`${buildComputeBase()}/compute/containers/${containerData.id}/ssh`, {
          headers: getAuthHeaders(),
          signal: abortControllerRef.current.signal,
        }),
        fetch(`${buildComputeBase()}/compute/containers/${containerData.id}/ingress`, {
          headers: getAuthHeaders(),
          signal: abortControllerRef.current.signal,
        }),
        fetch(`${buildComputeBase()}/compute/containers/${containerData.id}/mounts`, {
          headers: getAuthHeaders(),
          signal: abortControllerRef.current.signal,
        }),
      ]);
      if (sshRes.ok) {
        const data = await sshRes.json();
        setSshEnabled(data.ssh_enabled);
      }
      if (ingressRes.ok) {
        const data = await ingressRes.json();
        setIngressRules(data.rules || []);
      }
      if (mountsRes.ok) {
        const data = await mountsRes.json();
        setMountPaths(data.mount_paths || []);
      }
    } catch (err) {
      if ((err as Error).name === "AbortError") return;
      console.warn("Failed to load access settings:", (err as Error).message);
    } finally {
      setLoading(false);
    }
  }, []);

  const closeAccess = useCallback(() => {
    setContainer(null);
    setSshEnabled(false);
    setIngressRules([]);
    setMountPaths([]);
  }, []);

  const toggleSSH = useCallback(async (updateContainers?: React.Dispatch<React.SetStateAction<Container[]>>) => {
    if (!container || savingSSH) return;
    const newValue = !sshEnabled;
    setSshEnabled(newValue);
    setSavingSSH(true);
    try {
      const response = await fetch(
        `${buildComputeBase()}/compute/containers/${container.id}/ssh`,
        {
          method: "PUT",
          headers: { "Content-Type": "application/json", ...getAuthHeaders() },
          body: JSON.stringify({ ssh_enabled: newValue }),
        }
      );
      if (!response.ok) {
        setSshEnabled(!newValue);
        const msg = await response.text();
        throw new Error(msg || "Failed to update SSH access");
      }
      updateContainers?.((prev) =>
        prev.map((c) => c.id === container.id ? { ...c, ssh_enabled: newValue } : c)
      );
    } finally {
      setSavingSSH(false);
    }
  }, [container, sshEnabled, savingSSH]);

  const addIngressRule = useCallback(async (port: number, targetPort: number, updateContainers?: React.Dispatch<React.SetStateAction<Container[]>>): Promise<IngressRule | undefined> => {
    if (!container || addingRule) return;
    setAddingRule(true);
    try {
      const response = await fetch(
        `${buildComputeBase()}/compute/containers/${container.id}/ingress`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json", ...getAuthHeaders() },
          body: JSON.stringify({ port, target_port: targetPort || port }),
        }
      );
      if (!response.ok) {
        const msg = await response.text();
        throw new Error(msg || "Failed to add rule");
      }
      const rule: IngressRule = await response.json();
      setIngressRules((prev) => [...prev, rule]);
      if (port === 443) {
        updateContainers?.((prev) =>
          prev.map((c) => c.id === container.id ? { ...c, https_enabled: true } : c)
        );
      }
      return rule;
    } finally {
      setAddingRule(false);
    }
  }, [container, addingRule]);

  const removeIngressRule = useCallback(async (port: number, updateContainers?: React.Dispatch<React.SetStateAction<Container[]>>) => {
    if (!container) return;
    const response = await fetch(
      `${buildComputeBase()}/compute/containers/${container.id}/ingress/${port}`,
      { method: "DELETE", headers: getAuthHeaders() }
    );
    if (!response.ok) throw new Error("Failed to remove rule");
    setIngressRules((prev) => prev.filter((r) => r.port !== port));
    if (port === 443) {
      updateContainers?.((prev) =>
        prev.map((c) => c.id === container.id ? { ...c, https_enabled: false } : c)
      );
    }
  }, [container]);

  const updateMountPaths = useCallback(async (newPaths: string[]) => {
    if (!container || savingMounts) return;
    setSavingMounts(true);
    try {
      const response = await fetch(
        `${buildComputeBase()}/compute/containers/${container.id}/mounts`,
        {
          method: "PUT",
          headers: { "Content-Type": "application/json", ...getAuthHeaders() },
          body: JSON.stringify({ mount_paths: newPaths }),
        }
      );
      if (!response.ok) {
        const msg = await response.text();
        throw new Error(msg || "Failed to update mount paths");
      }
      setMountPaths(newPaths);
    } finally {
      setSavingMounts(false);
    }
  }, [container, savingMounts]);

  return {
    container,
    loading,
    sshEnabled,
    savingSSH,
    ingressRules,
    addingRule,
    mountPaths,
    savingMounts,
    openAccess,
    closeAccess,
    toggleSSH,
    addIngressRule,
    removeIngressRule,
    updateMountPaths,
  };
}
