import { Header } from "@/components/layout";
import { TAB_COPY } from "@/lib/constants";
import { MessageSquare } from "lucide-react";

export function MessageQueuePage() {
  const copy = TAB_COPY["message-queue"];

  return (
    <div>
      <Header eyebrow={copy.eyebrow} title={copy.title} description={copy.lead} />

      <div className="flex flex-col items-center justify-center text-center py-16">
        <div className="w-24 h-24 rounded-xl bg-secondary flex items-center justify-center mb-6">
          <MessageSquare className="w-12 h-12 opacity-50" />
        </div>
        <h3 className="text-lg font-semibold mb-2">Coming Soon</h3>
        <p className="text-sm text-muted-foreground max-w-xs">
          Message queue services are being developed. Check back soon for updates.
        </p>
      </div>
    </div>
  );
}
