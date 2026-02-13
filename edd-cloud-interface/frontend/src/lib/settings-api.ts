import { buildAuthBase, getAuthHeaders } from "@/lib/api";
import type { SecurityKey, UserSession } from "@/types";

const base = () => buildAuthBase();
const headers = () => ({ ...getAuthHeaders(), "Content-Type": "application/json" });

// --- Security Keys ---

export async function fetchSecurityKeys(): Promise<SecurityKey[]> {
  const res = await fetch(`${base()}/api/settings/keys`, { headers: getAuthHeaders() });
  if (!res.ok) throw new Error("Failed to fetch keys");
  const data = await res.json();
  return data.keys ?? [];
}

export async function beginAddKey(): Promise<{ options: Record<string, unknown>; state: string }> {
  const res = await fetch(`${base()}/api/settings/keys/add/begin`, {
    method: "POST",
    headers: getAuthHeaders(),
  });
  if (!res.ok) throw new Error("Failed to begin key registration");
  return res.json();
}

export async function finishAddKey(state: string, credential: Record<string, unknown>): Promise<void> {
  const res = await fetch(`${base()}/api/settings/keys/add/finish`, {
    method: "POST",
    headers: headers(),
    body: JSON.stringify({ state, credential }),
  });
  if (!res.ok) throw new Error("Failed to complete key registration");
}

export async function deleteKey(id: string): Promise<void> {
  const res = await fetch(`${base()}/api/settings/keys/delete`, {
    method: "POST",
    headers: headers(),
    body: JSON.stringify({ id }),
  });
  if (!res.ok) {
    const data = await res.json().catch(() => ({}));
    throw new Error(data.error || "Failed to delete key");
  }
}

export async function renameKey(id: string, name: string): Promise<void> {
  const res = await fetch(`${base()}/api/settings/keys/rename`, {
    method: "POST",
    headers: headers(),
    body: JSON.stringify({ id, name }),
  });
  if (!res.ok) throw new Error("Failed to rename key");
}

// --- Profile ---

export async function updateProfile(displayName: string): Promise<void> {
  const res = await fetch(`${base()}/api/settings/profile`, {
    method: "PUT",
    headers: headers(),
    body: JSON.stringify({ display_name: displayName }),
  });
  if (!res.ok) throw new Error("Failed to update profile");
}

export async function changePassword(currentPassword: string, newPassword: string): Promise<void> {
  const res = await fetch(`${base()}/api/settings/password`, {
    method: "PUT",
    headers: headers(),
    body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }),
  });
  if (!res.ok) {
    const data = await res.json().catch(() => ({}));
    throw new Error(data.error || "Failed to change password");
  }
}

// --- Sessions ---

export async function fetchSessions(): Promise<UserSession[]> {
  const res = await fetch(`${base()}/api/settings/sessions`, { headers: getAuthHeaders() });
  if (!res.ok) throw new Error("Failed to fetch sessions");
  return res.json();
}

export async function revokeSession(sessionId: number): Promise<void> {
  const res = await fetch(`${base()}/api/settings/sessions/${sessionId}`, {
    method: "DELETE",
    headers: getAuthHeaders(),
  });
  if (!res.ok) {
    const data = await res.json().catch(() => ({}));
    throw new Error(data.error || "Failed to revoke session");
  }
}
