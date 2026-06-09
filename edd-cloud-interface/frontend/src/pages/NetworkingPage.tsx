import { Breadcrumb } from "@/components/ui/breadcrumb";
import { PageHeader } from "@/components/ui/page-header";
import { DomainList, AddDomainForm } from "@/components/networking";
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
  } = useCustomDomains(user);

  if (!user) {
    return (
      <div>
        <Breadcrumb items={[{ label: "Networking" }]} />
        <PageHeader
          title="Networking"
          description="Point your own domains at your containers."
        />
        <p className="text-sm text-muted-foreground">Sign in to manage custom domains.</p>
      </div>
    );
  }

  return (
    <div>
      <Breadcrumb items={[{ label: "Networking" }]} />
      <PageHeader
        title="Networking"
        description="Point your own domains at your containers."
      />

      {error && (
        <div className="bg-destructive/10 border border-destructive/20 rounded-lg px-4 py-3 mb-4">
          <p className="text-sm text-destructive">{error}</p>
        </div>
      )}

      {/* Add domain */}
      <div className="bg-card border border-border rounded-lg mb-6">
        <div className="px-5 py-4 border-b border-border">
          <h2 className="text-sm font-semibold">Add custom domain</h2>
          <p className="text-xs text-muted-foreground mt-0.5">
            Map a hostname you own to one of your containers.
          </p>
        </div>
        <div className="p-5">
          <AddDomainForm onAdd={createDomain} />
        </div>
      </div>

      {/* Domain list */}
      <div className="bg-card border border-border rounded-lg">
        <div className="px-5 py-4 border-b border-border">
          <h2 className="text-sm font-semibold">Custom domains</h2>
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
