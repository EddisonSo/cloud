import { Button } from "@/components/ui/button";
import { StatusBadge, CopyableText } from "@/components/common";
import { Play, Square, Trash2, Settings, Terminal } from "lucide-react";

export function ContainerList({
  containers,
  actions,
  onStart,
  onStop,
  onDelete,
  onAccess,
  onTerminal,
  onSelect,
  loading,
}) {
  if (loading) {
    return <p className="text-muted-foreground py-8 text-center">Loading containers...</p>;
  }

  if (containers.length === 0) {
    return (
      <p className="text-muted-foreground py-8 text-center">
        No containers yet. Create your first container to get started.
      </p>
    );
  }

  return (
    <div className="space-y-2">
      {/* Header */}
      <div className="grid grid-cols-[2fr_1fr_2fr_140px] gap-4 px-4 py-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
        <div className="text-center">Container</div>
        <div className="text-center">Status</div>
        <div className="text-center">Hostname</div>
        <div className="text-center">Actions</div>
      </div>
      {/* Rows */}
      {containers.map((container) => {
        const action = actions[container.id];
        const isRunning = container.status === "running";
        const isStopped = container.status === "stopped";

        return (
          <div
            key={container.id}
            className="grid grid-cols-[2fr_1fr_2fr_140px] gap-4 px-4 py-3 bg-secondary rounded-md items-center cursor-pointer hover:bg-secondary/80"
            onClick={() => onSelect?.(container)}
          >
            <div className="min-w-0 text-center">
              <span className="font-medium block truncate">{container.name}</span>
              <CopyableText text={container.id.slice(0, 8)} mono />
            </div>
            <div className="flex justify-center">
              <StatusBadge status={container.status} />
            </div>
            <div className="min-w-0 text-center" onClick={(e) => e.stopPropagation()}>
              {container.hostname ? (
                <CopyableText text={container.hostname} mono className="text-sm" />
              ) : (
                <span className="text-sm text-muted-foreground">—</span>
              )}
            </div>
            <div className="flex gap-1 justify-center" onClick={(e) => e.stopPropagation()}>
              {isRunning && (
                <>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-8 w-8"
                    onClick={() => onTerminal?.(container)}
                    title="Terminal"
                  >
                    <Terminal className="w-4 h-4" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-8 w-8"
                    onClick={() => onStop?.(container.id)}
                    disabled={action === "stopping"}
                    title="Stop"
                  >
                    <Square className="w-4 h-4" />
                  </Button>
                </>
              )}
              {isStopped && (
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-8 w-8"
                  onClick={() => onStart?.(container.id)}
                  disabled={action === "starting"}
                  title="Start"
                >
                  <Play className="w-4 h-4" />
                </Button>
              )}
              <Button
                variant="ghost"
                size="icon"
                className="h-8 w-8"
                onClick={() => onAccess?.(container)}
                title="Access Settings"
              >
                <Settings className="w-4 h-4" />
              </Button>
              <Button
                variant="ghost"
                size="icon"
                className="h-8 w-8 text-destructive hover:text-destructive hover:bg-destructive/10"
                onClick={() => onDelete?.(container.id)}
                disabled={action === "deleting"}
                title="Delete"
              >
                <Trash2 className="w-4 h-4" />
              </Button>
            </div>
          </div>
        );
      })}
    </div>
  );
}
