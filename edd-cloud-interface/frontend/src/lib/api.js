const AUTH_TOKEN_KEY = "auth_token";

export function getAuthToken() {
  return localStorage.getItem(AUTH_TOKEN_KEY);
}

export function setAuthToken(token) {
  localStorage.setItem(AUTH_TOKEN_KEY, token);
}

export function clearAuthToken() {
  localStorage.removeItem(AUTH_TOKEN_KEY);
}

export function getAuthHeaders() {
  const token = getAuthToken();
  return token ? { Authorization: `Bearer ${token}` } : {};
}

export function resolveApiHost() {
  // Use cloud-api subdomain for API calls (legacy, used for auth)
  const host = window.location.host;
  if (host.startsWith("cloud.")) {
    return host.replace("cloud.", "cloud-api.");
  }
  return host;
}

// Service-specific hosts for better connection pooling
export function resolveServiceHost(service) {
  const host = window.location.host;
  if (host.startsWith("cloud.")) {
    // Use service.cloud.domain format (e.g., "storage.cloud.eddisonso.com")
    return `${service}.${host}`;
  }
  return host;
}

export function buildApiBase() {
  return `${window.location.protocol}//${resolveApiHost()}`;
}

export function buildAuthBase() {
  return buildServiceBase("auth");
}

export function buildServiceBase(service) {
  return `${window.location.protocol}//${resolveServiceHost(service)}`;
}

export function buildComputeBase() {
  return buildServiceBase("compute");
}

export function buildStorageBase() {
  return buildServiceBase("storage");
}

export function buildHealthBase() {
  return buildServiceBase("health");
}

export function buildWsBase() {
  const protocol = window.location.protocol === "https:" ? "wss" : "ws";
  return `${protocol}://${resolveApiHost()}`;
}

export function buildComputeWsBase() {
  const protocol = window.location.protocol === "https:" ? "wss" : "ws";
  return `${protocol}://${resolveServiceHost("compute")}`;
}

export function buildWsUrl(id) {
  return `${buildWsBase()}/ws?id=${encodeURIComponent(id)}`;
}

export function buildSseUrl(id) {
  return `${buildStorageBase()}/sse/progress?id=${encodeURIComponent(id)}`;
}

export function buildClusterInfoUrl() {
  return `${buildApiBase()}/cluster-info`;
}

export function buildClusterInfoWsUrl() {
  return `${buildWsBase()}/ws/cluster-info`;
}

export function createTransferId() {
  if (window.crypto?.randomUUID) {
    return window.crypto.randomUUID();
  }
  return `transfer-${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

export function waitForSocket(socket, timeoutMs = 1000) {
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

export function copyToClipboard(text, showToast = true) {
  navigator.clipboard.writeText(text).then(() => {
    if (showToast) {
      const toast = document.createElement('div');
      toast.className = 'fixed bottom-6 left-1/2 -translate-x-1/2 bg-secondary text-foreground px-4 py-2 rounded-md text-sm z-50 border border-border shadow-lg animate-in fade-in slide-in-from-bottom-2';
      toast.textContent = 'Copied!';
      document.body.appendChild(toast);
      setTimeout(() => toast.remove(), 1500);
    }
  });
}

export async function fetchMetricsHistory({ start, end, resolution, nodeName } = {}) {
  const params = new URLSearchParams();
  if (start) params.append("start", start.toISOString());
  if (end) params.append("end", end.toISOString());
  if (resolution) params.append("resolution", resolution);

  const endpoint = nodeName
    ? `${buildHealthBase()}/api/metrics/nodes/${encodeURIComponent(nodeName)}`
    : `${buildHealthBase()}/api/metrics/nodes`;

  const response = await fetch(`${endpoint}?${params}`, {
    headers: getAuthHeaders(),
  });
  if (!response.ok) {
    throw new Error(`Failed to fetch metrics: ${response.status}`);
  }
  return response.json();
}

export async function fetchServiceDependencies() {
  const response = await fetch(`${buildHealthBase()}/api/graph/dependencies`, {
    headers: getAuthHeaders(),
  });
  if (!response.ok) {
    throw new Error(`Failed to fetch dependencies: ${response.status}`);
  }
  return response.json();
}
