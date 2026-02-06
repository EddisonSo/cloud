import { useParams } from "react-router-dom";
import { Header } from "@/components/layout";
import { ServiceAccountList } from "@/components/service-accounts/ServiceAccountList";
import { ServiceAccountDetail } from "@/components/service-accounts/ServiceAccountDetail";
import { TAB_COPY } from "@/lib/constants";

export function ServiceAccountsPage() {
  const { id } = useParams();
  const copy = TAB_COPY["service-accounts"];

  return (
    <>
      <Header eyebrow={copy.eyebrow} title={copy.title} description={copy.lead} />
      {id ? <ServiceAccountDetail id={id} /> : <ServiceAccountList />}
    </>
  );
}
