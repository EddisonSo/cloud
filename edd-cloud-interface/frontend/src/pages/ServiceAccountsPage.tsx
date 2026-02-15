import { useState } from "react";
import { useParams } from "react-router-dom";
import { PageHeader } from "@/components/ui/page-header";
import { Breadcrumb } from "@/components/ui/breadcrumb";
import { Button } from "@/components/ui/button";
import { ServiceAccountList } from "@/components/service-accounts/ServiceAccountList";
import { ServiceAccountDetail } from "@/components/service-accounts/ServiceAccountDetail";
import { ApiTokenList } from "@/components/settings/ApiTokenList";
import { Plus } from "lucide-react";

interface ServiceAccountsPageProps {
  view?: string;
}

export function ServiceAccountsPage({ view }: ServiceAccountsPageProps) {
  const { id } = useParams();
  const [showCreate, setShowCreate] = useState(false);

  if (view === "tokens") {
    return (
      <div>
        <Breadcrumb
          items={[
            { label: "Service Accounts", href: "/service-accounts" },
            { label: "Account Tokens" },
          ]}
        />
        <PageHeader title="Account Tokens" description="Standalone API tokens with embedded permissions for programmatic access." />
        <ApiTokenList />
      </div>
    );
  }

  if (id) {
    return (
      <div>
        <Breadcrumb
          items={[
            { label: "Service Accounts", href: "/service-accounts" },
            { label: id.slice(0, 8) },
          ]}
        />
        <ServiceAccountDetail id={id} />
      </div>
    );
  }

  return (
    <div>
      <Breadcrumb items={[{ label: "Service Accounts" }]} />
      <PageHeader
        title="Service Accounts"
        description="Manage scoped API access for automation and integrations."
        actions={
          !showCreate ? (
            <Button onClick={() => setShowCreate(true)}>
              <Plus className="w-4 h-4 mr-2" />
              Create service account
            </Button>
          ) : undefined
        }
      />
      <ServiceAccountList
        showCreate={showCreate}
        onCloseCreate={() => setShowCreate(false)}
      />
    </div>
  );
}
