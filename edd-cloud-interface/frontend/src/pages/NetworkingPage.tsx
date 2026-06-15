import { Breadcrumb } from "@/components/ui/breadcrumb";
import { PageHeader } from "@/components/ui/page-header";
import { DomainList, AddDomainForm, CloudflareCard } from "@/components/networking";
import { useCustomDomains } from "@/hooks";
import { useAuth } from "@/contexts/AuthContext";

export function NetworkingPage() {
  const { user } = useAuth();
  const {
    domains,
    loading,
    error,
    createDomain,
    verifyDomain,
    deleteDomain,
    connections,
    addConnection,
    removeConnection,
    refreshConnection,
  } = useCustomDomains(user);

  if (!user) {
    return (
      <div>
        <Breadcrumb items={[{ label: "Networking" }, { label: "Domains" }]} />
        <PageHeader
          title="Domains"
          description="Manage the domains you own and map their hostnames to your containers."
        />
        <p className="text-sm text-muted-foreground">Sign in to manage domains and mappings.</p>
      </div>
    );
  }

  return (
    <div>
      <Breadcrumb items={[{ label: "Networking" }, { label: "Domains" }]} />
      <PageHeader
        title="Domains"
        description="Manage the domains you own and map their hostnames to your containers."
      />

      {error && (
        <div className="border border-destructive px-4 py-3 mb-4">
          <p className="text-sm text-destructive">{error}</p>
        </div>
      )}

      {/* Domains — owned zones (added via a Cloudflare API token) */}
      <CloudflareCard
        connections={connections}
        onAdd={addConnection}
        onRemove={removeConnection}
        onRefresh={refreshConnection}
      />

      {/* Add domain mapping */}
      <div className="bg-card border border-border mb-6">
        <div className="px-5 py-4 border-b border-border">
          <h2 className="font-mono text-[10.5px] font-medium uppercase tracking-[0.2em] text-faint">Add domain mapping</h2>
          <p className="text-xs text-muted-foreground mt-1">
            Map a hostname you own to one of your containers.
          </p>
        </div>
        <div className="p-5">
          <AddDomainForm onCreate={createDomain} />
        </div>
      </div>

      {/* Domain mappings list */}
      <div className="bg-card border border-border">
        <div className="px-5 py-4 border-b border-border">
          <h2 className="font-mono text-[10.5px] font-medium uppercase tracking-[0.2em] text-faint">Domain mappings</h2>
        </div>
        <DomainList
          domains={domains}
          loading={loading}
          onVerify={verifyDomain}
          onDelete={deleteDomain}
        />
      </div>
    </div>
  );
}
