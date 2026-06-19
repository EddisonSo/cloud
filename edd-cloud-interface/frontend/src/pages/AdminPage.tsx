import { useState, useEffect } from "react";
import { Breadcrumb } from "@/components/ui/breadcrumb";
import { PageHeader } from "@/components/ui/page-header";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { StatusChip } from "@/components/ui/status-chip";
import { CopyableText, Modal } from "@/components/common";
import { buildAuthBase, buildComputeBase, buildStorageBase, getAuthHeaders } from "@/lib/api";
import { useAuth } from "@/contexts/AuthContext";
import { Trash2, UserPlus, EyeOff, Link } from "lucide-react";
import type { AdminUser, AdminSession, AdminNamespace, Container } from "@/types";

export function AdminPage() {
  const { user, isAdmin } = useAuth();
  const [containers, setContainers] = useState<Container[]>([]);
  const [users, setUsers] = useState<AdminUser[]>([]);
  const [namespaces, setNamespaces] = useState<AdminNamespace[]>([]);
  const [sessions, setSessions] = useState<AdminSession[]>([]);
  const [loading, setLoading] = useState(false);
  const [newUser, setNewUser] = useState<{ displayName: string; username: string; password: string }>({ displayName: "", username: "", password: "" });
  const [showCreateUserModal, setShowCreateUserModal] = useState(false);
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState("");

  const parseJsonSafe = async (response: Response): Promise<unknown> => {
    const text = await response.text();
    if (!text) return null;
    try {
      return JSON.parse(text);
    } catch {
      return null;
    }
  };

  const loadData = async () => {
    if (!isAdmin) return;
    setLoading(true);
    setError("");
    try {
      const [containersRes, usersRes, namespacesRes, sessionsRes] = await Promise.all([
        fetch(`${buildComputeBase()}/compute/admin/containers`, { headers: getAuthHeaders() }),
        fetch(`${buildAuthBase()}/admin/users`, { headers: getAuthHeaders() }),
        fetch(`${buildStorageBase()}/admin/namespaces`, { headers: getAuthHeaders() }),
        fetch(`${buildAuthBase()}/admin/sessions`, { headers: getAuthHeaders() }),
      ]);
      if (containersRes.ok) {
        const data = await parseJsonSafe(containersRes) as Container[] | null;
        setContainers(data || []);
      }
      if (usersRes.ok) {
        const data = await parseJsonSafe(usersRes) as AdminUser[] | null;
        setUsers(data || []);
      } else {
        const errText = await usersRes.text();
        setError(`Failed to load users: ${errText}`);
      }
      if (namespacesRes.ok) {
        const data = await parseJsonSafe(namespacesRes) as AdminNamespace[] | null;
        setNamespaces(data || []);
      }
      if (sessionsRes.ok) {
        const data = await parseJsonSafe(sessionsRes) as AdminSession[] | null;
        setSessions(data || []);
      }
    } catch (err) {
      setError(`Failed to load admin data: ${(err as Error).message}`);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadData();
  }, [isAdmin]);

  const handleCreateUser = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (!newUser.username.trim() || !newUser.password) return;
    setCreating(true);
    setError("");
    try {
      const response = await fetch(`${buildAuthBase()}/admin/users`, {
        method: "POST",
        headers: { "Content-Type": "application/json", ...getAuthHeaders() },
        body: JSON.stringify({
          display_name: newUser.displayName.trim(),
          username: newUser.username.trim(),
          password: newUser.password,
        }),
      });
      if (!response.ok) {
        const msg = await response.text();
        throw new Error(msg || "Failed to create user");
      }
      setNewUser({ displayName: "", username: "", password: "" });
      setShowCreateUserModal(false);
      await loadData();
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setCreating(false);
    }
  };

  const handleDeleteUser = async (userId: string) => {
    if (!confirm("Delete this user?")) return;
    try {
      const response = await fetch(`${buildAuthBase()}/admin/users?id=${userId}`, {
        method: "DELETE",
        headers: getAuthHeaders(),
      });
      if (!response.ok) {
        const msg = await response.text();
        throw new Error(msg || "Failed to delete user");
      }
      await loadData();
    } catch (err) {
      setError((err as Error).message);
    }
  };

  if (!isAdmin) {
    return (
      <div>
        <Breadcrumb items={[{ label: "Admin" }]} />
        <PageHeader title="Admin Panel" description="Access denied. Admin privileges required." />
      </div>
    );
  }

  return (
    <div>
      <Breadcrumb items={[{ label: "Admin" }]} />
      <PageHeader
        title="Admin Panel"
        description="View all files and containers across the system."
        actions={
          <Button onClick={() => setShowCreateUserModal(true)}>
            <UserPlus className="w-4 h-4 mr-2" />
            Add User
          </Button>
        }
      />

      {/* Users Section */}
      <div className="bg-card border border-border mb-6">
        <div className="px-5 py-4 border-b border-border">
          <h2 className="font-mono text-[10.5px] font-medium uppercase tracking-[0.2em] text-faint">Users</h2>
        </div>
        {error && <p className="text-destructive text-sm px-5 pt-4">{error}</p>}

        {loading ? (
          <p className="text-muted-foreground py-4 px-5">Loading users...</p>
        ) : users.length === 0 ? (
          <p className="text-muted-foreground py-4 px-5">No users found</p>
        ) : (
          <div>
            {/* Header - hidden on mobile */}
            <div className="hidden sm:grid sm:grid-cols-[2fr_2fr_1fr_80px] gap-4 px-5 py-3 border-b border-border">
              <div className="font-mono text-[10px] uppercase tracking-[0.2em] text-faint">Username</div>
              <div className="font-mono text-[10px] uppercase tracking-[0.2em] text-faint">Display Name</div>
              <div className="font-mono text-[10px] uppercase tracking-[0.2em] text-faint">ID</div>
              <div className="font-mono text-[10px] uppercase tracking-[0.2em] text-faint text-center">Actions</div>
            </div>
            {users.map((u) => (
              <div
                key={u.user_id}
                className="flex flex-col sm:grid sm:grid-cols-[2fr_2fr_1fr_80px] gap-2 sm:gap-4 px-5 py-3 border-b border-border last:border-0 hover:bg-popover transition-colors sm:items-center"
              >
                <div className="flex justify-between sm:block">
                  <span className="font-mono text-[10px] uppercase tracking-[0.2em] text-faint sm:hidden">Username:</span>
                  <span className="font-mono text-[12.5px] text-muted-foreground truncate">{u.username}</span>
                </div>
                <div className="flex justify-between sm:block">
                  <span className="font-mono text-[10px] uppercase tracking-[0.2em] text-faint sm:hidden">Name:</span>
                  <span className="text-sm font-medium truncate">{u.display_name || u.username}</span>
                </div>
                <div className="flex justify-between sm:block">
                  <span className="font-mono text-[10px] uppercase tracking-[0.2em] text-faint sm:hidden">ID:</span>
                  <CopyableText text={u.user_id} mono />
                </div>
                <div className="flex justify-center">
                  <Button
                    variant="ghost"
                    size="sm"
                    className="text-destructive hover:text-destructive"
                    onClick={() => handleDeleteUser(u.user_id)}
                  >
                    <Trash2 className="w-4 h-4" />
                  </Button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Active Sessions Section */}
      <div className="bg-card border border-border mb-6">
        <div className="px-5 py-4 border-b border-border">
          <h2 className="font-mono text-[10.5px] font-medium uppercase tracking-[0.2em] text-faint">Active Sessions</h2>
        </div>
        {loading ? (
          <p className="text-muted-foreground py-4 px-5">Loading sessions...</p>
        ) : sessions.length === 0 ? (
          <p className="text-muted-foreground py-4 px-5">No active sessions</p>
        ) : (
          <div>
            {/* Header - hidden on mobile */}
            <div className="hidden sm:grid sm:grid-cols-[2fr_2fr_2fr] gap-4 px-5 py-3 border-b border-border">
              <div className="font-mono text-[10px] uppercase tracking-[0.2em] text-faint">User</div>
              <div className="font-mono text-[10px] uppercase tracking-[0.2em] text-faint">IP</div>
              <div className="font-mono text-[10px] uppercase tracking-[0.2em] text-faint">JWT Issued</div>
            </div>
            {sessions.map((s, idx) => {
              const formatTime = (unix: number | undefined) => {
                if (!unix) return "—";
                const date = new Date(unix * 1000);
                return date.toLocaleString();
              };

              return (
                <div
                  key={`${s.user_id}-${s.created_at}-${idx}`}
                  className="flex flex-col sm:grid sm:grid-cols-[2fr_2fr_2fr] gap-2 sm:gap-4 px-5 py-3 border-b border-border last:border-0 hover:bg-popover transition-colors sm:items-center"
                >
                  <div className="flex justify-between sm:block">
                    <span className="font-mono text-[10px] uppercase tracking-[0.2em] text-faint sm:hidden">User:</span>
                    <span className="text-sm font-medium truncate">{s.display_name || s.username}</span>
                  </div>
                  <div className="flex justify-between sm:block">
                    <span className="font-mono text-[10px] uppercase tracking-[0.2em] text-faint sm:hidden">IP:</span>
                    <span className="font-mono text-[12.5px] text-muted-foreground">{s.ip_address || "—"}</span>
                  </div>
                  <div className="flex justify-between sm:block">
                    <span className="font-mono text-[10px] uppercase tracking-[0.2em] text-faint sm:hidden">JWT Issued:</span>
                    <span className="font-mono text-[12.5px] text-muted-foreground">{formatTime(s.created_at)}</span>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>

      {/* Containers Section */}
      <div className="bg-card border border-border mb-6">
        <div className="px-5 py-4 border-b border-border">
          <h2 className="font-mono text-[10.5px] font-medium uppercase tracking-[0.2em] text-faint">All Containers</h2>
        </div>
        {loading ? (
          <p className="text-muted-foreground py-4 px-5">Loading...</p>
        ) : containers.length === 0 ? (
          <p className="text-muted-foreground py-4 px-5">No containers</p>
        ) : (
          <div>
            {/* Header - hidden on mobile */}
            <div className="hidden lg:grid lg:grid-cols-[1fr_2fr_2fr_1.5fr_1fr] gap-4 px-5 py-3 border-b border-border">
              <div className="font-mono text-[10px] uppercase tracking-[0.2em] text-faint">ID</div>
              <div className="font-mono text-[10px] uppercase tracking-[0.2em] text-faint">Name</div>
              <div className="font-mono text-[10px] uppercase tracking-[0.2em] text-faint">Hostname</div>
              <div className="font-mono text-[10px] uppercase tracking-[0.2em] text-faint">Owner</div>
              <div className="font-mono text-[10px] uppercase tracking-[0.2em] text-faint">Status</div>
            </div>
            {containers.map((c) => (
              <div
                key={c.id}
                className="flex flex-col lg:grid lg:grid-cols-[1fr_2fr_2fr_1.5fr_1fr] gap-2 lg:gap-4 px-5 py-3 border-b border-border last:border-0 hover:bg-popover transition-colors lg:items-center"
              >
                <div className="flex justify-between lg:block">
                  <span className="font-mono text-[10px] uppercase tracking-[0.2em] text-faint lg:hidden">ID:</span>
                  <CopyableText text={c.id.slice(0, 8)} mono />
                </div>
                <div className="flex justify-between lg:block min-w-0">
                  <span className="font-mono text-[10px] uppercase tracking-[0.2em] text-faint lg:hidden">Name:</span>
                  <span className="text-sm font-medium truncate">{c.name}</span>
                </div>
                <div className="flex justify-between lg:block min-w-0">
                  <span className="font-mono text-[10px] uppercase tracking-[0.2em] text-faint lg:hidden">Hostname:</span>
                  <span className="font-mono text-[12.5px] text-muted-foreground truncate">
                    {c.hostname || "—"}
                  </span>
                </div>
                <div className="flex justify-between lg:block min-w-0">
                  <span className="font-mono text-[10px] uppercase tracking-[0.2em] text-faint lg:hidden">Owner:</span>
                  <span className="font-mono text-[12.5px] text-muted-foreground truncate">{c.owner || "—"}</span>
                </div>
                <div className="flex justify-between lg:block items-center">
                  <span className="font-mono text-[10px] uppercase tracking-[0.2em] text-faint lg:hidden">Status:</span>
                  <StatusChip status={c.status} />
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Namespaces Section */}
      <div className="bg-card border border-border">
        <div className="px-5 py-4 border-b border-border">
          <h2 className="font-mono text-[10.5px] font-medium uppercase tracking-[0.2em] text-faint">All Namespaces</h2>
        </div>
        {loading ? (
          <p className="text-muted-foreground py-4 px-5">Loading...</p>
        ) : namespaces.length === 0 ? (
          <p className="text-muted-foreground py-4 px-5">No namespaces</p>
        ) : (
          <div>
            {/* Header - hidden on mobile */}
            <div className="hidden sm:grid sm:grid-cols-[2fr_1fr_1fr_1fr] gap-4 px-5 py-3 border-b border-border">
              <div className="font-mono text-[10px] uppercase tracking-[0.2em] text-faint">Name</div>
              <div className="font-mono text-[10px] uppercase tracking-[0.2em] text-faint">Files</div>
              <div className="font-mono text-[10px] uppercase tracking-[0.2em] text-faint">Visibility</div>
              <div className="font-mono text-[10px] uppercase tracking-[0.2em] text-faint">Owner ID</div>
            </div>
            {namespaces.map((ns) => (
              <div
                key={ns.name}
                className="flex flex-col sm:grid sm:grid-cols-[2fr_1fr_1fr_1fr] gap-2 sm:gap-4 px-5 py-3 border-b border-border last:border-0 hover:bg-popover transition-colors sm:items-center"
              >
                <div className="flex justify-between sm:block min-w-0">
                  <span className="font-mono text-[10px] uppercase tracking-[0.2em] text-faint sm:hidden">Name:</span>
                  <span className="text-sm font-medium truncate">{ns.name}</span>
                </div>
                <div className="flex justify-between sm:block">
                  <span className="font-mono text-[10px] uppercase tracking-[0.2em] text-faint sm:hidden">Files:</span>
                  <span className="font-mono text-[12.5px] text-muted-foreground">{ns.count}</span>
                </div>
                <div className="flex justify-between sm:block items-center">
                  <span className="font-mono text-[10px] uppercase tracking-[0.2em] text-faint sm:hidden">Visibility:</span>
                  <span className="flex items-center gap-1 text-sm">
                    {ns.visibility === 0 ? (
                      <>
                        <EyeOff className="w-3.5 h-3.5 text-muted-foreground" />
                        <span className="text-muted-foreground">Private</span>
                      </>
                    ) : (
                      <>
                        <Link className="w-3.5 h-3.5 text-success" />
                        <span className="text-success">Public</span>
                      </>
                    )}
                  </span>
                </div>
                <div className="flex justify-between sm:block min-w-0">
                  <span className="font-mono text-[10px] uppercase tracking-[0.2em] text-faint sm:hidden">Owner:</span>
                  <span className="font-mono text-[12.5px] text-muted-foreground">
                    {ns.owner_id != null ? ns.owner_id : "System"}
                  </span>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Create User Modal */}
      <Modal
        open={showCreateUserModal}
        onClose={() => {
          setShowCreateUserModal(false);
          setNewUser({ displayName: "", username: "", password: "" });
          setError("");
        }}
        title="Add User"
        description="Create a new user account."
      >
        <form onSubmit={handleCreateUser} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="displayName">Display Name</Label>
            <Input
              id="displayName"
              placeholder="e.g. John Doe"
              value={newUser.displayName}
              onChange={(e) => setNewUser((p) => ({ ...p, displayName: e.target.value }))}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="username">Username</Label>
            <Input
              id="username"
              placeholder="e.g. johndoe"
              value={newUser.username}
              onChange={(e) => setNewUser((p) => ({ ...p, username: e.target.value }))}
              autoFocus
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="password">Password</Label>
            <Input
              id="password"
              type="password"
              placeholder="Password"
              value={newUser.password}
              onChange={(e) => setNewUser((p) => ({ ...p, password: e.target.value }))}
            />
          </div>
          {error && <p className="text-sm text-destructive">{error}</p>}
          <div className="flex justify-end gap-2 pt-2">
            <Button type="button" variant="outline" onClick={() => setShowCreateUserModal(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={creating || !newUser.username.trim() || !newUser.password}>
              {creating ? "Creating..." : "Create User"}
            </Button>
          </div>
        </form>
      </Modal>
    </div>
  );
}
