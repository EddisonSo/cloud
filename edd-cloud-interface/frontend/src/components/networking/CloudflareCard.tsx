import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import type { CloudflareConnection } from "@/types";

interface ConnectionRowProps {
  connection: CloudflareConnection;
  onRefresh: (id: string) => Promise<void>;
  onRemove: (id: string) => Promise<void>;
}

function ConnectionRow({ connection, onRefresh, onRemove }: ConnectionRowProps) {
  const [refreshBusy, setRefreshBusy] = useState(false);
  const [removeBusy, setRemoveBusy] = useState(false);

  const handleRefresh = async () => {
    setRefreshBusy(true);
    try {
      await onRefresh(connection.id);
    } finally {
      setRefreshBusy(false);
    }
  };

  const handleRemove = async () => {
    setRemoveBusy(true);
    try {
      await onRemove(connection.id);
    } finally {
      setRemoveBusy(false);
    }
  };

  const zonesLabel =
    connection.zones.length > 0
      ? connection.zones.join(", ")
      : "no zones visible — refresh or reconnect";

  return (
    <div className="flex flex-col sm:flex-row sm:items-center gap-2 py-2 border-b border-border last:border-0">
      <span className="font-mono text-[12.5px] flex-1 text-muted-foreground">{zonesLabel}</span>
      <div className="flex gap-2 shrink-0">
        <Button
          size="sm"
          variant="outline"
          onClick={handleRefresh}
          disabled={refreshBusy || removeBusy}
        >
          {refreshBusy ? "Refreshing..." : "Refresh"}
        </Button>
        <Button
          size="sm"
          variant="outline"
          onClick={handleRemove}
          disabled={refreshBusy || removeBusy}
          className="text-destructive hover:text-destructive"
        >
          {removeBusy ? "Disconnecting..." : "Disconnect"}
        </Button>
      </div>
    </div>
  );
}

export function CloudflareCard({
  connections,
  onAdd,
  onRemove,
  onRefresh,
}: {
  connections: CloudflareConnection[];
  onAdd: (token: string) => Promise<CloudflareConnection>;
  onRemove: (id: string) => Promise<void>;
  onRefresh: (id: string) => Promise<void>;
}) {
  const [token, setToken] = useState("");
  const [addBusy, setAddBusy] = useState(false);
  const [err, setErr] = useState("");

  const handleAdd = async (e: React.FormEvent) => {
    e.preventDefault();
    setErr("");
    setAddBusy(true);
    try {
      await onAdd(token);
      setToken("");
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setAddBusy(false);
    }
  };

  return (
    <div className="bg-card border border-border p-5 mb-6">
      <h2 className="font-mono text-[10.5px] font-medium uppercase tracking-[0.2em] text-faint mb-1">Domains</h2>
      <p className="text-xs text-muted-foreground mb-4">
        Add a domain you own by providing a Cloudflare API token for its zone, and DNS
        records are created automatically when you map a hostname. One token per zone
        works great — scope each to Zone:Read + DNS:Edit. Add as many domains as you
        have zones.
      </p>

      {/* Owned domain list */}
      {connections.length > 0 ? (
        <div className="mb-4">
          {connections.map((conn) => (
            <ConnectionRow
              key={conn.id}
              connection={conn}
              onRefresh={onRefresh}
              onRemove={onRemove}
            />
          ))}
        </div>
      ) : (
        <p className="text-sm text-muted-foreground mb-4">
          No domains yet — add a Cloudflare API token for a zone you own to automate DNS.
        </p>
      )}

      {/* Add form */}
      <form onSubmit={handleAdd} className="flex flex-col sm:flex-row gap-2 max-w-xl">
        <Input
          type="password"
          placeholder="Cloudflare API token"
          value={token}
          onChange={(e) => setToken(e.target.value)}
          required
        />
        <Button type="submit" disabled={addBusy} className="shrink-0">
          {addBusy ? "Verifying..." : "Add domain"}
        </Button>
      </form>
      {err && <p className="text-sm text-destructive mt-2">{err}</p>}
    </div>
  );
}
