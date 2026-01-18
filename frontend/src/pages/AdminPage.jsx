import { useState, useEffect } from "react";
import { Header } from "@/components/layout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { TextSkeleton } from "@/components/ui/skeleton";
import { StatusBadge, CopyableText, Modal } from "@/components/common";
import { TAB_COPY } from "@/lib/constants";
import { buildApiBase, getAuthHeaders } from "@/lib/api";
import { useAuth } from "@/contexts/AuthContext";
import { Trash2, UserPlus, Eye, EyeOff } from "lucide-react";

export function AdminPage() {
  const copy = TAB_COPY.admin;
  const { user, isAdmin } = useAuth();
  const [containers, setContainers] = useState([]);
  const [users, setUsers] = useState([]);
  const [namespaces, setNamespaces] = useState([]);
  const [loading, setLoading] = useState(false);
  const [newUser, setNewUser] = useState({ displayName: "", username: "", password: "" });
  const [showCreateUserModal, setShowCreateUserModal] = useState(false);
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState("");

  const parseJsonSafe = async (response) => {
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
      const [containersRes, usersRes, namespacesRes] = await Promise.all([
        fetch(`${buildApiBase()}/compute/admin/containers`, { headers: getAuthHeaders() }),
        fetch(`${buildApiBase()}/admin/users`, { headers: getAuthHeaders() }),
        fetch(`${buildApiBase()}/admin/namespaces`, { headers: getAuthHeaders() }),
      ]);
      if (containersRes.ok) {
        const data = await parseJsonSafe(containersRes);
        setContainers(data || []);
      }
      if (usersRes.ok) {
        const data = await parseJsonSafe(usersRes);
        setUsers(data || []);
      } else {
        const errText = await usersRes.text();
        setError(`Failed to load users: ${errText}`);
      }
      if (namespacesRes.ok) {
        const data = await parseJsonSafe(namespacesRes);
        setNamespaces(data || []);
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
        headers: getAuthHeaders(),
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

      {/* Summary Stats */}
      <div className="flex flex-wrap gap-4 mb-6 text-sm text-muted-foreground">
        <span>
          Users:{" "}
          {loading ? (
            <TextSkeleton text="0" className="font-medium text-foreground" />
          ) : (
            <span className="font-medium text-foreground">{users.length}</span>
          )}
        </span>
        <span>
          Containers:{" "}
          {loading ? (
            <TextSkeleton text="0" className="font-medium text-foreground" />
          ) : (
            <span className="font-medium text-foreground">{containers.length}</span>
          )}
        </span>
        <span>
          Running:{" "}
          {loading ? (
            <TextSkeleton text="0" className="font-medium text-green-400" />
          ) : (
            <span className="font-medium text-green-400">
              {containers.filter((c) => c.status === "running").length}
            </span>
          )}
        </span>
        <span>
          Namespaces:{" "}
          {loading ? (
            <TextSkeleton text="0" className="font-medium text-foreground" />
          ) : (
            <span className="font-medium text-foreground">{namespaces.length}</span>
          )}
        </span>
      </div>

      {/* Users Section */}
      <Card className="mb-6">
        <CardHeader className="flex flex-row items-center justify-between space-y-0">
          <CardTitle>Users</CardTitle>
          <Button variant="outline" onClick={() => setShowCreateUserModal(true)}>
            <UserPlus className="w-4 h-4 mr-2" />
            Add User
          </Button>
        </CardHeader>
        <CardContent>
          {error && <p className="text-destructive text-sm mb-4">{error}</p>}

          {/* Users List */}
          {loading ? (
            <p className="text-muted-foreground py-4">Loading users...</p>
          ) : users.length === 0 ? (
            <p className="text-muted-foreground py-4">No users found</p>
          ) : (
            <div className="space-y-2">
              {/* Header - hidden on mobile */}
              <div className="hidden sm:grid sm:grid-cols-[1fr_2fr_2fr_80px] gap-4 px-4 py-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                <div className="text-center">ID</div>
                <div className="text-center">Display Name</div>
                <div className="text-center">Username</div>
                <div className="text-center">Actions</div>
              </div>
              {users.map((u) => (
                <div
                  key={u.id}
                  className="flex flex-col sm:grid sm:grid-cols-[1fr_2fr_2fr_80px] gap-2 sm:gap-4 px-4 py-3 bg-secondary rounded-md sm:items-center"
                >
                  <div className="flex justify-between sm:justify-center">
                    <span className="text-xs text-muted-foreground sm:hidden">ID:</span>
                    <CopyableText text={String(u.id)} mono />
                  </div>
                  <div className="flex justify-between sm:block sm:text-center">
                    <span className="text-xs text-muted-foreground sm:hidden">Name:</span>
                    <span className="font-medium truncate">{u.display_name || u.username}</span>
                  </div>
                  <div className="flex justify-between sm:block sm:text-center">
                    <span className="text-xs text-muted-foreground sm:hidden">Username:</span>
                    <span className="text-muted-foreground truncate">{u.username}</span>
                  </div>
                  <div className="flex justify-center">
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
      <Card className="mb-6">
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
              {/* Header - hidden on mobile */}
              <div className="hidden lg:grid lg:grid-cols-[1fr_2fr_1.5fr_1fr_2fr] gap-4 px-4 py-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                <div className="text-center">ID</div>
                <div className="text-center">Name</div>
                <div className="text-center">Owner</div>
                <div className="text-center">Status</div>
                <div className="text-center">Hostname</div>
              </div>
              {containers.map((c) => (
                <div
                  key={c.id}
                  className="flex flex-col lg:grid lg:grid-cols-[1fr_2fr_1.5fr_1fr_2fr] gap-2 lg:gap-4 px-4 py-3 bg-secondary rounded-md lg:items-center"
                >
                  <div className="flex justify-between lg:justify-center">
                    <span className="text-xs text-muted-foreground lg:hidden">ID:</span>
                    <CopyableText text={c.id.slice(0, 8)} mono />
                  </div>
                  <div className="flex justify-between lg:block lg:text-center min-w-0">
                    <span className="text-xs text-muted-foreground lg:hidden">Name:</span>
                    <span className="font-medium truncate">{c.name}</span>
                  </div>
                  <div className="flex justify-between lg:block lg:text-center min-w-0">
                    <span className="text-xs text-muted-foreground lg:hidden">Owner:</span>
                    <span className="text-sm text-muted-foreground truncate">{c.owner || "—"}</span>
                  </div>
                  <div className="flex justify-between lg:justify-center items-center">
                    <span className="text-xs text-muted-foreground lg:hidden">Status:</span>
                    <StatusBadge status={c.status} />
                  </div>
                  <div className="flex justify-between lg:block lg:text-center min-w-0">
                    <span className="text-xs text-muted-foreground lg:hidden">Hostname:</span>
                    <span className="text-sm text-muted-foreground font-mono truncate">
                      {c.hostname || "—"}
                    </span>
                  </div>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      {/* Namespaces Section */}
      <Card>
        <CardHeader>
          <CardTitle>All Namespaces</CardTitle>
        </CardHeader>
        <CardContent>
          {loading ? (
            <p className="text-muted-foreground py-4">Loading...</p>
          ) : namespaces.length === 0 ? (
            <p className="text-muted-foreground py-4">No namespaces</p>
          ) : (
            <div className="space-y-2">
              {/* Header - hidden on mobile */}
              <div className="hidden sm:grid sm:grid-cols-[2fr_1fr_1fr_1fr] gap-4 px-4 py-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                <div className="text-center">Name</div>
                <div className="text-center">Files</div>
                <div className="text-center">Visibility</div>
                <div className="text-center">Owner ID</div>
              </div>
              {namespaces.map((ns) => (
                <div
                  key={ns.name}
                  className="flex flex-col sm:grid sm:grid-cols-[2fr_1fr_1fr_1fr] gap-2 sm:gap-4 px-4 py-3 bg-secondary rounded-md sm:items-center"
                >
                  <div className="flex justify-between sm:block sm:text-center min-w-0">
                    <span className="text-xs text-muted-foreground sm:hidden">Name:</span>
                    <span className="font-medium truncate">{ns.name}</span>
                  </div>
                  <div className="flex justify-between sm:block sm:text-center">
                    <span className="text-xs text-muted-foreground sm:hidden">Files:</span>
                    <span className="text-muted-foreground">{ns.count}</span>
                  </div>
                  <div className="flex justify-between sm:justify-center items-center">
                    <span className="text-xs text-muted-foreground sm:hidden">Visibility:</span>
                    <span className="flex items-center gap-1 text-sm">
                      {ns.hidden ? (
                        <>
                          <EyeOff className="w-4 h-4 text-muted-foreground" />
                          <span className="text-muted-foreground">Hidden</span>
                        </>
                      ) : (
                        <>
                          <Eye className="w-4 h-4 text-green-400" />
                          <span className="text-green-400">Public</span>
                        </>
                      )}
                    </span>
                  </div>
                  <div className="flex justify-between sm:block sm:text-center min-w-0">
                    <span className="text-xs text-muted-foreground sm:hidden">Owner:</span>
                    <span className="text-sm text-muted-foreground">
                      {ns.owner_id != null ? ns.owner_id : "System"}
                    </span>
                  </div>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

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
