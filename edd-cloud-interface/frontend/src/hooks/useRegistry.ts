import { useState, useCallback, useEffect } from "react";
import { buildRegistryBase, getAuthHeaders } from "@/lib/api";

export interface RepoInfo {
  name: string;
  visibility: number;
  owner_id: string;
  tag_count: number;
  total_size: number;
  last_pushed: string;
}

export interface TagInfo {
  name: string;
  digest: string;
  size: number;
  pushed_at: string;
}

export function useRegistry() {
  const [repos, setRepos] = useState<RepoInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string>("");

  const loadRepos = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const resp = await fetch(`${buildRegistryBase()}/api/repos`, {
        headers: getAuthHeaders(),
      });
      if (!resp.ok) throw new Error(`${resp.status}`);
      const data = await resp.json();
      setRepos(data.repositories || []);
    } catch (e: any) {
      setError(e.message || "Failed to load repositories");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { loadRepos(); }, [loadRepos]);

  const loadTags = useCallback(async (repoName: string): Promise<TagInfo[]> => {
    const resp = await fetch(`${buildRegistryBase()}/api/repos/${repoName}/tags`, { headers: getAuthHeaders() });
    if (!resp.ok) return [];
    const data = await resp.json();
    return data.tags || [];
  }, []);

  const deleteTag = useCallback(async (repoName: string, tag: string) => {
    await fetch(`${buildRegistryBase()}/api/repos/${repoName}/tags/${tag}`, {
      method: "DELETE",
      headers: getAuthHeaders(),
    });
  }, []);

  return { repos, loading, error, loadRepos, loadTags, deleteTag };
}
