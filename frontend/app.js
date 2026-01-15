const { useEffect, useRef, useState } = React;

const emptyState = "No files yet. Upload your first file to share it.";
const defaultNamespace = "default";
const hiddenNamespace = "hidden";

function formatBytes(bytes) {
  if (!Number.isFinite(bytes) || bytes === 0) {
    return "0 B";
  }
  const units = ["B", "KB", "MB", "GB", "TB"];
  const index = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  const value = bytes / Math.pow(1024, index);
  return `${value.toFixed(value >= 10 || index === 0 ? 0 : 1)} ${units[index]}`;
}

function formatTimestamp(unixSeconds) {
  if (!unixSeconds) return "-";
  const date = new Date(unixSeconds * 1000);
  const now = new Date();
  const diffMs = now - date;
  const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24));

  if (diffDays === 0) {
    return date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
  } else if (diffDays < 7) {
    return date.toLocaleDateString([], { weekday: "short", hour: "2-digit", minute: "2-digit" });
  } else {
    return date.toLocaleDateString([], { month: "short", day: "numeric", year: "numeric" });
  }
}

function resolveApiHost() {
  // Use same-origin for API calls - ingress routes /api/* to backend
  return window.location.host;
}

function buildApiBase() {
  return `${window.location.protocol}//${resolveApiHost()}`;
}

function buildWsBase() {
  const protocol = window.location.protocol === "https:" ? "wss" : "ws";
  return `${protocol}://${resolveApiHost()}`;
}

function buildWsUrl(id) {
  return `${buildWsBase()}/ws?id=${encodeURIComponent(id)}`;
}

function buildClusterInfoUrl() {
  return `${buildApiBase()}/cluster-info`;
}

function buildClusterInfoWsUrl() {
  return `${buildWsBase()}/ws/cluster-info`;
}

