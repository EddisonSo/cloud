import { useState, useEffect } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Shield, Plus, Trash2, Pencil, Check, X, AlertTriangle } from "lucide-react";
import {
  fetchSecurityKeys,
  beginAddKey,
  finishAddKey,
  deleteKey,
  renameKey,
} from "@/lib/settings-api";
import {
  isWebAuthnSupported,
  parseCreationOptions,
  serializeCreationResponse,
  createCredential,
} from "@/lib/webauthn";
import { formatTimestamp } from "@/lib/formatters";
import type { SecurityKey } from "@/types";

export function SecurityKeys() {
  const [keys, setKeys] = useState<SecurityKey[]>([]);
  const [loading, setLoading] = useState(true);
  const [adding, setAdding] = useState(false);
  const [error, setError] = useState("");
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editName, setEditName] = useState("");
  const [deletingId, setDeletingId] = useState<string | null>(null);

  const supported = isWebAuthnSupported();

  const load = async () => {
    try {
      setKeys(await fetchSecurityKeys());
    } catch {
      // No keys yet is fine
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { load(); }, []);

  const handleAdd = async () => {
    setError("");
    setAdding(true);
    try {
      const { options, state } = await beginAddKey();
      const parsed = parseCreationOptions(options);
      const credential = await createCredential(parsed);
      const serialized = serializeCreationResponse(credential);
      await finishAddKey(state, serialized);
      await load();
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setAdding(false);
    }
  };

  const handleDelete = async (id: string) => {
    setError("");
    try {
      await deleteKey(id);
      setDeletingId(null);
      await load();
    } catch (err) {
      setError((err as Error).message);
    }
  };

  const handleRename = async (id: string) => {
    if (!editName.trim()) return;
    setError("");
    try {
      await renameKey(id, editName.trim());
      setEditingId(null);
      await load();
    } catch (err) {
      setError((err as Error).message);
    }
  };

  if (loading) {
    return (
      <Card>
        <CardHeader>
          <Skeleton className="h-5 w-32" />
        </CardHeader>
        <CardContent className="space-y-2">
          {[...Array(2)].map((_, i) => (
            <div key={i} className="flex items-center gap-4 px-4 py-3 bg-secondary rounded-md">
              <Skeleton className="h-5 w-40" />
              <Skeleton className="h-4 w-24 ml-auto" />
            </div>
          ))}
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between">
        <CardTitle className="text-base">Security Keys</CardTitle>
        <Button
          size="sm"
          onClick={handleAdd}
          disabled={!supported || adding}
        >
          {adding ? (
            "Touch your key..."
          ) : (
            <>
              <Plus className="w-4 h-4 mr-1.5" />
              Add key
            </>
          )}
        </Button>
      </CardHeader>
      <CardContent>
        {!supported && (
          <div className="flex items-center gap-2 mb-4 p-3 rounded-md bg-destructive/10 text-destructive text-sm">
            <AlertTriangle className="w-4 h-4 shrink-0" />
            WebAuthn is not supported in this browser.
          </div>
        )}

        {error && (
          <p className="text-sm text-destructive mb-4">{error}</p>
        )}

        {keys.length === 0 ? (
          <div className="text-center py-8 text-muted-foreground">
            <Shield className="w-8 h-8 mx-auto mb-2 opacity-50" />
            <p>No security keys registered</p>
            <p className="text-xs mt-1">
              Add a security key to enable two-factor authentication on login.
            </p>
          </div>
        ) : (
          <div className="space-y-2">
            {/* Header */}
            <div className="grid grid-cols-[1fr_140px_80px] gap-4 px-4 py-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
              <div>Name</div>
              <div className="text-center">Added</div>
              <div />
            </div>
            {/* Rows */}
            {keys.map((key) => (
              <div
                key={key.id}
                className="grid grid-cols-[1fr_140px_80px] gap-4 px-4 py-3 bg-secondary rounded-md items-center"
              >
                <div className="flex items-center gap-2 min-w-0">
                  <Shield className="w-4 h-4 text-muted-foreground shrink-0" />
                  {editingId === key.id ? (
                    <div className="flex items-center gap-1 flex-1 min-w-0">
                      <Input
                        value={editName}
                        onChange={(e) => setEditName(e.target.value)}
                        className="h-7 text-sm"
                        autoFocus
                        onKeyDown={(e) => {
                          if (e.key === "Enter") handleRename(key.id);
                          if (e.key === "Escape") setEditingId(null);
                        }}
                      />
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-7 w-7 shrink-0"
                        onClick={() => handleRename(key.id)}
                      >
                        <Check className="w-3.5 h-3.5" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-7 w-7 shrink-0"
                        onClick={() => setEditingId(null)}
                      >
                        <X className="w-3.5 h-3.5" />
                      </Button>
                    </div>
                  ) : (
                    <span
                      className="font-medium truncate cursor-pointer hover:text-primary transition-colors"
                      onClick={() => {
                        setEditingId(key.id);
                        setEditName(key.name || "Security Key");
                      }}
                    >
                      {key.name || "Security Key"}
                    </span>
                  )}
                </div>
                <div className="text-center text-sm text-muted-foreground">
                  {formatTimestamp(key.created_at)}
                </div>
                <div className="flex justify-end gap-1">
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-7 w-7"
                    onClick={() => {
                      setEditingId(key.id);
                      setEditName(key.name || "Security Key");
                    }}
                  >
                    <Pencil className="w-3.5 h-3.5" />
                  </Button>
                  {deletingId === key.id ? (
                    <div className="flex gap-1">
                      <Button
                        variant="destructive"
                        size="icon"
                        className="h-7 w-7"
                        onClick={() => handleDelete(key.id)}
                      >
                        <Check className="w-3.5 h-3.5" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-7 w-7"
                        onClick={() => setDeletingId(null)}
                      >
                        <X className="w-3.5 h-3.5" />
                      </Button>
                    </div>
                  ) : (
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-7 w-7 text-muted-foreground hover:text-destructive"
                      onClick={() => setDeletingId(key.id)}
                    >
                      <Trash2 className="w-3.5 h-3.5" />
                    </Button>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
