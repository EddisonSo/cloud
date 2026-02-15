import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Trash2, Copy } from "lucide-react";
import { copyToClipboard } from "@/lib/api";
import type { SshKey } from "@/types";

interface SshKeyListProps {
  sshKeys: SshKey[];
  onAdd: (name: string, publicKey: string) => Promise<unknown>;
  onDelete: (id: string) => Promise<void>;
  loading: boolean;
  showAdd?: boolean;
  onCloseAdd?: () => void;
}

export function SshKeyList({
  sshKeys,
  onAdd,
  onDelete,
  loading,
  showAdd = false,
  onCloseAdd,
}: SshKeyListProps) {
  const [name, setName] = useState<string>("");
  const [publicKey, setPublicKey] = useState<string>("");
  const [adding, setAdding] = useState<boolean>(false);
  const [error, setError] = useState<string>("");

  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (!name.trim() || !publicKey.trim()) return;
    setAdding(true);
    setError("");
    try {
      await onAdd?.(name.trim(), publicKey.trim());
      setName("");
      setPublicKey("");
      onCloseAdd?.();
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setAdding(false);
    }
  };

  return (
    <div className="space-y-4">
      {/* Add Key Form */}
      {showAdd ? (
        <Card>
          <CardHeader className="pb-4">
            <CardTitle className="text-sm font-semibold">Add SSH Key</CardTitle>
          </CardHeader>
          <CardContent>
            <form onSubmit={handleSubmit} className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="key-name">Name</Label>
                <Input
                  id="key-name"
                  placeholder="My laptop"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="key-public">Public Key</Label>
                <Textarea
                  id="key-public"
                  placeholder="ssh-rsa AAAA..."
                  className="font-mono text-xs"
                  value={publicKey}
                  onChange={(e) => setPublicKey(e.target.value)}
                  rows={3}
                />
              </div>
              {error && <p className="text-destructive text-sm">{error}</p>}
              <div className="flex justify-end gap-2">
                <Button type="button" variant="outline" onClick={() => onCloseAdd?.()}>
                  Cancel
                </Button>
                <Button type="submit" disabled={adding}>
                  {adding ? "Adding..." : "Add Key"}
                </Button>
              </div>
            </form>
          </CardContent>
        </Card>
      ) : null}

      {/* Key List */}
      {loading ? (
        <p className="text-muted-foreground py-4">Loading SSH keys...</p>
      ) : sshKeys.length === 0 ? (
        <p className="text-muted-foreground py-4">No SSH keys yet. Add one to access your containers.</p>
      ) : (
        <div className="space-y-2">
          {/* Header */}
          <div className="grid grid-cols-[1fr_3fr_80px] gap-4 px-4 py-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
            <div className="text-center">Name</div>
            <div className="text-center">Public Key</div>
            <div className="text-center">Actions</div>
          </div>
          {/* Rows */}
          {sshKeys.map((key) => (
            <div
              key={key.id}
              className="grid grid-cols-[1fr_3fr_80px] gap-4 px-4 py-3 bg-secondary rounded-md items-center"
            >
              <div className="text-center">
                <span className="font-medium truncate block">{key.name}</span>
              </div>
              <div className="min-w-0 flex items-center justify-center gap-2">
                <span className="text-xs text-muted-foreground font-mono truncate">
                  {key.public_key?.slice(0, 50)}...
                </span>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-7 w-7 shrink-0"
                  onClick={() => copyToClipboard(key.public_key)}
                  title="Copy public key"
                >
                  <Copy className="w-3.5 h-3.5" />
                </Button>
              </div>
              <div className="flex justify-center">
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-8 w-8 text-destructive hover:text-destructive hover:bg-destructive/10"
                  onClick={() => onDelete?.(key.id)}
                  title="Delete key"
                >
                  <Trash2 className="w-4 h-4" />
                </Button>
              </div>
            </div>
          ))}
        </div>
      )}

      </div>
  );
}
