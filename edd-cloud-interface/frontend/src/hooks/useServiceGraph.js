import { useState, useEffect, useCallback, useMemo } from "react";
import { fetchServiceDependencies } from "@/lib/api";

export function useServiceGraph(healthData) {
  const [graphData, setGraphData] = useState({ nodes: [], edges: [] });
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);

  const refetch = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const result = await fetchServiceDependencies();
      setGraphData(result);
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    refetch();
  }, [refetch]);

  // Merge live health status into nodes
  const nodesWithHealth = useMemo(() => {
    if (!graphData.nodes || !healthData?.nodes) {
      return graphData.nodes || [];
    }

    // Build a map of pod/service health from cluster health data
    const healthMap = new Map();

    // Add node health status
    for (const node of healthData.nodes || []) {
      const isHealthy = (node.conditions || []).every((c) => c.status === "False");
      healthMap.set(node.name, isHealthy ? "healthy" : "degraded");
    }

    // For services, we could check pods here if available
    // For now, assume services are healthy if cluster is ok

    return graphData.nodes.map((node) => ({
      ...node,
      health: healthMap.get(node.id) || "unknown",
    }));
  }, [graphData.nodes, healthData]);

  return {
    nodes: nodesWithHealth,
    edges: graphData.edges || [],
    loading,
    error,
    refetch,
  };
}
