import { useState, useEffect, useCallback } from "react";
import { fetchMetricsHistory } from "@/lib/api";

export function useMetricsHistory({ timeRange = "1h", nodeName = null, refreshInterval = 60000 } = {}) {
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);

  const getTimeRange = useCallback(() => {
    const end = new Date();
    let start;
    switch (timeRange) {
      case "1h":
        start = new Date(end.getTime() - 60 * 60 * 1000);
        break;
      case "6h":
        start = new Date(end.getTime() - 6 * 60 * 60 * 1000);
        break;
      case "24h":
        start = new Date(end.getTime() - 24 * 60 * 60 * 1000);
        break;
      case "7d":
        start = new Date(end.getTime() - 7 * 24 * 60 * 60 * 1000);
        break;
      default:
        start = new Date(end.getTime() - 60 * 60 * 1000);
    }
    return { start, end };
  }, [timeRange]);

  const refetch = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const { start, end } = getTimeRange();
      const result = await fetchMetricsHistory({ start, end, nodeName });
      setData(result);
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  }, [getTimeRange, nodeName]);

  useEffect(() => {
    refetch();

    if (refreshInterval > 0) {
      const interval = setInterval(refetch, refreshInterval);
      return () => clearInterval(interval);
    }
  }, [refetch, refreshInterval]);

  return { data, loading, error, refetch };
}
