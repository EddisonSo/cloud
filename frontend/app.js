const { useEffect, useRef, useState } = React;

const emptyState = "No files yet. Upload your first file to share it.";

function formatBytes(bytes) {
  if (!Number.isFinite(bytes) || bytes === 0) {
    return "0 B";
  }
  const units = ["B", "KB", "MB", "GB", "TB"];
  const index = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  const value = bytes / Math.pow(1024, index);
  return `${value.toFixed(value >= 10 || index === 0 ? 0 : 1)} ${units[index]}`;
}

function resolveApiHost() {
  const host = window.location.host;
  if (host.startsWith("cloud.")) {
    return host.replace("cloud.", "cloud-api.");
  }
  if (host.startsWith("cloud-api.")) {
    return host;
  }
  return host;
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

function buildHealthWsUrl() {
  return `${buildWsBase()}/ws/health`;
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
  const [health, setHealth] = useState({ master_ok: false, servers: [] });
  const [healthError, setHealthError] = useState("");
  const [healthLoading, setHealthLoading] = useState(false);
  const [lastHealthCheck, setLastHealthCheck] = useState(null);
  const [healthRefreshTick, setHealthRefreshTick] = useState(0);
  const [uploadProgress, setUploadProgress] = useState({
    bytes: 0,
    total: 0,
    active: false,
  });
  const [selectedFileName, setSelectedFileName] = useState("No file selected");
  const [downloadProgress, setDownloadProgress] = useState({});
  const [deleting, setDeleting] = useState({});
  const fileInputRef = useRef(null);

  const loadFiles = async () => {
    try {
      setLoading(true);
      const response = await fetch(`${buildApiBase()}/api/files`);
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
    loadFiles();
  }, [authChecked, user]);

  useEffect(() => {
    if (!user || activeTab !== "health") {
      return;
    }
    let mounted = true;
    setHealthLoading(true);
    setHealthError("");
    const socket = new WebSocket(buildHealthWsUrl());

    socket.onmessage = (event) => {
      try {
        const payload = JSON.parse(event.data);
        if (!mounted) {
          return;
        }
        setHealth(payload);
        setHealthError(payload.error || "");
        setLastHealthCheck(new Date());
        setHealthLoading(false);
      } catch (err) {
        if (mounted) {
          setHealthError("Failed to parse health update");
          setHealthLoading(false);
        }
      }
    };

    socket.onerror = () => {
      if (mounted) {
        setHealthError("Health stream error");
        setHealthLoading(false);
      }
    };

    return () => {
      mounted = false;
      socket.close();
    };
  }, [user, activeTab, healthRefreshTick]);

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
      const response = await fetch(`${buildApiBase()}/api/upload?id=${encodeURIComponent(transferId)}`, {
        method: "POST",
        body: formData,
        headers: {
          "X-File-Size": file.size.toString(),
        },
      });
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
      await loadFiles();
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
    if (user) {
      socket = new WebSocket(buildWsUrl(transferId));
      setDownloadProgress((prev) => ({
        ...prev,
        [file.name]: { bytes: 0, total: file.size, active: true },
      }));

      socket.onmessage = (event) => {
        try {
          const payload = JSON.parse(event.data);
          if (payload.direction !== "download") {
            return;
          }
          setDownloadProgress((prev) => ({
            ...prev,
            [file.name]: {
              bytes: payload.bytes ?? prev[file.name]?.bytes ?? 0,
              total: payload.total ?? prev[file.name]?.total ?? file.size,
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
    link.href = `${buildApiBase()}/api/download?name=${encodeURIComponent(
      file.name
    )}&id=${encodeURIComponent(transferId)}`;
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
    setDeleting((prev) => ({ ...prev, [file.name]: true }));
    setStatus(`Deleting ${file.name}...`);
    try {
      const response = await fetch(
        `${buildApiBase()}/api/delete?name=${encodeURIComponent(file.name)}`,
        {
          method: "DELETE",
        }
      );
      if (!response.ok) {
        const message = await response.text();
        throw new Error(message || "Delete failed");
      }
      setStatus(`Deleted ${file.name}`);
      await loadFiles();
    } catch (err) {
      setStatus(err.message);
    } finally {
      setDeleting((prev) => ({ ...prev, [file.name]: false }));
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
          <button
            type="button"
            className={`nav-item ${activeTab === "storage" ? "active" : ""}`}
            onClick={() => setActiveTab("storage")}
          >
            Storage
          </button>
          {user && (
            <button
              type="button"
              className={`nav-item ${activeTab === "health" ? "active" : ""}`}
              onClick={() => setActiveTab("health")}
            >
              Health
            </button>
          )}
        </nav>
        <div className="sidebar-foot">
          <span className="sidebar-label">Region</span>
          <span className="sidebar-value">192.168.1.201:30900</span>
        </div>
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
            <p className="eyebrow">Cloud Storage</p>
            <h1>Simple File Share</h1>
            <p className="lead">
              Manage shared assets with clear status, fast uploads, and controlled access.
            </p>
          </div>
          <div className="summary">
            <div className="summary-card">
              <span className="summary-label">Files stored</span>
              <span className="summary-value">{files.length}</span>
            </div>
          </div>
        </header>

        <div className="layout">
          {activeTab === "storage" && (
            <>
          {user && (
            <section className="panel upload">
            <div className="panel-header">
              <div>
                <h2>Upload</h2>
                <p>Store a file in the shared bucket. Existing files with the same name get replaced.</p>
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
                    className={`file-name ${selectedFileName === "No file selected" ? "muted" : ""}`}
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
                <h2>Shared files</h2>
                <p>Download or remove stored objects.</p>
              </div>
              <button type="button" className="ghost" onClick={loadFiles} disabled={loading}>
                {loading ? "Refreshing..." : "Refresh"}
              </button>
            </div>
            <div className="file-list">
              {loading && <p className="empty">Loading files...</p>}
              {!loading && files.length === 0 && <p className="empty">{emptyState}</p>}
              {!loading &&
                files.length > 0 && (
                  <div className="file-head">
                    <span>Name</span>
                    <span>Size</span>
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
                    <div className="file-actions file-col actions">
                      <button
                        type="button"
                        className="ghost"
                        disabled={deleting[file.name]}
                        onClick={() => handleDownload(file)}
                      >
                        Download
                      </button>
                      {user && (
                        <button
                          type="button"
                          className="danger"
                          disabled={deleting[file.name]}
                          onClick={() => handleDelete(file)}
                        >
                          {deleting[file.name] ? "Deleting..." : "Delete"}
                        </button>
                      )}
                    </div>
                  </div>
                ))}
            </div>
          </section>
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
                <div className={`health-card ${health.master_ok ? "ok" : "down"}`}>
                  <span className="health-label">Master</span>
                  <span className="health-value">{health.master_ok ? "Online" : "Offline"}</span>
                </div>
                <div className="health-card">
                  <span className="health-label">Chunkservers</span>
                  <span className="health-value">{health.servers?.length ?? 0}</span>
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
                  <span>Server</span>
                  <span>Status</span>
                  <span>CPU</span>
                  <span>Memory</span>
                  <span>Disk</span>
                  <span>Chunks</span>
                </div>
                {(health.servers || []).map((server) => (
                  <div className="health-row" key={`${server.id}-${server.host}`}>
                    <div className="health-cell">
                      <strong>{server.host || "unknown"}</strong>
                      <span className="health-meta">
                        {server.data_port ? `:${server.data_port}` : ""}
                      </span>
                    </div>
                    <span className={`pill ${server.alive ? "ok" : "down"}`}>
                      {server.alive ? "Alive" : "Down"}
                    </span>
                    <span>{Number.isFinite(server.cpu_usage) ? `${server.cpu_usage.toFixed(1)}%` : "—"}</span>
                    <span>
                      {Number.isFinite(server.memory_usage) ? `${server.memory_usage.toFixed(1)}%` : "—"}
                    </span>
                    <span>
                      {Number.isFinite(server.disk_usage) ? `${server.disk_usage.toFixed(1)}%` : "—"}
                    </span>
                    <span>{server.chunk_count ?? 0}</span>
                  </div>
                ))}
                {(health.servers || []).length === 0 && (
                  <p className="empty">No chunkservers reported.</p>
                )}
              </div>
            </section>
          )}
        </div>
      </main>
    </div>
  );
}

const root = ReactDOM.createRoot(document.getElementById("root"));
root.render(<App />);
