import { useState, useCallback, useRef, useEffect } from "react";
import { buildComputeBase, getAuthHeaders } from "@/lib/api";

export function useContainerAccess() {
  const [container, setContainer] = useState(null);
  const [loading, setLoading] = useState(false);
  const [sshEnabled, setSshEnabled] = useState(false);
  const [savingSSH, setSavingSSH] = useState(false);
  const [ingressRules, setIngressRules] = useState([]);
  const [addingRule, setAddingRule] = useState(false);
  const abortControllerRef = useRef(null);

  // Cleanup abort controller on unmount
  useEffect(() => {
    return () => {
      if (abortControllerRef.current) {
        abortControllerRef.current.abort();
      }
    };
  }, []);

  const openAccess = useCallback(async (containerData) => {
    setContainer(containerData);
    setLoading(true);
    setSshEnabled(containerData.ssh_enabled || false);
    setIngressRules([]);

    // Abort any in-flight request
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
    }
    abortControllerRef.current = new AbortController();

    try {
      const [sshRes, ingressRes] = await Promise.all([
        fetch(`${buildComputeBase()}/compute/containers/${containerData.id}/ssh`, {
          headers: getAuthHeaders(),
          signal: abortControllerRef.current.signal,
        }),
        fetch(`${buildComputeBase()}/compute/containers/${containerData.id}/ingress`, {
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
    } catch (err) {
      if (err.name === "AbortError") return;
      console.warn("Failed to load access settings:", err.message);
    } finally {
      setLoading(false);
    }
  }, []);

  const closeAccess = useCallback(() => {
    setContainer(null);
    setSshEnabled(false);
    setIngressRules([]);
  }, []);

  const toggleSSH = useCallback(async (updateContainers) => {
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

  const addIngressRule = useCallback(async (port, targetPort, updateContainers) => {
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
      const rule = await response.json();
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

  const removeIngressRule = useCallback(async (port, updateContainers) => {
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

  return {
    container,
    loading,
    sshEnabled,
    savingSSH,
    ingressRules,
    addingRule,
    openAccess,
    closeAccess,
    toggleSSH,
    addIngressRule,
    removeIngressRule,
  };
}
