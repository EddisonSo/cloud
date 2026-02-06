import { Header } from "@/components/layout";
import { ApiTokenList } from "@/components/settings/ApiTokenList";

export function SettingsPage() {
  return (
    <>
      <Header
        eyebrow="Auth"
        title="Legacy API Tokens"
        description="Standalone API tokens with embedded permissions. New tokens should use Service Accounts."
      />
      <ApiTokenList />
    </>
  );
}
