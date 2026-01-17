import { useState, useEffect } from "react";
import { Header } from "@/components/layout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { StatusBadge, CopyableText } from "@/components/common";
import { TAB_COPY } from "@/lib/constants";
import { buildApiBase } from "@/lib/api";
import { useAuth } from "@/contexts/AuthContext";
import { Trash2, UserPlus } from "lucide-react";

export function AdminPage() {
  const copy = TAB_COPY.admin;
  const { user, isAdmin } = useAuth();
  const [containers, setContainers] = useState([]);
  const [users, setUsers] = useState([]);
  const [loading, setLoading] = useState(false);
  const [newUser, setNewUser] = useState({ displayName: "", username: "", password: "" });
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState("");

  const loadData = async () => {
    if (!isAdmin) return;
    setLoading(true);
    setError("");
    try {
      const [containersRes, usersRes] = await Promise.all([
        fetch(`${buildApiBase()}/compute/admin/containers`, { credentials: "include" }),
        fetch(`${buildApiBase()}/admin/users`, { credentials: "include" }),
      ]);
      if (containersRes.ok) {
        const data = await containersRes.json();
        setContainers(data || []);
      }
      if (usersRes.ok) {
        const data = await usersRes.json();
        setUsers(data || []);
      } else {
        const errText = await usersRes.text();
        setError(`Failed to load users: ${errText}`);
      }
    } catch (err) {
      setError(`Failed to load admin data: ${err.message}`);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadData();
  }, [isAdmin]);

  const handleCreateUser = async (e) => {
    e.preventDefault();
    if (!newUser.username.trim() || !newUser.password) return;
    setCreating(true);
    setError("");
    try {
      const response = await fetch(`${buildApiBase()}/admin/users`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
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
      await loadData();
    } catch (err) {
      setError(err.message);
    } finally {
      setCreating(false);
    }
  };

  const handleDeleteUser = async (userId) => {
    if (!confirm("Delete this user?")) return;
    try {
      const response = await fetch(`${buildApiBase()}/admin/users?id=${userId}`, {
        method: "DELETE",
        credentials: "include",
      });
      if (!response.ok) {
        const msg = await response.text();
        throw new Error(msg || "Failed to delete user");
      }
      await loadData();
    } catch (err) {
      setError(err.message);
    }
  };

  if (!isAdmin) {
    return (
      <div>
        <Header eyebrow={copy.eyebrow} title={copy.title} description={copy.lead} />
        <p className="text-muted-foreground">Access denied. Admin privileges required.</p>
      </div>
    );
  }

  return (
    <div>
      <Header eyebrow={copy.eyebrow} title={copy.title} description={copy.lead} />

      {/* Summary Cards */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 mb-6">
        <Card className="min-w-0">
          <CardContent className="pt-6">
            <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground mb-1 truncate">
              Total Users
            </p>
            <span className="text-2xl font-semibold">{users.length}</span>
          </CardContent>
        </Card>
        <Card className="min-w-0">
          <CardContent className="pt-6">
            <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground mb-1 truncate">
              Total Containers
            </p>
            <span className="text-2xl font-semibold">{containers.length}</span>
          </CardContent>
        </Card>
        <Card className="min-w-0">
          <CardContent className="pt-6">
            <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground mb-1 truncate">
              Running
            </p>
            <span className="text-2xl font-semibold text-green-400">
              {containers.filter((c) => c.status === "running").length}
            </span>
          </CardContent>
        </Card>
      </div>

      {/* Users Section */}
      <Card className="mb-6">
        <CardHeader>
          <CardTitle>Users</CardTitle>
        </CardHeader>
        <CardContent>
          {/* Create User Form */}
          <form onSubmit={handleCreateUser} className="flex gap-3 mb-4">
            <Input
              placeholder="Display Name"
              value={newUser.displayName}
              onChange={(e) => setNewUser((p) => ({ ...p, displayName: e.target.value }))}
              className="flex-1"
            />
            <Input
              placeholder="Username (login)"
              value={newUser.username}
              onChange={(e) => setNewUser((p) => ({ ...p, username: e.target.value }))}
              className="flex-1"
            />
            <Input
              type="password"
              placeholder="Password"
              value={newUser.password}
              onChange={(e) => setNewUser((p) => ({ ...p, password: e.target.value }))}
              className="flex-1"
            />
            <Button type="submit" disabled={creating}>
              <UserPlus className="w-4 h-4 mr-1" />
              Add User
            </Button>
          </form>
          {error && <p className="text-destructive text-sm mb-4">{error}</p>}

          {/* Users List */}
          {loading ? (
            <p className="text-muted-foreground py-4">Loading users...</p>
          ) : users.length === 0 ? (
            <p className="text-muted-foreground py-4">No users found</p>
          ) : (
            <div className="space-y-2">
              <div className="grid grid-cols-4 gap-4 px-4 py-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                <div>ID</div>
                <div>Display Name</div>
                <div>Username</div>
                <div>Actions</div>
              </div>
              {users.map((u) => (
                <div
                  key={u.id}
                  className="grid grid-cols-4 gap-4 px-4 py-3 bg-secondary rounded-md items-center"
                >
                  <CopyableText text={String(u.id)} mono />
                  <span className="font-medium">{u.display_name || u.username}</span>
                  <span className="text-muted-foreground">{u.username}</span>
                  <div>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="text-destructive hover:text-destructive hover:bg-destructive/10"
                      onClick={() => handleDeleteUser(u.id)}
                    >
                      <Trash2 className="w-4 h-4" />
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      {/* Containers Section */}
      <Card>
        <CardHeader>
          <CardTitle>All Containers</CardTitle>
        </CardHeader>
        <CardContent>
          {loading ? (
            <p className="text-muted-foreground py-4">Loading...</p>
          ) : containers.length === 0 ? (
            <p className="text-muted-foreground py-4">No containers</p>
          ) : (
            <div className="space-y-2">
              <div className="grid grid-cols-5 gap-4 px-4 py-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                <div>ID</div>
                <div>Name</div>
                <div>Owner</div>
                <div>Status</div>
                <div>External IP</div>
              </div>
              {containers.map((c) => (
                <div
                  key={c.id}
                  className="grid grid-cols-5 gap-4 px-4 py-3 bg-secondary rounded-md items-center"
                >
                  <CopyableText text={c.id.slice(0, 8)} mono />
                  <span className="font-medium truncate">{c.name}</span>
                  <span className="text-sm text-muted-foreground truncate">{c.owner || "—"}</span>
                  <StatusBadge status={c.status} />
                  <span className="text-sm text-muted-foreground font-mono">
                    {c.external_ip || "—"}
                  </span>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
