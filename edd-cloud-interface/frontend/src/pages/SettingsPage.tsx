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

      {/* Bare-views switcher */}
      <div className="flex gap-5 mb-6">
        {TABS.map(({ key, label }) => (
          <button
            key={key}
            onClick={() => setTab(key)}
            className={cn(
              "font-mono text-[11.5px] uppercase tracking-[0.16em] transition-colors duration-150",
              tab === key
                ? "text-foreground"
                : "text-faint hover:text-foreground"
            )}
          >
            {tab === key && <span className="text-primary">› </span>}
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
