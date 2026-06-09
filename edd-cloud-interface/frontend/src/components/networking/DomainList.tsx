import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { EmptyState } from "@/components/ui/empty-state";
import { CopyableText } from "@/components/common";
import { Globe, RefreshCw, Trash2 } from "lucide-react";
import type { CustomDomain } from "@/types";

interface DomainListProps {
  domains: CustomDomain[];
  loading: boolean;
  onVerify: (id: string) => Promise<unknown>;
  onDelete: (id: string) => Promise<void>;
}

function StatusBadge({ status }: { status: CustomDomain["status"] }) {
  switch (status) {
    case "pending":
      return <Badge variant="warning">Pending verification</Badge>;
    case "verified":
      return <Badge variant="secondary">Verified</Badge>;
    case "active":
      return <Badge variant="success">Live</Badge>;
    case "failed":
      return <Badge variant="destructive">Failed</Badge>;
    default:
      return <Badge variant="outline">{status}</Badge>;
  }
}

export function DomainList({ domains, loading, onVerify, onDelete }: DomainListProps) {
  const [verifying, setVerifying] = useState<string | null>(null);
  const [deleting, setDeleting] = useState<string | null>(null);
  const [actionError, setActionError] = useState<Record<string, string>>({});

  const handleVerify = async (id: string) => {
    setVerifying(id);
    setActionError((prev) => ({ ...prev, [id]: "" }));
    try {
      await onVerify(id);
    } catch (err) {
      setActionError((prev) => ({ ...prev, [id]: (err as Error).message }));
    } finally {
      setVerifying(null);
    }
  };

  const handleDelete = async (id: string) => {
    setDeleting(id);
    setActionError((prev) => ({ ...prev, [id]: "" }));
    try {
      await onDelete(id);
    } catch (err) {
      setActionError((prev) => ({ ...prev, [id]: (err as Error).message }));
      setDeleting(null);
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center py-12 text-sm text-muted-foreground">
        Loading domains...
      </div>
    );
  }

  if (domains.length === 0) {
    return (
      <EmptyState
        icon={Globe}
        title="No custom domains yet"
        description="Add a domain below to point your own hostname at a container."
      />
    );
  }

  return (
    <div className="divide-y divide-border/50">
      {/* Column headers — hidden on mobile */}
      <div className="hidden md:grid md:grid-cols-[2fr_1fr_1.5fr_auto] gap-4 px-5 py-3 border-b border-border">
        <div className="text-xs font-medium text-muted-foreground uppercase tracking-wider">Domain</div>
        <div className="text-xs font-medium text-muted-foreground uppercase tracking-wider">Status</div>
        <div className="text-xs font-medium text-muted-foreground uppercase tracking-wider">Target</div>
        <div className="text-xs font-medium text-muted-foreground uppercase tracking-wider text-right">Actions</div>
      </div>

      {domains.map((domain) => (
        <div key={domain.id} className="flex flex-col md:grid md:grid-cols-[2fr_1fr_1.5fr_auto] gap-3 md:gap-4 px-5 py-4 items-start md:items-center">
          {/* Domain */}
          <div className="min-w-0">
            <span className="md:hidden text-xs text-muted-foreground block mb-0.5">Domain</span>
            <span className="font-medium text-sm break-all">{domain.domain}</span>
          </div>

          {/* Status */}
          <div>
            <span className="md:hidden text-xs text-muted-foreground block mb-0.5">Status</span>
            <StatusBadge status={domain.status} />
          </div>

          {/* Target */}
          <div className="text-sm text-muted-foreground">
            <span className="md:hidden text-xs text-muted-foreground block mb-0.5">Target</span>
            <span className="font-mono text-xs">{domain.container_id.slice(0, 8)}…:{domain.target_port}</span>
          </div>

          {/* Actions */}
          <div className="flex items-center gap-2 md:justify-end w-full md:w-auto">
            {domain.status === "pending" && (
              <Button
                size="sm"
                variant="outline"
                onClick={() => handleVerify(domain.id)}
                disabled={verifying === domain.id}
              >
                <RefreshCw className={`w-3.5 h-3.5 mr-1.5 ${verifying === domain.id ? "animate-spin" : ""}`} />
                Verify now
              </Button>
            )}
            <Button
              size="sm"
              variant="ghost"
              onClick={() => handleDelete(domain.id)}
              disabled={deleting === domain.id}
              className="text-destructive hover:text-destructive hover:bg-destructive/10"
            >
              <Trash2 className="w-3.5 h-3.5" />
              <span className="sr-only">Delete</span>
            </Button>
          </div>

          {/* Per-row action error */}
          {actionError[domain.id] && (
            <div className="md:col-span-4 text-xs text-destructive">{actionError[domain.id]}</div>
          )}

          {/* DNS setup instructions for pending domains */}
          {domain.status === "pending" && (
            <div className="md:col-span-4 w-full rounded-md bg-muted/50 border border-border px-4 py-3 space-y-2 text-xs text-muted-foreground">
              <p className="font-medium text-foreground">DNS setup required</p>
              <p>Add the following TXT record to verify ownership:</p>
              <div className="rounded bg-background border border-border px-3 py-2 font-mono space-y-0.5">
                <div>
                  <span className="text-muted-foreground">Name: </span>
                  <CopyableText text={domain.verify_name} mono />
                </div>
                <div>
                  <span className="text-muted-foreground">Value: </span>
                  <CopyableText text={domain.verify_token} mono />
                </div>
              </div>
              <p>Then point traffic to the ingress:</p>
              <div className="rounded bg-background border border-border px-3 py-2 font-mono space-y-0.5">
                <div>
                  <span className="text-muted-foreground">CNAME (subdomain): </span>
                  <CopyableText text="ingress.cloud.eddisonso.com" mono />
                </div>
                <div>
                  <span className="text-muted-foreground">A record (apex): </span>
                  <span className="text-xs">use the ingress IP from your cluster dashboard</span>
                </div>
              </div>
            </div>
          )}
        </div>
      ))}
    </div>
  );
}