function createTransferId() {
  if (window.crypto?.randomUUID) {
    return window.crypto.randomUUID();
  }
  return `transfer-${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

function waitForSocket(socket, timeoutMs = 1000) {
  if (socket.readyState === WebSocket.OPEN) {
    return Promise.resolve();
  }
  return new Promise((resolve, reject) => {
    const timeout = setTimeout(() => reject(new Error("WebSocket timeout")), timeoutMs);
    socket.addEventListener("open", () => {
      clearTimeout(timeout);
      resolve();
    });
    socket.addEventListener("error", () => {
      clearTimeout(timeout);
      reject(new Error("WebSocket error"));
    });
  });
}

function progressPercent(bytes, total) {
  if (!total) {
    return 0;
  }
  return Math.min(100, Math.round((bytes / total) * 100));
}

function App() {
  const [files, setFiles] = useState([]);
  const [status, setStatus] = useState("Ready to share.");
  const [loading, setLoading] = useState(true);
  const [uploading, setUploading] = useState(false);
  const [authChecked, setAuthChecked] = useState(false);
  const [user, setUser] = useState(null);
  const [loginForm, setLoginForm] = useState({ username: "", password: "" });
  const [loginError, setLoginError] = useState("");
  const [activeTab, setActiveTab] = useState("storage");
  const [health, setHealth] = useState({ cluster_ok: false, nodes: [] });
  const [healthError, setHealthError] = useState("");
  const [healthLoading, setHealthLoading] = useState(false);
  const [lastHealthCheck, setLastHealthCheck] = useState(null);
  const [healthRefreshTick, setHealthRefreshTick] = useState(0);
  // Logs state
  const [logs, setLogs] = useState([]);
  const [logsConnected, setLogsConnected] = useState(false);
  const [logsError, setLogsError] = useState("");
  const [logSourceFilter, setLogSourceFilter] = useState("");
  const [logLevelFilter, setLogLevelFilter] = useState("DEBUG");
  const [logSources, setLogSources] = useState([]);
  const [logsAutoScroll, setLogsAutoScroll] = useState(true);
  const [logUpdateFrequency, setLogUpdateFrequency] = useState(0); // 0 = unlimited
  const [lastLogTime, setLastLogTime] = useState(null);
  const logsAutoScrollRef = useRef(true);
  const logsContainerRef = useRef(null);
  const logsEndRef = useRef(null);
  const logBufferRef = useRef([]);
  const lastFlushRef = useRef(0);
  const [uploadProgress, setUploadProgress] = useState({
    bytes: 0,
    total: 0,
    active: false,
  });
  const [selectedFileName, setSelectedFileName] = useState("No file selected");
  const [downloadProgress, setDownloadProgress] = useState({});
  const [deleting, setDeleting] = useState({});
  const [namespaces, setNamespaces] = useState([]);
  // Compute state
  const [containers, setContainers] = useState([]);
  const [containersLoading, setContainersLoading] = useState(false);
  const [containersError, setContainersError] = useState("");
  const [sshKeys, setSshKeys] = useState([]);
  const [sshKeysLoading, setSshKeysLoading] = useState(false);
  const [computeView, setComputeView] = useState("containers"); // containers, ssh-keys
  const [showCreateContainer, setShowCreateContainer] = useState(false);
  const [newContainer, setNewContainer] = useState({ name: "", memory_mb: 512, storage_gb: 5 });
  const [selectedSshKeyIds, setSelectedSshKeyIds] = useState([]);
  const [creatingContainer, setCreatingContainer] = useState(false);
  const [newSshKey, setNewSshKey] = useState({ name: "", public_key: "" });
  const [addingSshKey, setAddingSshKey] = useState(false);
  const [containerActions, setContainerActions] = useState({}); // {id: 'starting'|'stopping'|'deleting'}
  const [activeNamespace, setActiveNamespace] = useState("");
  const [namespaceInput, setNamespaceInput] = useState("");
  const [namespaceHidden, setNamespaceHidden] = useState(false);
  const [namespaceDeleteTarget, setNamespaceDeleteTarget] = useState(null);
  const [showNamespaceView, setShowNamespaceView] = useState(false);
  const fileInputRef = useRef(null);
  const modalRef = useRef(null);
  const modalCancelRef = useRef(null);
  const lastActiveElementRef = useRef(null);
  const navItems = [
    { id: "storage", label: "Storage" },
    { id: "compute", label: "Compute" },
    { id: "message-queue", label: "Message Queue" },
    { id: "datastore", label: "Datastore" },
    { id: "health", label: "Health" },
    { id: "logs", label: "Logs" },
  ];

  const tabCopy = {
    storage: {
      eyebrow: "Cloud Storage",
      title: "Simple File Share",
      lead: "Manage shared assets with clear status, fast uploads, and controlled access.",
    },
    compute: {
      eyebrow: "Compute Services",
      title: "Dev Containers",
      lead: "Manage dev environment containers with dedicated IPs and SSH access.",
    },
    "message-queue": {
      eyebrow: "Messaging",
      title: "Message Queue",
      lead: "Queue and stream services are not available yet, but the surface is ready.",
    },
    datastore: {
      eyebrow: "Data Systems",
      title: "Datastore",
      lead: "Datastore provisioning is coming soon with managed database workflows.",
    },
    health: {
      eyebrow: "Operations",
      title: "Health Monitor",
      lead: "Live telemetry for master connectivity and chunkserver status.",
    },
    logs: {
      eyebrow: "Observability",
      title: "Cluster Logs",
      lead: "Real-time log streaming from all cluster services.",
    },
  };

  const activeCopy = tabCopy[activeTab] ?? tabCopy.storage;

  const normalizeNamespace = (value) => (value && value.trim() ? value.trim() : defaultNamespace);

  useEffect(() => {
    if (!namespaceDeleteTarget) {
      return undefined;
    }
    lastActiveElementRef.current = document.activeElement;
    if (modalCancelRef.current) {
      modalCancelRef.current.focus();
    }
    const onKeyDown = (event) => {
      if (event.key === "Escape") {
        setNamespaceDeleteTarget(null);
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => {
      window.removeEventListener("keydown", onKeyDown);
      const previous = lastActiveElementRef.current;
      if (previous && typeof previous.focus === "function") {
        previous.focus();
      }
    };
  }, [namespaceDeleteTarget]);

  const loadFiles = async (namespace = activeNamespace) => {
    try {
      setLoading(true);
      const selectedNamespace = normalizeNamespace(namespace);
      if (!selectedNamespace) {
        setFiles([]);
        return;
      }
      const response = await fetch(
        `${buildApiBase()}/storage/files?namespace=${encodeURIComponent(selectedNamespace)}`
      );
      if (!response.ok) {
        throw new Error("Failed to load files");
      }
      const payload = await response.json();
      setFiles(payload);
    } catch (err) {
      setStatus(err.message);
    } finally {
      setLoading(false);
    }
  };

  const loadNamespaces = async () => {
    try {
      const response = await fetch(`${buildApiBase()}/storage/namespaces`);
      if (!response.ok) {
        throw new Error("Failed to load namespaces");
      }
      const payload = await response.json();
      const sorted = payload
        .map((item) => ({
          name: item.name,
          count: item.count ?? 0,
          hidden: item.hidden ?? false,
        }))
        .sort((a, b) => a.name.localeCompare(b.name));
      setNamespaces(sorted);
      if (!sorted.find((item) => item.name === activeNamespace)) {
        setActiveNamespace(sorted[0]?.name || defaultNamespace);
      }
    } catch (err) {
      console.warn(err.message);
    }
  };

  // Compute data loading functions
  const loadContainers = async () => {
    try {
      setContainersLoading(true);
      setContainersError("");
      const response = await fetch(`${buildApiBase()}/compute/containers`, {
        credentials: "include",
      });
      if (!response.ok) {
        if (response.status === 401) {
          setContainersError("Sign in to manage containers");
          return;
        }
        throw new Error("Failed to load containers");
      }
      const payload = await response.json();
      setContainers(payload.containers || []);
    } catch (err) {
      setContainersError(err.message);
    } finally {
      setContainersLoading(false);
    }
  };

  const loadSshKeys = async () => {
    try {
      setSshKeysLoading(true);
      const response = await fetch(`${buildApiBase()}/compute/ssh-keys`, {
        credentials: "include",
      });
      if (!response.ok) return;
      const payload = await response.json();
      setSshKeys(payload.ssh_keys || []);
    } catch (err) {
      console.warn("Failed to load SSH keys:", err.message);
    } finally {
      setSshKeysLoading(false);
    }
  };

  const handleCreateContainer = async (e) => {
    e.preventDefault();
    if (!newContainer.name.trim()) return;
    if (selectedSshKeyIds.length === 0) {
      setContainersError("Select at least one SSH key");
      return;
    }
    try {
      setCreatingContainer(true);
      const response = await fetch(`${buildApiBase()}/compute/containers`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify({
          name: newContainer.name.trim(),
          memory_mb: newContainer.memory_mb,
          storage_gb: newContainer.storage_gb,
          ssh_key_ids: selectedSshKeyIds,
        }),
      });
      if (!response.ok) {
        const message = await response.text();
        throw new Error(message || "Failed to create container");
      }
      setNewContainer({ name: "", memory_mb: 512, storage_gb: 5 });
      setSelectedSshKeyIds([]);
      setShowCreateContainer(false);
      await loadContainers();
    } catch (err) {
      setContainersError(err.message);
    } finally {
      setCreatingContainer(false);
    }
  };

  const handleContainerAction = async (id, action) => {
    setContainerActions((prev) => ({ ...prev, [id]: action }));
    try {
      const method = action === "deleting" ? "DELETE" : "POST";
      const endpoint =
        action === "deleting"
          ? `${buildApiBase()}/compute/containers/${id}`
          : `${buildApiBase()}/compute/containers/${id}/${action === "starting" ? "start" : "stop"}`;
      const response = await fetch(endpoint, { method, credentials: "include" });
      if (!response.ok) {
        const message = await response.text();
        throw new Error(message || `Failed to ${action.replace("ing", "")} container`);
      }
      await loadContainers();
    } catch (err) {
      setContainersError(err.message);
    } finally {
      setContainerActions((prev) => ({ ...prev, [id]: null }));
    }
  };

  const handleAddSshKey = async (e) => {
    e.preventDefault();
    if (!newSshKey.name.trim() || !newSshKey.public_key.trim()) return;
    try {
      setAddingSshKey(true);
      const response = await fetch(`${buildApiBase()}/compute/ssh-keys`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify({
          name: newSshKey.name.trim(),
          public_key: newSshKey.public_key.trim(),
        }),
      });
      if (!response.ok) {
        const message = await response.text();
        throw new Error(message || "Failed to add SSH key");
      }
      setNewSshKey({ name: "", public_key: "" });
      await loadSshKeys();
    } catch (err) {
      setContainersError(err.message);
    } finally {
      setAddingSshKey(false);
    }
  };

  const handleDeleteSshKey = async (id) => {
    try {
      const response = await fetch(`${buildApiBase()}/compute/ssh-keys/${id}`, {
        method: "DELETE",
        credentials: "include",
      });
      if (!response.ok) throw new Error("Failed to delete SSH key");
      await loadSshKeys();
    } catch (err) {
      setContainersError(err.message);
    }
  };

  useEffect(() => {
    const checkSession = async () => {
      try {
        const response = await fetch(`${buildApiBase()}/api/session`);
        if (!response.ok) {
          setUser(null);
          return;
        }
        const payload = await response.json();
        setUser(payload.username);
      } catch (err) {
        setUser(null);
      } finally {
        setAuthChecked(true);
      }
    };
    checkSession();
  }, []);

  useEffect(() => {
    if (!authChecked) {
      return;
    }
    loadNamespaces();
  }, [authChecked, user]);

  useEffect(() => {
    const applyState = (state) => {
      if (!state || state.view !== "namespace") {
        setShowNamespaceView(false);
        setActiveNamespace("");
        return;
      }
      setActiveNamespace(state.namespace || "");
      setShowNamespaceView(Boolean(state.namespace));
    };

    applyState(window.history.state);

    const onPopState = (event) => {
      applyState(event.state);
    };

    window.addEventListener("popstate", onPopState);
    return () => window.removeEventListener("popstate", onPopState);
  }, []);

  useEffect(() => {
    if (activeTab !== "storage") {
      return;
    }
    if (!showNamespaceView || !activeNamespace) {
      setFiles([]);
      return;
    }
    loadFiles(activeNamespace);
  }, [activeNamespace, activeTab, showNamespaceView]);

  // Load compute data when tab is active
  useEffect(() => {
    if (!user || activeTab !== "compute") {
      return;
    }
    loadContainers();
    loadSshKeys();
  }, [user, activeTab]);

  // Compute WebSocket for real-time container status updates
  useEffect(() => {
    if (!user || activeTab !== "compute") {
      return;
    }

    let ws = null;
    let reconnectTimeout = null;
    let isCleaningUp = false;

    const connect = () => {
      if (isCleaningUp) return;

      ws = new WebSocket(`${buildWsBase()}/compute/ws`);

      ws.onopen = () => {
        console.log("Compute WebSocket connected");
      };

      ws.onmessage = (event) => {
        try {
          const msg = JSON.parse(event.data);
          if (msg.type === "containers") {
            // Initial container list from server
            setContainers(msg.data || []);
          } else if (msg.type === "container_status") {
            // Status update for a single container
            const update = msg.data;
            setContainers((prev) =>
              prev.map((c) =>
                c.id === update.container_id
                  ? {
                      ...c,
                      status: update.status,
                      external_ip: update.external_ip || c.external_ip,
                    }
                  : c
              )
            );
          }
        } catch (err) {
          console.error("Failed to parse compute WebSocket message:", err);
        }
      };

      ws.onerror = () => {
        console.warn("Compute WebSocket error");
      };

      ws.onclose = () => {
        if (!isCleaningUp) {
          reconnectTimeout = setTimeout(connect, 5000);
        }
      };
    };

    connect();

    return () => {
      isCleaningUp = true;
      if (reconnectTimeout) clearTimeout(reconnectTimeout);
      if (ws) ws.close();
    };
  }, [user, activeTab]);

  const openNamespace = (namespace) => {
    const nextNamespace = normalizeNamespace(namespace);
    if (!nextNamespace) {
      return;
    }
    setActiveNamespace(nextNamespace);
    setShowNamespaceView(true);
    window.history.pushState({ view: "namespace", namespace: nextNamespace }, "");
  };

  const closeNamespace = () => {
    setShowNamespaceView(false);
    setActiveNamespace("");
    window.history.pushState({ view: "list" }, "");
  };

  useEffect(() => {
    if (!user || activeTab !== "health") {
      return;
    }

    let ws = null;
    let reconnectTimeout = null;

    const connect = () => {
      setHealthLoading(true);
      setHealthError("");

      ws = new WebSocket(buildClusterInfoWsUrl());

      ws.onopen = () => {
        setHealthLoading(false);
        setHealthError("");
      };

      ws.onmessage = (event) => {
        try {
          const payload = JSON.parse(event.data);
          setHealth({ cluster_ok: true, nodes: payload.nodes || [] });
          setLastHealthCheck(new Date());
        } catch (err) {
          console.error("Failed to parse cluster info:", err);
        }
      };

      ws.onerror = () => {
        setHealthError("WebSocket error");
        setHealth({ cluster_ok: false, nodes: [] });
      };

      ws.onclose = () => {
        setHealthLoading(false);
        // Reconnect after 5 seconds
        reconnectTimeout = setTimeout(connect, 5000);
      };
    };

    connect();

    return () => {
      if (reconnectTimeout) clearTimeout(reconnectTimeout);
      if (ws) ws.close();
    };
  }, [user, activeTab, healthRefreshTick]);

  // Logs WebSocket connection
  useEffect(() => {
    if (!user || activeTab !== "logs") {
      return;
    }

    let ws = null;
    let reconnectTimeout = null;
    let flushInterval = null;
    let isCleaningUp = false;
    const maxLogs = 1000;

    // Clear logs and buffer when filters change
    setLogs([]);
    logBufferRef.current = [];

    // Flush buffered logs to state
    const flushBuffer = () => {
      if (logBufferRef.current.length === 0) return;

      const toFlush = logBufferRef.current;
      logBufferRef.current = [];
      lastFlushRef.current = Date.now();

      setLogs((prev) => {
        const next = [...prev, ...toFlush];
        return next.length > maxLogs ? next.slice(-maxLogs) : next;
      });
    };

    const connect = () => {
      if (isCleaningUp) return;
      setLogsError("");

      const params = new URLSearchParams();
      if (logSourceFilter) params.set("source", logSourceFilter);
      if (logLevelFilter && logLevelFilter !== "DEBUG") params.set("level", logLevelFilter);
      const wsUrl = `${buildWsBase()}/ws/logs${params.toString() ? "?" + params.toString() : ""}`;

      ws = new WebSocket(wsUrl);

      ws.onopen = () => {
        setLogsConnected(true);
        setLogsError("");
      };

      ws.onmessage = (event) => {
        try {
          const entry = JSON.parse(event.data);

          // Update last log time
          setLastLogTime(new Date());

          // Track unique sources
          if (entry.source) {
            setLogSources((prev) => {
              if (prev.includes(entry.source)) return prev;
              return [...prev, entry.source].sort();
            });
          }

          // If unlimited frequency, update immediately
          if (logUpdateFrequency === 0) {
            setLogs((prev) => {
              const next = [...prev, entry];
              return next.length > maxLogs ? next.slice(-maxLogs) : next;
            });
          } else {
            // Buffer the log entry
            logBufferRef.current.push(entry);
          }
        } catch (err) {
          console.error("Failed to parse log entry:", err);
        }
      };

      ws.onerror = () => {
        setLogsError("WebSocket connection error");
        setLogsConnected(false);
      };

      ws.onclose = () => {
        setLogsConnected(false);
        if (!isCleaningUp) {
          reconnectTimeout = setTimeout(connect, 5000);
        }
      };
    };

    connect();

    // Set up flush interval if frequency is limited
    if (logUpdateFrequency > 0) {
      flushInterval = setInterval(flushBuffer, logUpdateFrequency);
    }

    return () => {
      isCleaningUp = true;
      if (reconnectTimeout) clearTimeout(reconnectTimeout);
      if (flushInterval) clearInterval(flushInterval);
      if (ws) ws.close();
      // Flush remaining logs
      flushBuffer();
    };
  }, [user, activeTab, logSourceFilter, logLevelFilter, logUpdateFrequency]);

  // Auto-scroll logs (use ref to avoid race conditions with rapid updates)
  useEffect(() => {
    if (logsAutoScrollRef.current && logsContainerRef.current) {
      const container = logsContainerRef.current;
      container.scrollTop = container.scrollHeight;
    }
  }, [logs]);

  const clearLogs = () => setLogs([]);

  const logLevelColor = (level) => {
    switch (level) {
      case 0: return "debug";
      case 1: return "info";
      case 2: return "warn";
      case 3: return "error";
      default: return "info";
    }
  };

  const logLevelName = (level) => {
    switch (level) {
      case 0: return "DEBUG";
      case 1: return "INFO";
      case 2: return "WARN";
      case 3: return "ERROR";
      default: return "INFO";
    }
  };

  const formatLogTime = (timestamp) => {
    if (!timestamp) return "";
    const date = new Date(timestamp * 1000);
    return date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });
  };

  const handleUpload = async (event) => {
    event.preventDefault();
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
          if (payload.direction !== "upload") {
            return;
          }
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
        `${buildApiBase()}/storage/upload?id=${encodeURIComponent(
          transferId
        )}&namespace=${encodeURIComponent(activeNamespace)}`,
        {
          method: "POST",
          body: formData,
          credentials: "include",
          headers: {
            "X-File-Size": file.size.toString(),
          },
        }
      );
      if (!response.ok) {
        const message = await response.text();
        throw new Error(message || "Upload failed");
      }
      await response.json();
      setStatus(`Uploaded ${file.name}`);
      if (fileInputRef.current) {
        fileInputRef.current.value = "";
      }
      setSelectedFileName("No file selected");
      await loadFiles(activeNamespace);
      await loadNamespaces();
    } catch (err) {
      setStatus(err.message);
    } finally {
      setUploading(false);
      socket.close();
    }
  };

  const handleDownload = async (file) => {
    const transferId = createTransferId();
    let socket;
    const fileKey = `${file.namespace || defaultNamespace}:${file.name}`;
    if (user) {
      socket = new WebSocket(buildWsUrl(transferId));
      setDownloadProgress((prev) => ({
        ...prev,
        [fileKey]: { bytes: 0, total: file.size, active: true },
      }));

      socket.onmessage = (event) => {
        try {
          const payload = JSON.parse(event.data);
          if (payload.direction !== "download") {
            return;
          }
          setDownloadProgress((prev) => ({
            ...prev,
            [fileKey]: {
              bytes: payload.bytes ?? prev[fileKey]?.bytes ?? 0,
              total: payload.total ?? prev[fileKey]?.total ?? file.size,
              active: !payload.done,
            },
          }));
          if (payload.done) {
            socket.close();
          }
        } catch (err) {
          console.warn("Failed to parse download progress", err);
        }
      };

      await waitForSocket(socket, 2000).catch(() => {});
    }
    const link = document.createElement("a");
    link.href = `${buildApiBase()}/storage/download?name=${encodeURIComponent(
      file.name
    )}&id=${encodeURIComponent(transferId)}&namespace=${encodeURIComponent(
      file.namespace || defaultNamespace
    )}`;
    link.rel = "noopener";
    link.style.display = "none";
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    if (socket) {
      socket.close();
    }
  };

  const handleLogin = async (event) => {
    event.preventDefault();
    setLoginError("");
    try {
      const response = await fetch(`${buildApiBase()}/api/login`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(loginForm),
      });
      if (!response.ok) {
        const message = await response.text();
        throw new Error(message || "Login failed");
      }
      const payload = await response.json();
      setUser(payload.username);
      setLoginForm({ username: "", password: "" });
    } catch (err) {
      setLoginError(err.message);
    }
  };

  const handleLogout = async () => {
    try {
      await fetch(`${buildApiBase()}/api/logout`, { method: "POST" });
    } finally {
      setUser(null);
    }
  };

  const handleDelete = async (file) => {
    const fileKey = `${file.namespace || defaultNamespace}:${file.name}`;
    setDeleting((prev) => ({ ...prev, [fileKey]: true }));
    setStatus(`Deleting ${file.name}...`);
    try {
      const response = await fetch(
        `${buildApiBase()}/storage/delete?name=${encodeURIComponent(
          file.name
        )}&namespace=${encodeURIComponent(file.namespace || defaultNamespace)}`,
        {
          method: "DELETE",
        }
      );
      if (!response.ok) {
        const message = await response.text();
        throw new Error(message || "Delete failed");
      }
      setStatus(`Deleted ${file.name}`);
      await loadFiles(activeNamespace);
      await loadNamespaces();
    } catch (err) {
      setStatus(err.message);
    } finally {
      setDeleting((prev) => ({ ...prev, [fileKey]: false }));
    }
  };

  const uploadPercent = progressPercent(uploadProgress.bytes, uploadProgress.total);

  return (
    <div className="page">
      <aside className="sidebar">
        <div className="brand">
          <span className="brand-mark">GFS</span>
          <span className="brand-name">Cloud Share</span>
        </div>
        <nav className="nav">
          {navItems.map((item) => (
            <button
              key={item.id}
              type="button"
              className={`nav-item ${activeTab === item.id ? "active" : ""}`}
              onClick={() => {
                setActiveTab(item.id);
                if (item.id === "storage") {
                  closeNamespace();
                }
              }}
            >
              {item.label}
            </button>
          ))}
        </nav>
        {user ? (
          <button type="button" className="ghost logout" onClick={handleLogout}>
            Sign out
          </button>
        ) : (
          <section className="panel login mini">
            <form onSubmit={handleLogin} className="login-form">
              <div className="field">
                <label htmlFor="login-username">Username</label>
                <input
                  id="login-username"
                  type="text"
                  value={loginForm.username}
                  onChange={(event) =>
                    setLoginForm((prev) => ({ ...prev, username: event.target.value }))
                  }
                  autoComplete="username"
                />
              </div>
              <div className="field">
                <label htmlFor="login-password">Password</label>
                <input
                  id="login-password"
                  type="password"
                  value={loginForm.password}
                  onChange={(event) =>
                    setLoginForm((prev) => ({ ...prev, password: event.target.value }))
                  }
                  autoComplete="current-password"
                />
              </div>
              <button type="submit">Sign in</button>
              {loginError && <p className="status error">{loginError}</p>}
            </form>
          </section>
        )}
      </aside>

      <main className="content">
        <header className="topbar">
          <div>
            <p className="eyebrow">{activeCopy.eyebrow}</p>
            <h1>{activeCopy.title}</h1>
            <p className="lead">{activeCopy.lead}</p>
          </div>
          <div className="summary">
            <div className="summary-card">
              <span className="summary-label">Total files</span>
              <span className="summary-value">{namespaces.reduce((sum, ns) => sum + (ns.count || 0), 0)}</span>
            </div>
          </div>
        </header>

        <div className="layout">
          {activeTab === "storage" && (
            <>
              {!showNamespaceView && (
                <section className="panel namespaces">
                <div className="panel-header">
                  <div>
                    <h2>Namespaces</h2>
                    <p>Choose a namespace to view its files and activity.</p>
                  </div>
                </div>
                {user && (
                  <div className="namespace-controls">
                    <div className="field">
                      <label htmlFor="namespace-new">Create namespace</label>
                      <div className="namespace-create">
                        <input
                          id="namespace-new"
                          type="text"
                          placeholder="eg. team-alpha"
                          value={namespaceInput}
                          onChange={(event) => setNamespaceInput(event.target.value)}
                        />
                        <button
                          type="button"
                          className="ghost"
                          onClick={async () => {
                            const nextNamespace = normalizeNamespace(namespaceInput);
                            if (!nextNamespace) {
                              return;
                            }
                            try {
                              const response = await fetch(`${buildApiBase()}/storage/namespaces`, {
                                method: "POST",
                                headers: { "Content-Type": "application/json" },
                                credentials: "include",
                                body: JSON.stringify({
                                  name: nextNamespace,
                                  hidden: namespaceHidden,
                                }),
                              });
                              if (!response.ok) {
                                if (response.status === 409) {
                                  throw new Error("Namespace already exists.");
                                }
                                const message = await response.text();
                                throw new Error(message || "Failed to create namespace");
                              }
                              await response.json();
                              await loadNamespaces();
                              setNamespaceInput("");
                              setNamespaceHidden(false);
                            } catch (err) {
                              setStatus(err.message);
                            }
                          }}
                        >
                          Add
                        </button>
                        <button
                          type="button"
                          className={`ghost toggle ${namespaceHidden ? "active" : ""}`}
                          onClick={() => setNamespaceHidden((prev) => !prev)}
                        >
                          Hidden
                        </button>
                      </div>
                    </div>
                  </div>
                )}
                <div className="namespace-list">
                  {namespaces.map((item) => (
                    <button
                      key={item.name}
                      type="button"
                      className={`namespace-card ${
                        activeNamespace === item.name ? "active" : ""
                      } ${item.hidden ? "hidden" : ""}`}
                      onClick={() => openNamespace(item.name)}
                    >
                      <div>
                        <h3>{item.name}</h3>
                        <p>{item.count} files</p>
                      </div>
                      <div className="namespace-actions">
                        <span className="namespace-pill">Open</span>
                      </div>
                    </button>
                  ))}
                </div>
                </section>
              )}
              {showNamespaceView && (
                <>
                  <section className="panel namespace-hero">
                    <div>
                      <p className="eyebrow">Namespace</p>
                      <h2>{activeNamespace}</h2>
                    </div>
                    <div className="namespace-count">
                      <span className="namespace-count-label">Files</span>
                      <span className="namespace-count-value">
                        {namespaces.find((item) => item.name === activeNamespace)?.count ?? 0}
                      </span>
                    </div>
                    {user && (
                      <div className="namespace-hero-actions">
                        <button
                          type="button"
                          className="ghost namespace-toggle"
                          disabled={activeNamespace === hiddenNamespace}
                          title={
                            activeNamespace === hiddenNamespace
                              ? "Reserved namespace cannot be unhidden."
                              : undefined
                          }
                          onClick={() => {
                            const current = namespaces.find(
                              (item) => item.name === activeNamespace
                            );
                            if (!current) {
                              return;
                            }
                            fetch(`${buildApiBase()}/storage/namespaces`, {
                              method: "PATCH",
                              credentials: "include",
                              headers: {
                                "Content-Type": "application/json",
                              },
                              body: JSON.stringify({
                                name: current.name,
                                hidden: !current.hidden,
                              }),
                            })
                              .then(async (response) => {
                                if (!response.ok) {
                                  const message = await response.text();
                                  throw new Error(message || "Failed to update namespace");
                                }
                                await loadNamespaces();
                              })
                              .catch((err) => setStatus(err.message));
                          }}
                        >
                          {namespaces.find((item) => item.name === activeNamespace)?.hidden
                            ? "Unhide"
                            : "Hide"}
                        </button>
                        <button
                          type="button"
                          className="ghost namespace-delete"
                          onClick={() => {
                            const current = namespaces.find(
                              (item) => item.name === activeNamespace
                            );
                            if (current) {
                              setNamespaceDeleteTarget(current);
                            }
                          }}
                        >
                          Delete
                        </button>
                      </div>
                    )}
                  </section>
                  {user && (
                    <section className="panel upload">
                      <div className="panel-header">
                        <div>
                          <h2>Upload</h2>
                          <p>
                            Store a file in {activeNamespace}. Existing files with the same name get
                            replaced.
                          </p>
                        </div>
                      </div>
                      <form onSubmit={handleUpload} className="upload-form">
                        <div className="field">
                          <span className="field-label">Select file</span>
                          <div className="file-picker">
                            <input
                              id="upload-file"
                              ref={fileInputRef}
                              type="file"
                              name="file"
                              className="file-input"
                              onChange={(event) => {
                                const file = event.target.files?.[0];
                                setSelectedFileName(file ? file.name : "No file selected");
                              }}
                            />
                            <label htmlFor="upload-file" className="file-button">
                              Browse files
                            </label>
                            <span
                              className={`file-name ${
                                selectedFileName === "No file selected" ? "muted" : ""
                              }`}
                            >
                              {selectedFileName}
                            </span>
                          </div>
                        </div>
                        <button type="submit" disabled={uploading}>
                          {uploading ? "Uploading..." : "Upload"}
                        </button>
                      </form>
                      {uploadProgress.active && (
                        <div className={`progress ${uploadProgress.total ? "" : "indeterminate"}`}>
                          <div className="progress-bar" style={{ width: `${uploadPercent}%` }} />
                        </div>
                      )}
                      {uploadProgress.active && (
                        <p className="progress-text">
                          {uploadProgress.total
                            ? `${uploadPercent}% of ${formatBytes(uploadProgress.total)}`
                            : "Uploading..."}
                        </p>
                      )}
                    </section>
                  )}

                  <section className="panel files">
                    <div className="panel-header">
                      <div>
                        <h2>Files</h2>
                        <p>Download or remove stored objects in {activeNamespace}.</p>
                      </div>
                      <div className="panel-actions">
                        <button
                          type="button"
                          className="ghost"
                          onClick={() => {
                            closeNamespace();
                            loadNamespaces();
                          }}
                        >
                          Back to namespaces
                        </button>
                        <button
                          type="button"
                          className="ghost"
                          onClick={() => loadFiles(activeNamespace)}
                          disabled={loading}
                        >
                          {loading ? "Refreshing..." : "Refresh"}
                        </button>
                      </div>
                    </div>
                    <div className="file-list">
                      {loading && <p className="empty">Loading files...</p>}
                      {!loading && files.length === 0 && <p className="empty">{emptyState}</p>}
                      {!loading &&
                        files.length > 0 && (
                          <div className="file-head">
                            <span>Name</span>
                            <span>Size</span>
                            <span>Modified</span>
                            <span>Actions</span>
                          </div>
                        )}
                      {!loading &&
                        files.map((file) => (
                          <div className="file-row" key={file.path}>
                            <div className="file-col name">
                              <p className="file-name">{file.name}</p>
                              <p className="file-meta">{formatBytes(file.size)}</p>
                            </div>
                            <div className="file-col size">{formatBytes(file.size)}</div>
                            <div className="file-col modified">{formatTimestamp(file.modified_at)}</div>
                            <div className="file-actions file-col actions">
                              <button
                                type="button"
                                className="ghost"
                                disabled={
                                  deleting[`${file.namespace || defaultNamespace}:${file.name}`]
                                }
                                onClick={() => handleDownload(file)}
                              >
                                Download
                              </button>
                              {user && (
                                <button
                                  type="button"
                                  className="danger"
                                  disabled={
                                    deleting[`${file.namespace || defaultNamespace}:${file.name}`]
                                  }
                                  onClick={() => handleDelete(file)}
                                >
                                  {deleting[
                                    `${file.namespace || defaultNamespace}:${file.name}`
                                  ]
                                    ? "Deleting..."
                                    : "Delete"}
                                </button>
                              )}
                            </div>
                          </div>
                        ))}
                    </div>
                  </section>
                </>
              )}
            </>
          )}
          {activeTab === "health" && (
            <section className="panel health">
              <div className="panel-header">
                <div>
                  <h2>Cluster health</h2>
                  <p>Master connectivity and chunkserver status.</p>
                </div>
                <button
                  type="button"
                  className="ghost"
                  onClick={() => setHealthRefreshTick((prev) => prev + 1)}
                >
                  {healthLoading ? "Refreshing..." : "Refresh"}
                </button>
              </div>
              <div className="health-summary">
                <div className={`health-card ${health.cluster_ok ? "ok" : "down"}`}>
                  <span className="health-label">Cluster</span>
                  <span className="health-value">{health.cluster_ok ? "Online" : "Offline"}</span>
                </div>
                <div className="health-card">
                  <span className="health-label">Nodes</span>
                  <span className="health-value">{health.nodes?.length ?? 0}</span>
                </div>
                <div className="health-card">
                  <span className="health-label">Last refresh</span>
                  <span className="health-value small">
                    {lastHealthCheck ? lastHealthCheck.toLocaleTimeString() : "—"}
                  </span>
                </div>
              </div>
              {healthError && <p className="status error">{healthError}</p>}
              <div className="health-table">
                <div className="health-head">
                  <span>Node</span>
                  <span>CPU</span>
                  <span>Memory</span>
                  <span>Disk</span>
                  <span>Pressure</span>
                </div>
                {(health.nodes || []).map((node) => (
                  <div className="health-row" key={node.name}>
                    <div className="health-cell">
                      <strong>{node.name || "unknown"}</strong>
                    </div>
                    <span>{Number.isFinite(node.cpu_percent) ? `${node.cpu_percent.toFixed(1)}%` : "—"}</span>
                    <span>
                      {Number.isFinite(node.memory_percent) ? `${node.memory_percent.toFixed(1)}%` : "—"}
                    </span>
                    <span>
                      {node.disk_allocatable > 0 ? formatBytes(node.disk_allocatable) : "—"}
                    </span>
                    <span>
                      {(node.conditions || []).filter(c => c.status === "True").map(c => c.type.replace("Pressure", "")).join(", ") || "OK"}
                    </span>
                  </div>
                ))}
                {(health.nodes || []).length === 0 && (
                  <p className="empty">No nodes reported.</p>
                )}
              </div>
            </section>
          )}
          {activeTab === "compute" && (
            <>
              {!user ? (
                <section className="panel">
                  <div className="panel-header">
                    <div>
                      <h2>Compute</h2>
                      <p>Sign in to manage compute containers.</p>
                    </div>
                  </div>
                </section>
              ) : (
                <>
                  <div className="compute-nav">
                    <button
                      type="button"
                      className={`compute-nav-item ${computeView === "containers" ? "active" : ""}`}
                      onClick={() => setComputeView("containers")}
                    >
                      Containers
                    </button>
                    <button
                      type="button"
                      className={`compute-nav-item ${computeView === "ssh-keys" ? "active" : ""}`}
                      onClick={() => setComputeView("ssh-keys")}
                    >
                      SSH Keys
                    </button>
                  </div>

                  {computeView === "containers" && (
                    <section className="panel">
                      <div className="panel-header">
                        <div>
                          <h2>Containers</h2>
                          <p>Dev environment containers with dedicated IPs.</p>
                        </div>
                        <div className="panel-actions">
                          <button
                            type="button"
                            className="ghost"
                            onClick={loadContainers}
                            disabled={containersLoading}
                          >
                            {containersLoading ? "Loading..." : "Refresh"}
                          </button>
                          <button
                            type="button"
                            onClick={() => setShowCreateContainer(true)}
                            disabled={sshKeys.length === 0}
                            title={sshKeys.length === 0 ? "Add an SSH key first" : ""}
                          >
                            Create Container
                          </button>
                        </div>
                      </div>
                      {containersError && <p className="status error">{containersError}</p>}
                      {sshKeys.length === 0 && (
                        <p className="status warn">Add an SSH key before creating containers.</p>
                      )}
                      <div className="container-list">
                        {containersLoading && containers.length === 0 && (
                          <p className="empty">Loading containers...</p>
                        )}
                        {!containersLoading && containers.length === 0 && (
                          <p className="empty">No containers yet. Create one to get started.</p>
                        )}
                        {containers.length > 0 && (
                          <div className="container-head">
                            <span>Name</span>
                            <span>Status</span>
                            <span>IP Address</span>
                            <span>Resources</span>
                            <span>Actions</span>
                          </div>
                        )}
                        {containers.map((container) => (
                          <div className="container-row" key={container.id}>
                            <div className="container-col name">
                              <strong>{container.name}</strong>
                              <span className="container-id">{container.id.slice(0, 8)}</span>
                            </div>
                            <div className="container-col status">
                              <span className={`container-status ${container.status}`}>
                                {container.status}
                              </span>
                            </div>
                            <div className="container-col ip">
                              {container.external_ip ? (
                                <code>{container.external_ip}</code>
                              ) : (
                                <span className="muted">Pending...</span>
                              )}
                            </div>
                            <div className="container-col resources">
                              <span>{container.memory_mb} MB</span>
                              <span>{container.storage_gb} GB</span>
                            </div>
                            <div className="container-col actions">
                              {container.status === "running" && (
                                <button
                                  type="button"
                                  className="ghost"
                                  disabled={containerActions[container.id]}
                                  onClick={() => handleContainerAction(container.id, "stopping")}
                                >
                                  {containerActions[container.id] === "stopping"
                                    ? "Stopping..."
                                    : "Stop"}
                                </button>
                              )}
                              {container.status === "stopped" && (
                                <button
                                  type="button"
                                  className="ghost"
                                  disabled={containerActions[container.id]}
                                  onClick={() => handleContainerAction(container.id, "starting")}
                                >
                                  {containerActions[container.id] === "starting"
                                    ? "Starting..."
                                    : "Start"}
                                </button>
                              )}
                              <button
                                type="button"
                                className="danger"
                                disabled={containerActions[container.id]}
                                onClick={() => handleContainerAction(container.id, "deleting")}
                              >
                                {containerActions[container.id] === "deleting"
                                  ? "Deleting..."
                                  : "Delete"}
                              </button>
                            </div>
                          </div>
                        ))}
                      </div>
                      {containers.length > 0 && containers.some((c) => c.external_ip) && (
                        <div className="ssh-hint">
                          <p>
                            Connect via SSH: <code>ssh root@IP_ADDRESS</code>
                          </p>
                        </div>
                      )}
                    </section>
                  )}

                  {computeView === "ssh-keys" && (
                    <section className="panel">
                      <div className="panel-header">
                        <div>
                          <h2>SSH Keys</h2>
                          <p>Public keys for container access.</p>
                        </div>
                        <button
                          type="button"
                          className="ghost"
                          onClick={loadSshKeys}
                          disabled={sshKeysLoading}
                        >
                          {sshKeysLoading ? "Loading..." : "Refresh"}
                        </button>
                      </div>
                      <div className="ssh-key-form">
                        <h3>Add SSH Key</h3>
                        <form onSubmit={handleAddSshKey}>
                          <div className="field">
                            <label htmlFor="ssh-key-name">Name</label>
                            <input
                              id="ssh-key-name"
                              type="text"
                              placeholder="My Laptop"
                              value={newSshKey.name}
                              onChange={(e) =>
                                setNewSshKey((prev) => ({ ...prev, name: e.target.value }))
                              }
                            />
                          </div>
                          <div className="field">
                            <label htmlFor="ssh-key-public">Public Key</label>
                            <textarea
                              id="ssh-key-public"
                              placeholder="ssh-rsa AAAA... or ssh-ed25519 AAAA..."
                              rows={3}
                              value={newSshKey.public_key}
                              onChange={(e) =>
                                setNewSshKey((prev) => ({ ...prev, public_key: e.target.value }))
                              }
                            />
                          </div>
                          <button type="submit" disabled={addingSshKey}>
                            {addingSshKey ? "Adding..." : "Add Key"}
                          </button>
                        </form>
                      </div>
                      <div className="ssh-keys-list">
                        {sshKeysLoading && sshKeys.length === 0 && (
                          <p className="empty">Loading SSH keys...</p>
                        )}
                        {!sshKeysLoading && sshKeys.length === 0 && (
                          <p className="empty">No SSH keys yet. Add one to enable container access.</p>
                        )}
                        {sshKeys.map((key) => (
                          <div className="ssh-key-row" key={key.id}>
                            <div className="ssh-key-info">
                              <strong>{key.name}</strong>
                              <code className="fingerprint">{key.fingerprint}</code>
                            </div>
                            <button
                              type="button"
                              className="danger"
                              onClick={() => handleDeleteSshKey(key.id)}
                            >
                              Delete
                            </button>
                          </div>
                        ))}
                      </div>
                    </section>
                  )}

                </>
              )}
            </>
          )}
          {activeTab === "message-queue" && (
            <section className="panel placeholder">
              <div className="panel-header">
                <div>
                  <h2>Message Queue</h2>
                  <p>Queue status, topics, and delivery insights will appear here.</p>
                </div>
                <span className="badge">Unimplemented</span>
              </div>
              <div className="placeholder-body">
                <div className="placeholder-hero">
                  <svg
                    className="placeholder-image"
                    viewBox="0 0 200 140"
                    role="img"
                    aria-label="Building illustration"
                  >
                    <rect x="20" y="30" width="60" height="90" rx="8" fill="#dbeafe" />
                    <rect x="85" y="15" width="95" height="105" rx="10" fill="#e2e8f0" />
                    <rect x="30" y="45" width="12" height="12" rx="2" fill="#93c5fd" />
                    <rect x="50" y="45" width="12" height="12" rx="2" fill="#93c5fd" />
                    <rect x="30" y="65" width="12" height="12" rx="2" fill="#93c5fd" />
                    <rect x="50" y="65" width="12" height="12" rx="2" fill="#93c5fd" />
                    <rect x="30" y="85" width="12" height="12" rx="2" fill="#93c5fd" />
                    <rect x="50" y="85" width="12" height="12" rx="2" fill="#93c5fd" />
                    <rect x="100" y="30" width="16" height="14" rx="2" fill="#94a3b8" />
                    <rect x="124" y="30" width="16" height="14" rx="2" fill="#94a3b8" />
                    <rect x="148" y="30" width="16" height="14" rx="2" fill="#94a3b8" />
                    <rect x="100" y="52" width="16" height="14" rx="2" fill="#94a3b8" />
                    <rect x="124" y="52" width="16" height="14" rx="2" fill="#94a3b8" />
                    <rect x="148" y="52" width="16" height="14" rx="2" fill="#94a3b8" />
                    <rect x="100" y="74" width="16" height="14" rx="2" fill="#94a3b8" />
                    <rect x="124" y="74" width="16" height="14" rx="2" fill="#94a3b8" />
                    <rect x="148" y="74" width="16" height="14" rx="2" fill="#94a3b8" />
                    <rect x="120" y="92" width="40" height="28" rx="4" fill="#cbd5f5" />
                    <rect x="0" y="120" width="200" height="12" rx="6" fill="#bfdbfe" />
                  </svg>
                </div>
                <div>
                  <h3>Queue services are staged</h3>
                  <p>
                    Messaging endpoints and observability controls will land here. For now, this
                    surface is a placeholder for upcoming work.
                  </p>
                </div>
              </div>
            </section>
          )}
          {activeTab === "datastore" && (
            <section className="panel placeholder">
              <div className="panel-header">
                <div>
                  <h2>Datastore</h2>
                  <p>Managed databases and backups will live in this workspace.</p>
                </div>
                <span className="badge">Unimplemented</span>
              </div>
              <div className="placeholder-body">
                <div className="placeholder-hero">
                  <svg
                    className="placeholder-image"
                    viewBox="0 0 200 140"
                    role="img"
                    aria-label="Building illustration"
                  >
                    <rect x="20" y="30" width="60" height="90" rx="8" fill="#dbeafe" />
                    <rect x="85" y="15" width="95" height="105" rx="10" fill="#e2e8f0" />
                    <rect x="30" y="45" width="12" height="12" rx="2" fill="#93c5fd" />
                    <rect x="50" y="45" width="12" height="12" rx="2" fill="#93c5fd" />
                    <rect x="30" y="65" width="12" height="12" rx="2" fill="#93c5fd" />
                    <rect x="50" y="65" width="12" height="12" rx="2" fill="#93c5fd" />
                    <rect x="30" y="85" width="12" height="12" rx="2" fill="#93c5fd" />
                    <rect x="50" y="85" width="12" height="12" rx="2" fill="#93c5fd" />
                    <rect x="100" y="30" width="16" height="14" rx="2" fill="#94a3b8" />
                    <rect x="124" y="30" width="16" height="14" rx="2" fill="#94a3b8" />
                    <rect x="148" y="30" width="16" height="14" rx="2" fill="#94a3b8" />
                    <rect x="100" y="52" width="16" height="14" rx="2" fill="#94a3b8" />
                    <rect x="124" y="52" width="16" height="14" rx="2" fill="#94a3b8" />
                    <rect x="148" y="52" width="16" height="14" rx="2" fill="#94a3b8" />
                    <rect x="100" y="74" width="16" height="14" rx="2" fill="#94a3b8" />
                    <rect x="124" y="74" width="16" height="14" rx="2" fill="#94a3b8" />
                    <rect x="148" y="74" width="16" height="14" rx="2" fill="#94a3b8" />
                    <rect x="120" y="92" width="40" height="28" rx="4" fill="#cbd5f5" />
                    <rect x="0" y="120" width="200" height="12" rx="6" fill="#bfdbfe" />
                  </svg>
                </div>
                <div>
                  <h3>Datastore controls on deck</h3>
                  <p>
                    Schema management, backups, and performance insights will populate this
                    section after the service is implemented.
                  </p>
                </div>
              </div>
            </section>
          )}
          {activeTab === "logs" && (
            <section className="panel logs">
              <div className="panel-header">
                <div>
                  <h2>Live Logs</h2>
                  <p>Streaming logs from cluster services.</p>
                </div>
                <div className="logs-actions">
                  <span className={`connection-status ${logsConnected ? "connected" : "disconnected"}`}>
                    {logsConnected ? "Connected" : "Disconnected"}
                  </span>
                  <button type="button" className="ghost" onClick={clearLogs}>
                    Clear
                  </button>
                </div>
              </div>
              <div className="logs-filters">
                <label className="filter-group">
                  <span>Source</span>
                  <select
                    value={logSourceFilter}
                    onChange={(e) => setLogSourceFilter(e.target.value)}
                  >
                    <option value="">All Sources</option>
                    {logSources.map((source) => (
                      <option key={source} value={source}>
                        {source}
                      </option>
                    ))}
                  </select>
                </label>
                <label className="filter-group">
                  <span>Min Level</span>
                  <select
                    value={logLevelFilter}
                    onChange={(e) => setLogLevelFilter(e.target.value)}
                  >
                    <option value="DEBUG">DEBUG</option>
                    <option value="INFO">INFO</option>
                    <option value="WARN">WARN</option>
                    <option value="ERROR">ERROR</option>
                  </select>
                </label>
                <label className="filter-group">
                  <span>Update Rate</span>
                  <select
                    value={logUpdateFrequency}
                    onChange={(e) => setLogUpdateFrequency(Number(e.target.value))}
                  >
                    <option value={0}>Unlimited</option>
                    <option value={500}>0.5s</option>
                    <option value={1000}>1s</option>
                    <option value={5000}>5s</option>
                    <option value={10000}>10s</option>
                    <option value={60000}>1min</option>
                  </select>
                </label>
                <label className="filter-group checkbox">
                  <input
                    type="checkbox"
                    checked={logsAutoScroll}
                    onChange={(e) => {
                      logsAutoScrollRef.current = e.target.checked;
                      setLogsAutoScroll(e.target.checked);
                    }}
                  />
                  <span>Auto-scroll</span>
                </label>
              </div>
              {logsError && <p className="status error">{logsError}</p>}
              <div className="logs-container" ref={logsContainerRef}>
                {logs.length === 0 ? (
                  <p className="empty">No log entries yet. Logs will appear as they stream in.</p>
                ) : (
                  <div className="logs-list">
                    {logs.map((entry, idx) => (
                      <div key={idx} className={`log-entry level-${logLevelColor(entry.level)}`}>
                        <span className="log-time">{formatLogTime(entry.timestamp)}</span>
                        <span className={`log-level ${logLevelColor(entry.level)}`}>
                          {logLevelName(entry.level)}
                        </span>
                        <span className="log-source">{entry.source}</span>
                        <span className="log-message">{entry.message}</span>
                      </div>
                    ))}
                    <div ref={logsEndRef} />
                  </div>
                )}
              </div>
              <div className="logs-footer">
                <span>{logs.length} entries</span>
                {lastLogTime && (
                  <span className="last-log-time">
                    Last log: {lastLogTime.toLocaleTimeString()}
                  </span>
                )}
              </div>
            </section>
          )}
        </div>
      </main>
      {namespaceDeleteTarget && (
        <div
          className="modal-overlay"
          onClick={() => setNamespaceDeleteTarget(null)}
          role="presentation"
        >
          <div
            className="modal"
            onClick={(event) => event.stopPropagation()}
            onKeyDown={(event) => {
              if (event.key !== "Tab") {
                return;
              }
              const container = modalRef.current;
              if (!container) {
                return;
              }
              const focusable = container.querySelectorAll(
                'button:not(:disabled), [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'
              );
              if (focusable.length === 0) {
                return;
              }
              const first = focusable[0];
              const last = focusable[focusable.length - 1];
              if (event.shiftKey && document.activeElement === first) {
                event.preventDefault();
                last.focus();
              } else if (!event.shiftKey && document.activeElement === last) {
                event.preventDefault();
                first.focus();
              }
            }}
            role="dialog"
            aria-modal="true"
            aria-labelledby="namespace-delete-title"
            ref={modalRef}
          >
            <h3 id="namespace-delete-title">Delete namespace?</h3>
            <p>
              This will remove "{namespaceDeleteTarget.name}" and all files inside it.
            </p>
            <div className="modal-actions">
              <button
                type="button"
                className="ghost"
                onClick={() => setNamespaceDeleteTarget(null)}
                ref={modalCancelRef}
              >
                Cancel
              </button>
              <button
                type="button"
                className="danger"
                onClick={async () => {
                  const target = namespaceDeleteTarget;
                  if (!target) {
                    return;
                  }
                  try {
                    const response = await fetch(
                      `${buildApiBase()}/storage/namespaces?name=${encodeURIComponent(
                        target.name
                      )}`,
                      {
                        method: "DELETE",
                        credentials: "include",
                      }
                    );
                    if (!response.ok) {
                      const message = await response.text();
                      throw new Error(message || "Failed to delete namespace");
                    }
                    setNamespaceDeleteTarget(null);
                    await loadNamespaces();
                    if (activeNamespace === target.name) {
                      closeNamespace();
                    }
                  } catch (err) {
                    setNamespaceDeleteTarget(null);
                    setStatus(err.message);
                  }
                }}
              >
                Delete
              </button>
            </div>
          </div>
        </div>
      )}
      {showCreateContainer && (
        <div
          className="modal-overlay"
          onClick={() => setShowCreateContainer(false)}
          role="presentation"
        >
          <div
            className="modal modal-lg"
            onClick={(event) => event.stopPropagation()}
            role="dialog"
            aria-modal="true"
            aria-labelledby="create-container-title"
          >
            <h3 id="create-container-title">Create Container</h3>
            <form onSubmit={handleCreateContainer}>
              <div className="field">
                <label htmlFor="container-name">Name</label>
                <input
                  id="container-name"
                  type="text"
                  placeholder="my-dev-env"
                  value={newContainer.name}
                  onChange={(e) =>
                    setNewContainer((prev) => ({ ...prev, name: e.target.value }))
                  }
                  autoFocus
                />
              </div>
              <div className="field-row">
                <div className="field">
                  <label htmlFor="container-memory">Memory</label>
                  <select
                    id="container-memory"
                    value={newContainer.memory_mb}
                    onChange={(e) =>
                      setNewContainer((prev) => ({
                        ...prev,
                        memory_mb: Number(e.target.value),
                      }))
                    }
                  >
                    <option value={256}>256 MB</option>
                    <option value={512}>512 MB</option>
                    <option value={1024}>1 GB</option>
                    <option value={2048}>2 GB</option>
                  </select>
                </div>
                <div className="field">
                  <label htmlFor="container-storage">Storage</label>
                  <select
                    id="container-storage"
                    value={newContainer.storage_gb}
                    onChange={(e) =>
                      setNewContainer((prev) => ({
                        ...prev,
                        storage_gb: Number(e.target.value),
                      }))
                    }
                  >
                    <option value={5}>5 GB</option>
                    <option value={10}>10 GB</option>
                    <option value={20}>20 GB</option>
                  </select>
                </div>
              </div>
              <div className="field">
                <label>SSH Keys</label>
                <p className="field-hint">Select keys to copy into the container</p>
                <div className="ssh-key-select">
                  {sshKeys.length === 0 ? (
                    <p className="empty">No SSH keys available. Add one first.</p>
                  ) : (
                    sshKeys.map((key) => (
                      <label key={key.id} className="ssh-key-option">
                        <input
                          type="checkbox"
                          checked={selectedSshKeyIds.includes(key.id)}
                          onChange={(e) => {
                            if (e.target.checked) {
                              setSelectedSshKeyIds((prev) => [...prev, key.id]);
                            } else {
                              setSelectedSshKeyIds((prev) =>
                                prev.filter((id) => id !== key.id)
                              );
                            }
                          }}
                        />
                        <span className="ssh-key-option-name">{key.name}</span>
                        <code className="ssh-key-option-fp">{key.fingerprint}</code>
                      </label>
                    ))
                  )}
                </div>
              </div>
              <div className="modal-actions">
                <button
                  type="button"
                  className="ghost"
                  onClick={() => {
                    setShowCreateContainer(false);
                    setSelectedSshKeyIds([]);
                  }}
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={creatingContainer || selectedSshKeyIds.length === 0}
                >
                  {creatingContainer ? "Creating..." : "Create Container"}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
      <footer className="build-footer">
        <span>Build: {window.BUILD_INFO?.commit || "dev"}</span>
        <span>{window.BUILD_INFO?.time || ""}</span>
      </footer>
    </div>
  );
}

const root = ReactDOM.createRoot(document.getElementById("root"));
root.render(<App />);
