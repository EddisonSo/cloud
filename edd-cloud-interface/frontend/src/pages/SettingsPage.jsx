import { Header } from "@/components/layout";
import { ApiTokenList } from "@/components/settings/ApiTokenList";
import { NotificationMutes } from "@/components/settings/NotificationMutes";

export function SettingsPage() {
  return (
    <>
      <Header
        eyebrow="Preferences"
        title="Settings"
        description="Manage notification preferences and legacy API tokens."
      />
      <div className="space-y-8">
        <NotificationMutes />
        <ApiTokenList />
      </div>
    </>
  );
}
