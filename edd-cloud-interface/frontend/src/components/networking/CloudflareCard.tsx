import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import type { CloudflareStatus } from "@/types";

export function CloudflareCard({
  status,
  onSave,
  onDisconnect,
}: {
  status: CloudflareStatus | null;
  onSave: (token: string) => Promise<CloudflareStatus>;
  onDisconnect: () => Promise<void>;
}) {
  const [token, setToken] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");

  const save = async (e: React.FormEvent) => {
    e.preventDefault();
    setErr("");
    setBusy(true);
    try {
      await onSave(token);
      setToken("");
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="bg-card border border-border rounded-lg p-5 mb-6">
      <h2 className="text-sm font-semibold mb-1">Cloudflare integration</h2>
      <p className="text-xs text-muted-foreground mb-4">
        Connect a Cloudflare API token (scoped to Zone:Read + DNS:Edit on your zone)
        and DNS records are created automatically when you add a domain.
      </p>
      {status?.configured ? (
        <div className="flex items-center justify-between gap-3">
          <p className="text-sm">
            {status.zones && status.zones.length > 0
              ? `Connected — zones: ${status.zones.join(", ")}`
              : "Connected (token no longer lists zones — consider reconnecting)"}
          </p>
          <Button size="sm" variant="outline" onClick={() => onDisconnect()}>
            Disconnect
          </Button>
        </div>
      ) : (
        <form onSubmit={save} className="flex flex-col sm:flex-row gap-2 max-w-xl">
          <Input
            type="password"
            placeholder="Cloudflare API token"
            value={token}
            onChange={(e) => setToken(e.target.value)}
            required
          />
          <Button type="submit" disabled={busy}>
            {busy ? "Verifying..." : "Connect"}
          </Button>
        </form>
      )}
      {err && <p className="text-sm text-destructive mt-2">{err}</p>}
    </div>
  );
}
