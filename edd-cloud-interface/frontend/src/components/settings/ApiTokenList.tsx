import { useState, useEffect } from "react";
import { useNavigate } from "react-router-dom";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  buildAuthBase,
  getAuthHeaders,
  copyToClipboard,
} from "@/lib/api";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Plus,
  Copy,
  AlertTriangle,
  KeyRound,
} from "lucide-react";
import { EXPIRY_OPTIONS } from "@/components/service-accounts/PermissionPicker";
import type { ApiToken, ServiceAccount } from "@/types";

interface ApiTokenWithTimestamps extends Omit<ApiToken, "expires_at" | "created_at"> {
  expires_at: number;
  created_at: number;
}

function formatDate(unix: number | undefined): string {
  if (!unix) return "Never";
  return new Date(unix * 1000).toLocaleDateString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}

function formatRelative(unix: number | undefined): string {
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

export function ApiTokenList(): React.ReactElement {
  const navigate = useNavigate();
  const [tokens, setTokens] = useState<ApiTokenWithTimestamps[]>([]);
  const [serviceAccounts, setServiceAccounts] = useState<ServiceAccount[]>([]);
  const [loading, setLoading] = useState<boolean>(true);
  const [showCreate, setShowCreate] = useState<boolean>(false);
  const [createdToken, setCreatedToken] = useState<string | null>(null);

  const loadData = async (): Promise<void> => {
    try {
      const [tokensRes, saRes] = await Promise.all([
        fetch(`${buildAuthBase()}/api/tokens`, { headers: getAuthHeaders() }),
        fetch(`${buildAuthBase()}/api/service-accounts`, { headers: getAuthHeaders() }),
      ]);
      if (tokensRes.ok) {
        const data: ApiTokenWithTimestamps[] = await tokensRes.json();
        setTokens(data || []);
      }
      if (saRes.ok) {
        const data: ServiceAccount[] = await saRes.json();
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

  const handleCreated = (token: string): void => {
    setCreatedToken(token);
    setShowCreate(false);
    loadData();
  };

  // Build lookup for service account names
  const saMap: Record<string, string> = {};
  for (const sa of serviceAccounts) {
    saMap[sa.id] = sa.name;
  }

  if (loading) {
    return (
      <div className="space-y-4">
        <div className="flex justify-end">
          <Skeleton className="h-9 w-32" />
        </div>
        <Card>
          <CardHeader>
            <Skeleton className="h-5 w-28" />
          </CardHeader>
          <CardContent className="space-y-2">
            {[...Array(3)].map((_, i) => (
              <div key={i} className="grid grid-cols-[1fr_120px_100px_120px] gap-4 px-4 py-3 bg-secondary rounded-md">
                <Skeleton className="h-5 w-32" />
                <Skeleton className="h-4 w-20 mx-auto" />
                <Skeleton className="h-4 w-16 mx-auto" />
                <Skeleton className="h-4 w-20 mx-auto" />
              </div>
            ))}
          </CardContent>
        </Card>
      </div>
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
          <CardTitle className="text-base">Active Tokens</CardTitle>
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
            <div className="space-y-2">
              {/* Header */}
              <div className="grid grid-cols-[1fr_120px_100px_120px] gap-4 px-4 py-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                <div>Name</div>
                <div className="text-center">Account</div>
                <div className="text-center">Expires</div>
                <div className="text-center">Created</div>
              </div>
              {/* Rows */}
              {tokens.map((token) => {
                const expired = token.expires_at > 0 && token.expires_at < Date.now() / 1000;
                return (
                  <div
                    key={token.id}
                    className={`grid grid-cols-[1fr_120px_100px_120px] gap-4 px-4 py-3 bg-secondary rounded-md items-center${expired ? " opacity-60" : ""}`}
                  >
                    <div className="flex items-center gap-2 min-w-0">
                      <KeyRound className="w-4 h-4 text-muted-foreground shrink-0" />
                      <span className="font-medium truncate">{token.name}</span>
                    </div>
                    <div className="text-center text-sm truncate">
                      {token.service_account_id && saMap[token.service_account_id] ? (
                        <button
                          onClick={() => navigate(`/service-accounts/${token.service_account_id}`)}
                          className="text-primary hover:underline cursor-pointer"
                        >
                          {saMap[token.service_account_id!]}
                        </button>
                      ) : (
                        <span className="text-muted-foreground">standalone</span>
                      )}
                    </div>
                    <div className="text-center text-sm">
                      {expired ? (
                        <span className="text-destructive">Expired</span>
                      ) : token.expires_at > 0 ? (
                        <span className="text-muted-foreground">{formatRelative(token.expires_at)}</span>
                      ) : (
                        <span className="text-muted-foreground">Never</span>
                      )}
                    </div>
                    <div className="text-center text-sm text-muted-foreground">
                      {formatDate(token.created_at)}
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

interface CreateTokenFormProps {
  serviceAccounts: ServiceAccount[];
  onCreated: (token: string) => void;
  onCancel: () => void;
}

function CreateTokenForm({ serviceAccounts, onCreated, onCancel }: CreateTokenFormProps): React.ReactElement {
  const [name, setName] = useState<string>("");
  const [saId, setSaId] = useState<string>(serviceAccounts[0]?.id || "");
  const [expiresIn, setExpiresIn] = useState<string>("90d");
  const [creating, setCreating] = useState<boolean>(false);
  const [error, setError] = useState<string>("");

  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>): Promise<void> => {
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
        throw new Error((data as { error?: string }).error || "Failed to create token");
      }

      const data: { token: string } = await res.json();
      onCreated(data.token);
    } catch (err) {
      setError((err as Error).message);
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
                onChange={(e: React.ChangeEvent<HTMLInputElement>) => setName(e.target.value)}
                autoFocus
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="token-sa">Service Account</Label>
              <select
                id="token-sa"
                value={saId}
                onChange={(e: React.ChangeEvent<HTMLSelectElement>) => setSaId(e.target.value)}
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
                onChange={(e: React.ChangeEvent<HTMLSelectElement>) => setExpiresIn(e.target.value)}
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
