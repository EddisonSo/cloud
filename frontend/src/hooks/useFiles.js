import { useState, useCallback, useRef } from "react";
import { buildApiBase, buildWsUrl, createTransferId, waitForSocket } from "@/lib/api";
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
    try {
      setLoading(true);
      const response = await fetch(
        `${buildApiBase()}/storage/files?namespace=${encodeURIComponent(selectedNamespace)}`,
        { credentials: "include" }
      );
      if (!response.ok) throw new Error("Failed to load files");
      const payload = await response.json();
      setFiles(payload);
      filesCache[selectedNamespace] = payload;
      currentNamespaceRef.current = selectedNamespace;
      return payload;
    } catch (err) {
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

  const uploadFile = useCallback(async (namespace, onComplete) => {
    const file = fileInputRef.current?.files?.[0];
    if (!file) {
      setStatus("Choose a file to upload.");
      return;
    }

    const formData = new FormData();
    formData.append("file", file);
    const transferId = createTransferId();
    const socket = new WebSocket(buildWsUrl(transferId));

    try {
      setUploading(true);
      setUploadProgress({ bytes: 0, total: file.size, active: true });
      setStatus("Uploading...");

      socket.onmessage = (event) => {
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
            socket.close();
          }
        } catch (err) {
          console.warn("Failed to parse upload progress", err);
        }
      };

      await waitForSocket(socket, 2000).catch(() => {});
      const response = await fetch(
        `${buildApiBase()}/storage/upload?id=${encodeURIComponent(transferId)}&namespace=${encodeURIComponent(namespace)}`,
        {
          method: "POST",
          body: formData,
          credentials: "include",
          headers: { "X-File-Size": file.size.toString() },
        }
      );
      if (!response.ok) {
        const message = await response.text();
        throw new Error(message || "Upload failed");
      }
      await response.json();
      setStatus(`Uploaded ${file.name}`);
      if (fileInputRef.current) fileInputRef.current.value = "";
      setSelectedFileName("No file selected");
      await loadFiles(namespace, true);
      onComplete?.();
    } catch (err) {
      setStatus(err.message);
    } finally {
      setUploading(false);
      socket.close();
    }
  }, [loadFiles]);

  const downloadFile = useCallback(async (file, user) => {
    const transferId = createTransferId();
    let socket;
    const fileKey = `${file.namespace || DEFAULT_NAMESPACE}:${file.name}`;

    if (user) {
      socket = new WebSocket(buildWsUrl(transferId));
      setDownloadProgress((prev) => ({
        ...prev,
        [fileKey]: { bytes: 0, total: file.size, active: true },
      }));

      socket.onmessage = (event) => {
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
          if (payload.done) socket.close();
        } catch (err) {
          console.warn("Failed to parse download progress", err);
        }
      };

      await waitForSocket(socket, 2000).catch(() => {});
    }

    const link = document.createElement("a");
    link.href = `${buildApiBase()}/storage/download?name=${encodeURIComponent(file.name)}&id=${encodeURIComponent(transferId)}&namespace=${encodeURIComponent(file.namespace || DEFAULT_NAMESPACE)}`;
    link.rel = "noopener";
    link.style.display = "none";
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    if (socket) socket.close();
  }, []);

  const deleteFile = useCallback(async (file, namespace, onComplete) => {
    const fileKey = `${file.namespace || DEFAULT_NAMESPACE}:${file.name}`;
    setDeleting((prev) => ({ ...prev, [fileKey]: true }));
    setStatus(`Deleting ${file.name}...`);
    try {
      const response = await fetch(
        `${buildApiBase()}/storage/delete?name=${encodeURIComponent(file.name)}&namespace=${encodeURIComponent(file.namespace || DEFAULT_NAMESPACE)}`,
        { method: "DELETE", credentials: "include" }
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
