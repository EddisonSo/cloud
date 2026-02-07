import { useState, useEffect } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import {
  buildAuthBase,
  getAuthHeaders,
  copyToClipboard,
} from "@/lib/api";
import {
  Plus,
  Copy,
  AlertTriangle,
  KeyRound,
} from "lucide-react";
import { EXPIRY_OPTIONS } from "@/components/service-accounts/PermissionPicker";

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
  const [tokens, setTokens] = useState([]);
  const [serviceAccounts, setServiceAccounts] = useState([]);
  const [loading, setLoading] = useState(true);
  const [showCreate, setShowCreate] = useState(false);
  const [createdToken, setCreatedToken] = useState(null);

  const loadData = async () => {
    try {
      const [tokensRes, saRes] = await Promise.all([
        fetch(`${buildAuthBase()}/api/tokens`, { headers: getAuthHeaders() }),
        fetch(`${buildAuthBase()}/api/service-accounts`, { headers: getAuthHeaders() }),
      ]);
      if (tokensRes.ok) {
        const data = await tokensRes.json();
        setTokens(data || []);
      }
      if (saRes.ok) {
        const data = await saRes.json();
        setServiceAccounts(data || []);
      }
    } catch (err) {
      console.warn("Failed to load data:", err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadData();
  }, []);

  const handleCreated = (token) => {
    setCreatedToken(token);
    setShowCreate(false);
    loadData();
  };

  // Build lookup for service account names
  const saMap = {};
  for (const sa of serviceAccounts) {
    saMap[sa.id] = sa.name;
  }

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
          serviceAccounts={serviceAccounts}
          onCreated={handleCreated}
          onCancel={() => setShowCreate(false)}
        />
      ) : (
        <div className="flex justify-end">
          <Button onClick={() => setShowCreate(true)} disabled={serviceAccounts.length === 0}>
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
                Create a token bound to a service account for programmatic API access.
              </p>
            </div>
          ) : (
            <div className="space-y-3">
              {tokens.map((token) => (
                <div
                  key={token.id}
                  className="p-3 rounded-md border border-border bg-secondary/30"
                >
                  <div className="flex items-center gap-2 mb-1">
                    <span className="font-medium text-sm">{token.name}</span>
                    {token.service_account_id && saMap[token.service_account_id] && (
                      <Badge variant="outline" className="text-xs">
                        {saMap[token.service_account_id]}
                      </Badge>
                    )}
                    {!token.service_account_id && (
                      <Badge variant="secondary" className="text-xs">standalone</Badge>
                    )}
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
              ))}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function CreateTokenForm({ serviceAccounts, onCreated, onCancel }) {
  const [name, setName] = useState("");
  const [saId, setSaId] = useState(serviceAccounts[0]?.id || "");
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
    if (!saId) {
      setError("Select a service account");
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
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Create API token</CardTitle>
      </CardHeader>
      <CardContent>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="grid grid-cols-3 gap-4">
            <div className="space-y-2">
              <Label htmlFor="token-name">Name</Label>
              <Input
                id="token-name"
                placeholder="e.g., production"
                value={name}
                onChange={(e) => setName(e.target.value)}
                autoFocus
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="token-sa">Service Account</Label>
              <select
                id="token-sa"
                value={saId}
                onChange={(e) => setSaId(e.target.value)}
                className="w-full h-9 rounded-md border border-border bg-background px-3 text-sm"
              >
                {serviceAccounts.map((sa) => (
                  <option key={sa.id} value={sa.id}>
                    {sa.name}
                  </option>
                ))}
              </select>
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
