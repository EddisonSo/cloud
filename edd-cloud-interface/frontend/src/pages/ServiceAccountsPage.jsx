import { useParams } from "react-router-dom";
import { Header } from "@/components/layout";
import { ServiceAccountList } from "@/components/service-accounts/ServiceAccountList";
import { ServiceAccountDetail } from "@/components/service-accounts/ServiceAccountDetail";
import { ApiTokenList } from "@/components/settings/ApiTokenList";
import { TAB_COPY } from "@/lib/constants";

export function ServiceAccountsPage({ view }) {
  const { id } = useParams();
  const copy = TAB_COPY["service-accounts"];

  if (view === "tokens") {
    return (
      <>
        <Header eyebrow={copy.eyebrow} title="Account Tokens" description="Standalone API tokens with embedded permissions for programmatic access." />
        <ApiTokenList />
      </>
    );
  }

  return (
    <>
      <Header eyebrow={copy.eyebrow} title={copy.title} description={copy.lead} />
      {id ? <ServiceAccountDetail id={id} /> : <ServiceAccountList />}
    </>
  );
}
