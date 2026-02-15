import { useSearchParams } from "react-router-dom";
import { Breadcrumb } from "@/components/ui/breadcrumb";
import { PageHeader } from "@/components/ui/page-header";
import { SecurityKeys } from "@/components/settings/SecurityKeys";
import { ProfileSettings } from "@/components/settings/ProfileSettings";
import { ActiveSessions } from "@/components/settings/ActiveSessions";
import { cn } from "@/lib/utils";

const TABS = [
  { key: "security", label: "Security Keys" },
  { key: "profile", label: "Profile" },
  { key: "sessions", label: "Sessions" },
] as const;

type TabKey = (typeof TABS)[number]["key"];

export function SettingsPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const tab = (searchParams.get("tab") as TabKey) || "security";

  const setTab = (key: TabKey) => {
    setSearchParams(key === "security" ? {} : { tab: key }, { replace: true });
  };

  return (
    <>
      <Breadcrumb items={[{ label: "Settings" }]} />
      <PageHeader title="Settings" description="Manage security keys, profile, and active sessions." />

      {/* Tab navigation */}
      <div className="flex gap-1 mb-6 border-b border-border">
        {TABS.map(({ key, label }) => (
          <button
            key={key}
            onClick={() => setTab(key)}
            className={cn(
              "px-4 py-2 text-sm font-medium transition-colors border-b-2 -mb-px",
              tab === key
                ? "border-primary text-primary"
                : "border-transparent text-muted-foreground hover:text-foreground"
            )}
          >
            {label}
          </button>
        ))}
      </div>

      {/* Tab content */}
      {tab === "security" && <SecurityKeys />}
      {tab === "profile" && <ProfileSettings />}
      {tab === "sessions" && <ActiveSessions />}
    </>
  );
}
