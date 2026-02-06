import { Header } from "@/components/layout";
import { ApiTokenList } from "@/components/settings/ApiTokenList";
import { TAB_COPY } from "@/lib/constants";

export function SettingsPage() {
  const copy = TAB_COPY.settings;

  return (
    <>
      <Header eyebrow={copy.eyebrow} title={copy.title} description={copy.lead} />
      <ApiTokenList />
    </>
  );
}
