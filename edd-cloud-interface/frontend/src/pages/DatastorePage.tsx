import { Header } from "@/components/layout";
import { TAB_COPY } from "@/lib/constants";
import { Database } from "lucide-react";

export function DatastorePage() {
  const copy = TAB_COPY.datastore;

  return (
    <div>
      <Header eyebrow={copy.eyebrow} title={copy.title} description={copy.lead} />

      <div className="flex flex-col items-center justify-center text-center py-16">
        <div className="w-24 h-24 rounded-xl bg-secondary flex items-center justify-center mb-6">
          <Database className="w-12 h-12 opacity-50" />
        </div>
        <h3 className="text-lg font-semibold mb-2">Coming Soon</h3>
        <p className="text-sm text-muted-foreground max-w-xs">
          Datastore provisioning is coming soon with managed database workflows.
        </p>
      </div>
    </div>
  );
}
