export function resolveApiHost() {
  // Use cloud-api subdomain for API calls
  const host = window.location.host;
  if (host.startsWith("cloud.")) {
    return host.replace("cloud.", "cloud-api.");
  }
  return host;
}

export function buildApiBase() {
  return `${window.location.protocol}//${resolveApiHost()}`;
}

export function buildWsBase() {
  const protocol = window.location.protocol === "https:" ? "wss" : "ws";
  return `${protocol}://${resolveApiHost()}`;
}

export function buildWsUrl(id) {
  return `${buildWsBase()}/ws?id=${encodeURIComponent(id)}`;
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
