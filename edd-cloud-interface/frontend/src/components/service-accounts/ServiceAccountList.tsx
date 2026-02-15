import { useState, useEffect } from "react";
import { useNavigate } from "react-router-dom";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useAuth } from "@/contexts/AuthContext";
import { buildAuthBase, getAuthHeaders } from "@/lib/api";
import { PermissionPicker } from "./PermissionPicker";
import { Skeleton } from "@/components/ui/skeleton";
import { KeyRound } from "lucide-react";
import type { ServiceAccount } from "@/types";

function formatDate(unix: number | undefined): string {
  if (!unix) return "Never";
  return new Date(unix * 1000).toLocaleDateString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}

interface ServiceAccountListProps {
  showCreate?: boolean;
  onCloseCreate?: () => void;
}

export function ServiceAccountList({ showCreate = false, onCloseCreate }: ServiceAccountListProps): React.ReactElement {
  const { userId } = useAuth();
  const navigate = useNavigate();
  const [accounts, setAccounts] = useState<ServiceAccount[]>([]);
  const [loading, setLoading] = useState<boolean>(true);

  const loadAccounts = async (): Promise<void> => {
    try {
      const res = await fetch(`${buildAuthBase()}/api/service-accounts`, {
        headers: getAuthHeaders(),
      });
      if (res.ok) {
        const data: ServiceAccount[] = await res.json();
        setAccounts(data || []);
      }
    } catch (err) {
      console.warn("Failed to load service accounts:", err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadAccounts();
  }, []);

  const handleCreated = (): void => {
    onCloseCreate?.();
    loadAccounts();
  };

  if (loading) {
    return (
      <div className="bg-card border border-border rounded-lg">
        <div className="px-5 py-4 border-b border-border">
          <h2 className="text-sm font-semibold">Service Accounts</h2>
        </div>
        <div className="p-5 space-y-2">
          {[...Array(3)].map((_, i) => (
            <div key={i} className="grid grid-cols-[1fr_100px_120px] gap-4 px-4 py-3 bg-secondary rounded-md">
              <Skeleton className="h-5 w-32" />
              <Skeleton className="h-4 w-16 mx-auto" />
              <Skeleton className="h-4 w-20 mx-auto" />
            </div>
          ))}
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {showCreate && (
        <CreateServiceAccountForm
          userId={userId}
          onCreated={handleCreated}
          onCancel={() => onCloseCreate?.()}
        />
      )}

      <div className="bg-card border border-border rounded-lg">
        <div className="px-5 py-4 border-b border-border">
          <h2 className="text-sm font-semibold">Service Accounts</h2>
        </div>
        <div className="p-5">
          {accounts.length === 0 ? (
            <div className="text-center py-8 text-muted-foreground">
              <KeyRound className="w-8 h-8 mx-auto mb-2 opacity-50" />
              <p>No service accounts yet</p>
              <p className="text-xs mt-1">
                Create a service account to manage scoped API access.
              </p>
            </div>
          ) : (
            <div className="space-y-2">
              {/* Header */}
              <div className="grid grid-cols-[1fr_100px_120px] gap-4 px-4 py-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                <div>Name</div>
                <div className="text-center">Tokens</div>
                <div className="text-center">Created</div>
              </div>
              {/* Rows */}
              {accounts.map((sa) => (
                <button
                  key={sa.id}
                  onClick={() => navigate(`/service-accounts/${sa.id}`)}
                  className="w-full grid grid-cols-[1fr_100px_120px] gap-4 px-4 py-3 bg-secondary rounded-md items-center cursor-pointer transition-all hover:bg-secondary/80"
                >
                  <div className="flex items-center gap-2 min-w-0 text-left">
                    <KeyRound className="w-4 h-4 text-muted-foreground shrink-0" />
                    <span className="font-medium truncate">{sa.name}</span>
                  </div>
                  <div className="text-center text-sm text-muted-foreground">
                    {sa.token_count} {sa.token_count === 1 ? "token" : "tokens"}
                  </div>
                  <div className="text-center text-sm text-muted-foreground">
                    {formatDate(sa.created_at as unknown as number)}
                  </div>
                </button>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

interface CreateServiceAccountFormProps {
  userId: string | null | undefined;
  onCreated: () => void;
  onCancel: () => void;
}

function CreateServiceAccountForm({ userId, onCreated, onCancel }: CreateServiceAccountFormProps): React.ReactElement {
  const [name, setName] = useState<string>("");
  const [selectedScopes, setSelectedScopes] = useState<Record<string, string[]>>({});
  const [creating, setCreating] = useState<boolean>(false);
  const [error, setError] = useState<string>("");

  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>): Promise<void> => {
    e.preventDefault();
    setError("");

    if (!name.trim()) {
      setError("Name is required");
      return;
    }
    if (Object.keys(selectedScopes).length === 0) {
      setError("Select at least one permission");
      return;
    }

    setCreating(true);
    try {
      const res = await fetch(`${buildAuthBase()}/api/service-accounts`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          ...getAuthHeaders(),
        },
        body: JSON.stringify({
          name: name.trim(),
          scopes: selectedScopes,
        }),
      });

      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error((data as { error?: string }).error || "Failed to create service account");
      }

      onCreated();
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setCreating(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Create service account</CardTitle>
      </CardHeader>
      <CardContent>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="sa-name">Name</Label>
            <Input
              id="sa-name"
              placeholder="e.g., CI/CD pipeline"
              value={name}
              onChange={(e: React.ChangeEvent<HTMLInputElement>) => setName(e.target.value)}
              autoFocus
            />
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
              {creating ? "Creating..." : "Create"}
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  );
}
