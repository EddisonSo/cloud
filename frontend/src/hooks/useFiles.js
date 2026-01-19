import { useState, useCallback, useRef, useEffect } from "react";
import { buildStorageBase, buildSseUrl, createTransferId, getAuthHeaders, getAuthToken } from "@/lib/api";
import { DEFAULT_NAMESPACE } from "@/lib/constants";
import { registerCacheClear } from "@/lib/cache";

// Module-level cache that persists across component mounts
const filesCache = {};  // { namespace: files[] }

// Register cache clear function
registerCacheClear(() => {
  Object.keys(filesCache).forEach((key) => delete filesCache[key]);
});

export function useFiles() {
  const [files, setFiles] = useState([]);
  const [loading, setLoading] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [uploadProgress, setUploadProgress] = useState({ bytes: 0, total: 0, active: false });
  const [downloadProgress, setDownloadProgress] = useState({});
  const [deleting, setDeleting] = useState({});
  const [status, setStatus] = useState("");
  const fileInputRef = useRef(null);
  const [selectedFileName, setSelectedFileName] = useState("No file selected");
  const currentNamespaceRef = useRef(null);
  const abortControllerRef = useRef(null);

  // Cleanup abort controller on unmount
  useEffect(() => {
    return () => {
      if (abortControllerRef.current) {
        abortControllerRef.current.abort();
      }
    };
  }, []);

  const loadFiles = useCallback(async (namespace, forceRefresh = false) => {
    const selectedNamespace = namespace || DEFAULT_NAMESPACE;
    // Skip if already loaded for this namespace and not forcing refresh
    if (filesCache[selectedNamespace] && !forceRefresh) {
      // If switching namespaces, update state from cache
      if (currentNamespaceRef.current !== selectedNamespace) {
        setFiles(filesCache[selectedNamespace]);
        currentNamespaceRef.current = selectedNamespace;
      }
      return filesCache[selectedNamespace];
    }

    // Abort any in-flight request
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
    }
    abortControllerRef.current = new AbortController();

    try {
      setLoading(true);
      const response = await fetch(
        `${buildStorageBase()}/storage/files?namespace=${encodeURIComponent(selectedNamespace)}`,
        { headers: getAuthHeaders(), signal: abortControllerRef.current.signal }
      );
      if (!response.ok) throw new Error("Failed to load files");
      const payload = await response.json();
      // Sort by modification date, most recent first
      const sorted = [...payload].sort((a, b) => (b.modified || 0) - (a.modified || 0));
      setFiles(sorted);
      filesCache[selectedNamespace] = sorted;
      currentNamespaceRef.current = selectedNamespace;
      return sorted;
    } catch (err) {
      if (err.name === "AbortError") return [];
      setStatus(err.message);
      return [];
    } finally {
      setLoading(false);
    }
  }, []);

  const clearFilesCache = useCallback((namespace) => {
    if (namespace) {
      delete filesCache[namespace];
    } else {
      Object.keys(filesCache).forEach(key => delete filesCache[key]);
    }
  }, []);

  const uploadFile = useCallback(async (namespace, onComplete, { overwrite = false } = {}) => {
    const file = fileInputRef.current?.files?.[0];
    if (!file) {
      setStatus("Choose a file to upload.");
      return { success: false };
    }

    const formData = new FormData();
    formData.append("file", file);
    const transferId = createTransferId();
    const token = getAuthToken();

    // Use SSE for progress tracking
    const sseUrl = token
      ? `${buildSseUrl(transferId)}&token=${encodeURIComponent(token)}`
      : buildSseUrl(transferId);
    const eventSource = new EventSource(sseUrl, { withCredentials: true });

    try {
      setUploading(true);
      setUploadProgress({ bytes: 0, total: file.size, active: true });
      setStatus("Uploading...");

      eventSource.onmessage = (event) => {
        try {
          const payload = JSON.parse(event.data);
          if (payload.direction !== "upload") return;
          setUploadProgress((prev) => ({
            ...prev,
            bytes: payload.bytes ?? prev.bytes,
            total: payload.total ?? prev.total,
          }));
          if (payload.done) {
            setUploadProgress((prev) => ({ ...prev, active: false }));
            eventSource.close();
          }
        } catch (err) {
          console.warn("Failed to parse upload progress", err);
        }
      };

      eventSource.onerror = () => {
        // SSE connection closed or errored - this is normal when upload completes
        eventSource.close();
      };

      // Start fetch immediately - browser can prepare upload while SSE connects
      const url = `${buildStorageBase()}/storage/upload?id=${encodeURIComponent(transferId)}&namespace=${encodeURIComponent(namespace)}${overwrite ? "&overwrite=true" : ""}`;
      const response = await fetch(url, {
        method: "POST",
        body: formData,
        headers: { "X-File-Size": file.size.toString(), ...getAuthHeaders() },
      });
      if (!response.ok) {
        const message = await response.text();
        // Return special flag for file exists conflict
        if (response.status === 409) {
          setStatus("");
          setUploading(false);
          eventSource.close();
          return { success: false, fileExists: true, fileName: file.name };
        }
        throw new Error(message || "Upload failed");
      }
      await response.json();
      setStatus(`Uploaded ${file.name}`);
      if (fileInputRef.current) fileInputRef.current.value = "";
      setSelectedFileName("No file selected");
      await loadFiles(namespace, true);
      onComplete?.();
      return { success: true };
    } catch (err) {
      setStatus(err.message);
      return { success: false };
    } finally {
      setUploading(false);
      eventSource.close();
    }
  }, [loadFiles]);

  const downloadFile = useCallback(async (file, user) => {
    const transferId = createTransferId();
    let eventSource;
    const fileKey = `${file.namespace || DEFAULT_NAMESPACE}:${file.name}`;
    const token = getAuthToken();

    if (user) {
      const sseUrl = token
        ? `${buildSseUrl(transferId)}&token=${encodeURIComponent(token)}`
        : buildSseUrl(transferId);
      eventSource = new EventSource(sseUrl, { withCredentials: true });
      setDownloadProgress((prev) => ({
        ...prev,
        [fileKey]: { bytes: 0, total: file.size, active: true },
      }));

      eventSource.onmessage = (event) => {
        try {
          const payload = JSON.parse(event.data);
          if (payload.direction !== "download") return;
          setDownloadProgress((prev) => ({
            ...prev,
            [fileKey]: {
              bytes: payload.bytes ?? prev[fileKey]?.bytes ?? 0,
              total: payload.total ?? prev[fileKey]?.total ?? file.size,
              active: !payload.done,
            },
          }));
          if (payload.done) eventSource.close();
        } catch (err) {
          console.warn("Failed to parse download progress", err);
        }
      };

      eventSource.onerror = () => {
        eventSource.close();
      };
    }

    // For downloads, pass token as query parameter since we're using a link
    let downloadUrl = `${buildStorageBase()}/storage/download?name=${encodeURIComponent(file.name)}&id=${encodeURIComponent(transferId)}&namespace=${encodeURIComponent(file.namespace || DEFAULT_NAMESPACE)}`;
    if (token) {
      downloadUrl += `&token=${encodeURIComponent(token)}`;
    }

    const link = document.createElement("a");
    link.href = downloadUrl;
    link.rel = "noopener";
    link.style.display = "none";
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    if (eventSource) eventSource.close();
  }, []);

  const deleteFile = useCallback(async (file, namespace, onComplete) => {
    const fileKey = `${file.namespace || DEFAULT_NAMESPACE}:${file.name}`;
    setDeleting((prev) => ({ ...prev, [fileKey]: true }));
    setStatus(`Deleting ${file.name}...`);
    try {
      const response = await fetch(
        `${buildStorageBase()}/storage/delete?name=${encodeURIComponent(file.name)}&namespace=${encodeURIComponent(file.namespace || DEFAULT_NAMESPACE)}`,
        { method: "DELETE", headers: getAuthHeaders() }
      );
      if (!response.ok) {
        const message = await response.text();
        throw new Error(message || "Delete failed");
      }
      setStatus(`Deleted ${file.name}`);
      await loadFiles(namespace, true);
      onComplete?.();
    } catch (err) {
      setStatus(err.message);
    } finally {
      setDeleting((prev) => ({ ...prev, [fileKey]: false }));
    }
  }, [loadFiles]);

  return {
    files,
    loading,
    uploading,
    uploadProgress,
    downloadProgress,
    deleting,
    status,
    setStatus,
    fileInputRef,
    selectedFileName,
    setSelectedFileName,
    loadFiles,
    clearFilesCache,
    uploadFile,
    downloadFile,
    deleteFile,
  };
}
