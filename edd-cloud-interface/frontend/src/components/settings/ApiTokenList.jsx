import { useState, useEffect } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { useAuth } from "@/contexts/AuthContext";
import {
  buildAuthBase,
  getAuthHeaders,
  copyToClipboard,
} from "@/lib/api";
import {
  Plus,
  Trash2,
  Copy,
  AlertTriangle,
  KeyRound,
} from "lucide-react";
import {
  scopeSummary,
  PermissionPicker,
  EXPIRY_OPTIONS,
} from "@/components/service-accounts/PermissionPicker";

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

export function ApiTokenList() {
  const { userId } = useAuth();
  const [tokens, setTokens] = useState([]);
  const [loading, setLoading] = useState(true);
  const [showCreate, setShowCreate] = useState(false);
  const [createdToken, setCreatedToken] = useState(null);

  const loadTokens = async () => {
    try {
      const res = await fetch(`${buildAuthBase()}/api/tokens`, {
        headers: getAuthHeaders(),
      });
      if (res.ok) {
        const data = await res.json();
        // Filter out service-account tokens (they're managed under Service Accounts)
        setTokens((data || []).filter((t) => !t.service_account_id));
      }
    } catch (err) {
      console.warn("Failed to load tokens:", err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadTokens();
  }, []);

  const handleDelete = async (id) => {
    try {
      const res = await fetch(`${buildAuthBase()}/api/tokens/${id}`, {
        method: "DELETE",
        headers: getAuthHeaders(),
      });
      if (res.ok) {
        setTokens((prev) => prev.filter((t) => t.id !== id));
      }
    } catch (err) {
      console.warn("Failed to delete token:", err);
    }
  };

  const handleCreated = (token, meta) => {
    setCreatedToken(token);
    setShowCreate(false);
    loadTokens();
  };

  if (loading) {
    return (
      <Card>
        <CardContent className="py-8 text-center text-muted-foreground">
          Loading tokens...
        </CardContent>
      </Card>
    );
  }

  return (
    <div className="space-y-4">
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

      {/* Create form */}
      {showCreate ? (
        <CreateTokenForm
          userId={userId}
          onCreated={handleCreated}
          onCancel={() => setShowCreate(false)}
        />
      ) : (
        <div className="flex justify-end">
          <Button onClick={() => setShowCreate(true)}>
            <Plus className="w-4 h-4 mr-2" />
            Create token
          </Button>
        </div>
      )}

      {/* Token list */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Active tokens</CardTitle>
        </CardHeader>
        <CardContent>
          {tokens.length === 0 ? (
            <div className="text-center py-8 text-muted-foreground">
              <KeyRound className="w-8 h-8 mx-auto mb-2 opacity-50" />
              <p>No API tokens yet</p>
              <p className="text-xs mt-1">
                Create a token for programmatic access to compute and storage APIs.
              </p>
            </div>
          ) : (
            <div className="space-y-3">
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
                    <div className="text-xs text-muted-foreground space-y-0.5">
                      <p>{scopeSummary(token.scopes)}</p>
                      <p>
                        Created {formatDate(token.created_at)}
                        {token.expires_at > 0 && ` · Expires in ${formatRelative(token.expires_at)}`}
                        {token.last_used_at > 0 && ` · Last used ${formatDate(token.last_used_at)}`}
                      </p>
                    </div>
                  </div>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="text-muted-foreground hover:text-destructive"
                    onClick={() => handleDelete(token.id)}
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

function CreateTokenForm({ userId, onCreated, onCancel }) {
  const [name, setName] = useState("");
  const [expiresIn, setExpiresIn] = useState("90d");
  const [selectedScopes, setSelectedScopes] = useState({});
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState("");

  const handleSubmit = async (e) => {
    e.preventDefault();
    setError("");

    if (!name.trim()) {
      setError("Token name is required");
      return;
    }
    if (Object.keys(selectedScopes).length === 0) {
      setError("Select at least one permission");
      return;
    }

    setCreating(true);
    try {
      const res = await fetch(`${buildAuthBase()}/api/tokens`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          ...getAuthHeaders(),
        },
        body: JSON.stringify({
          name: name.trim(),
          scopes: selectedScopes,
          expires_in: expiresIn,
        }),
      });

      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error(data.error || "Failed to create token");
      }

      const data = await res.json();
      onCreated(data.token, data);
    } catch (err) {
      setError(err.message);
    } finally {
      setCreating(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Create API token</CardTitle>
      </CardHeader>
      <CardContent>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="token-name">Name</Label>
              <Input
                id="token-name"
                placeholder="e.g., CI/CD pipeline"
                value={name}
                onChange={(e) => setName(e.target.value)}
                autoFocus
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="token-expiry">Expiry</Label>
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

          <PermissionPicker
            userId={userId}
            selectedScopes={selectedScopes}
            setSelectedScopes={setSelectedScopes}
          />

          {error && <p className="text-sm text-destructive">{error}</p>}

          <div className="flex gap-2 justify-end">
            <Button type="button" variant="outline" onClick={onCancel}>
              Cancel
            </Button>
            <Button type="submit" disabled={creating}>
              {creating ? "Creating..." : "Create token"}
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  );
}
