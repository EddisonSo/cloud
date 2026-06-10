import { Breadcrumb } from "@/components/ui/breadcrumb";
import { PageHeader } from "@/components/ui/page-header";
import { Database } from "lucide-react";

export function DatastorePage() {
  return (
    <div>
      <Breadcrumb items={[{ label: "Datastore" }]} />
      <PageHeader title="Datastore" description="Datastore provisioning is coming soon with managed database workflows." />

      <div className="flex flex-col items-center justify-center text-center py-16">
        <div className="w-24 h-24 border border-border bg-card flex items-center justify-center mb-6">
          <Database className="w-12 h-12 opacity-40" />
        </div>
        <h3 className="text-[14px] font-semibold mb-2">Coming Soon</h3>
        <p className="text-sm text-muted-foreground max-w-xs">
          Datastore provisioning is coming soon with managed database workflows.
        </p>
      </div>
    </div>
  );
}
