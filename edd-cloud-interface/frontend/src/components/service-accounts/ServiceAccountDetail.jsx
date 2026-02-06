import { useState, useEffect } from "react";
import { useNavigate } from "react-router-dom";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { useAuth } from "@/contexts/AuthContext";
import { buildAuthBase, getAuthHeaders, copyToClipboard } from "@/lib/api";
import {
  PermissionPicker,
  EXPIRY_OPTIONS,
} from "./PermissionPicker";
import {
  Trash2,
  Copy,
  AlertTriangle,
  ArrowLeft,
  Plus,
  KeyRound,
} from "lucide-react";

function formatDate(unix) {
  if (!unix) return "Never";
  return new Date(unix * 1000).toLocaleDateString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}

function formatRelative(unix) {
  if (!unix) return "Never";
  const now = Date.now() / 1000;
  if (unix < now) return "Expired";
  const days = Math.ceil((unix - now) / 86400);
  if (days === 1) return "1 day";
  if (days < 30) return `${days} days`;
  const months = Math.round(days / 30);
  if (months < 12) return `${months} month${months > 1 ? "s" : ""}`;
  return `${Math.round(days / 365)} year${Math.round(days / 365) > 1 ? "s" : ""}`;
}

export function ServiceAccountDetail({ id }) {
  const { userId } = useAuth();
  const navigate = useNavigate();
  const [account, setAccount] = useState(null);
  const [tokens, setTokens] = useState([]);
  const [loading, setLoading] = useState(true);
  const [editScopes, setEditScopes] = useState(null);
  const [saving, setSaving] = useState(false);
  const [showCreateToken, setShowCreateToken] = useState(false);
  const [createdToken, setCreatedToken] = useState(null);
  const [deleting, setDeleting] = useState(false);

  const loadAccount = async () => {
    try {
      const [saRes, tokensRes] = await Promise.all([
        fetch(`${buildAuthBase()}/api/service-accounts/${id}`, {
          headers: getAuthHeaders(),
        }),
        fetch(`${buildAuthBase()}/api/service-accounts/${id}/tokens`, {
          headers: getAuthHeaders(),
        }),
      ]);
      if (saRes.ok) {
        const sa = await saRes.json();
        setAccount(sa);
        setEditScopes(null);
      }
      if (tokensRes.ok) {
        const data = await tokensRes.json();
        setTokens(data || []);
      }
    } catch (err) {
      console.warn("Failed to load service account:", err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadAccount();
  }, [id]);

  const handleSaveScopes = async () => {
    if (!editScopes || Object.keys(editScopes).length === 0) return;
    setSaving(true);
    try {
      const res = await fetch(`${buildAuthBase()}/api/service-accounts/${id}/scopes`, {
        method: "PUT",
        headers: {
          "Content-Type": "application/json",
          ...getAuthHeaders(),
        },
        body: JSON.stringify({ scopes: editScopes }),
      });
      if (res.ok) {
        setAccount((prev) => ({ ...prev, scopes: editScopes }));
        setEditScopes(null);
      }
    } catch (err) {
      console.warn("Failed to update scopes:", err);
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async () => {
    if (!confirm("Delete this service account? All its tokens will be revoked.")) return;
    setDeleting(true);
    try {
      const res = await fetch(`${buildAuthBase()}/api/service-accounts/${id}`, {
        method: "DELETE",
        headers: getAuthHeaders(),
      });
      if (res.ok) {
        navigate("/service-accounts");
      }
    } catch (err) {
      console.warn("Failed to delete service account:", err);
    } finally {
      setDeleting(false);
    }
  };

  const handleDeleteToken = async (tokenId) => {
    try {
      const res = await fetch(`${buildAuthBase()}/api/tokens/${tokenId}`, {
        method: "DELETE",
        headers: getAuthHeaders(),
      });
      if (res.ok) {
        setTokens((prev) => prev.filter((t) => t.id !== tokenId));
      }
    } catch (err) {
      console.warn("Failed to delete token:", err);
    }
  };

  const handleTokenCreated = (token) => {
    setCreatedToken(token);
    setShowCreateToken(false);
    loadAccount();
  };

  if (loading) {
    return (
      <Card>
        <CardContent className="py-8 text-center text-muted-foreground">
          Loading...
        </CardContent>
      </Card>
    );
  }

  if (!account) {
    return (
      <Card>
        <CardContent className="py-8 text-center text-muted-foreground">
          Service account not found.
        </CardContent>
      </Card>
    );
  }

  const isEditing = editScopes !== null;
  const currentScopes = isEditing ? editScopes : account.scopes;

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <Button
          variant="ghost"
          size="sm"
          onClick={() => navigate("/service-accounts")}
        >
          <ArrowLeft className="w-4 h-4 mr-1" />
          Back
        </Button>
        <Button
          variant="destructive"
          size="sm"
          onClick={handleDelete}
          disabled={deleting}
        >
          <Trash2 className="w-4 h-4 mr-1" />
          {deleting ? "Deleting..." : "Delete"}
        </Button>
      </div>

      {/* Permissions */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle className="text-base">{account.name}</CardTitle>
            <div className="flex gap-2">
              {isEditing ? (
                <>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setEditScopes(null)}
                  >
                    Cancel
                  </Button>
                  <Button
                    size="sm"
                    onClick={handleSaveScopes}
                    disabled={saving || Object.keys(editScopes).length === 0}
                  >
                    {saving ? "Saving..." : "Save"}
                  </Button>
                </>
              ) : (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setEditScopes({ ...account.scopes })}
                >
                  Edit permissions
                </Button>
              )}
            </div>
          </div>
        </CardHeader>
        <CardContent>
          {isEditing ? (
            <PermissionPicker
              userId={userId}
              selectedScopes={editScopes}
              setSelectedScopes={setEditScopes}
            />
          ) : (
            <div className="text-sm text-muted-foreground">
              {Object.entries(currentScopes).map(([scope, actions]) => {
                const segments = scope.split(".");
                let label;
                if (segments.length === 4) {
                  label = `${segments[0]} > ${segments[2]} > ${segments[3]}`;
                } else if (segments.length === 3) {
                  label = `${segments[0]} > ${segments[2]}`;
                } else {
                  label = segments[0];
                }
                return (
                  <div key={scope} className="flex items-center gap-2 py-1">
                    <span className="font-medium text-foreground">{label}</span>
                    <div className="flex gap-1">
                      {actions.map((a) => (
                        <Badge key={a} variant="secondary" className="text-xs">
                          {a}
                        </Badge>
                      ))}
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </CardContent>
      </Card>

      {/* Created token display */}
      {createdToken && (
        <Card className="border-primary/50">
          <CardContent className="py-4">
            <div className="flex items-start gap-3">
              <AlertTriangle className="w-5 h-5 text-primary mt-0.5 flex-shrink-0" />
              <div className="flex-1 min-w-0">
                <p className="text-sm font-medium mb-2">
                  Your new API token. Copy it now — it won't be shown again.
                </p>
                <div className="flex items-center gap-2">
                  <code className="flex-1 text-xs bg-secondary px-3 py-2 rounded font-mono break-all select-all">
                    {createdToken}
                  </code>
                  <Button
                    variant="outline"
                    size="icon"
                    onClick={() => copyToClipboard(createdToken)}
                  >
                    <Copy className="w-4 h-4" />
                  </Button>
                </div>
              </div>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setCreatedToken(null)}
              >
                Dismiss
              </Button>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Tokens */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle className="text-base">Tokens</CardTitle>
            {!showCreateToken && (
              <Button size="sm" onClick={() => setShowCreateToken(true)}>
                <Plus className="w-4 h-4 mr-1" />
                Create token
              </Button>
            )}
          </div>
        </CardHeader>
        <CardContent>
          {showCreateToken && (
            <CreateTokenForm
              saId={id}
              onCreated={handleTokenCreated}
              onCancel={() => setShowCreateToken(false)}
            />
          )}

          {tokens.length === 0 && !showCreateToken ? (
            <div className="text-center py-6 text-muted-foreground">
              <KeyRound className="w-6 h-6 mx-auto mb-2 opacity-50" />
              <p className="text-sm">No tokens yet</p>
              <p className="text-xs mt-1">
                Tokens inherit this service account's permissions.
              </p>
            </div>
          ) : (
            <div className="space-y-3 mt-3">
              {tokens.map((token) => (
                <div
                  key={token.id}
                  className="flex items-center justify-between p-3 rounded-md border border-border bg-secondary/30"
                >
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2 mb-1">
                      <span className="font-medium text-sm">{token.name}</span>
                      {token.expires_at > 0 && token.expires_at < Date.now() / 1000 && (
                        <Badge variant="destructive" className="text-xs">Expired</Badge>
                      )}
                    </div>
                    <div className="text-xs text-muted-foreground">
                      Created {formatDate(token.created_at)}
                      {token.expires_at > 0 && ` · Expires in ${formatRelative(token.expires_at)}`}
                      {token.last_used_at > 0 && ` · Last used ${formatDate(token.last_used_at)}`}
                    </div>
                  </div>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="text-muted-foreground hover:text-destructive"
                    onClick={() => handleDeleteToken(token.id)}
                  >
                    <Trash2 className="w-4 h-4" />
                  </Button>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function CreateTokenForm({ saId, onCreated, onCancel }) {
  const [name, setName] = useState("");
  const [expiresIn, setExpiresIn] = useState("90d");
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState("");

  const handleSubmit = async (e) => {
    e.preventDefault();
    setError("");

    if (!name.trim()) {
      setError("Token name is required");
      return;
    }

    setCreating(true);
    try {
      const res = await fetch(`${buildAuthBase()}/api/service-accounts/${saId}/tokens`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          ...getAuthHeaders(),
        },
        body: JSON.stringify({
          name: name.trim(),
          expires_in: expiresIn,
        }),
      });

      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error(data.error || "Failed to create token");
      }

      const data = await res.json();
      onCreated(data.token);
    } catch (err) {
      setError(err.message);
    } finally {
      setCreating(false);
    }
  };

  return (
    <form onSubmit={handleSubmit} className="space-y-3 p-3 border border-border rounded-md mb-3">
      <div className="grid grid-cols-2 gap-3">
        <div className="space-y-1.5">
          <Label htmlFor="token-name" className="text-xs">Name</Label>
          <Input
            id="token-name"
            placeholder="e.g., production"
            value={name}
            onChange={(e) => setName(e.target.value)}
            autoFocus
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="token-expiry" className="text-xs">Expiry</Label>
          <select
            id="token-expiry"
            value={expiresIn}
            onChange={(e) => setExpiresIn(e.target.value)}
            className="w-full h-9 rounded-md border border-border bg-background px-3 text-sm"
          >
            {EXPIRY_OPTIONS.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {opt.label}
              </option>
            ))}
          </select>
        </div>
      </div>
      {error && <p className="text-sm text-destructive">{error}</p>}
      <div className="flex gap-2 justify-end">
        <Button type="button" variant="outline" size="sm" onClick={onCancel}>
          Cancel
        </Button>
        <Button type="submit" size="sm" disabled={creating}>
          {creating ? "Creating..." : "Create token"}
        </Button>
      </div>
    </form>
  );
}
