import { Button } from "@/components/ui/button";
import { StatusChip } from "@/components/ui/status-chip";
import { CopyableText } from "@/components/common";
import { Play, Square, Trash2, Settings, Terminal, Server } from "lucide-react";
import { EmptyState } from "@/components/ui/empty-state";
import type { Container, ContainerAction } from "@/types";

interface ContainerListProps {
  containers: Container[];
  actions: Record<string, ContainerAction | null>;
  onStart: (id: string) => void;
  onStop: (id: string) => void;
  onDelete: (id: string) => void;
  onAccess: (container: Container) => void;
  onTerminal: (container: Container) => void;
  onSelect: (container: Container) => void;
  loading: boolean;
}

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
}: ContainerListProps) {
  if (loading) {
    return (
      <div className="flex items-center justify-center py-12 text-sm text-muted-foreground">
        Loading containers...
      </div>
    );
  }

  if (containers.length === 0) {
    return (
      <EmptyState
        icon={Server}
        title="No containers yet"
        description="Create your first container to get started."
      />
    );
  }

  return (
    <table className="w-full">
      <thead>
        <tr className="border-b border-border">
          <th className="px-5 py-3 text-left text-xs font-medium text-muted-foreground uppercase tracking-wider">
            Name
          </th>
          <th className="px-5 py-3 text-left text-xs font-medium text-muted-foreground uppercase tracking-wider">
            Status
          </th>
          <th className="px-5 py-3 text-left text-xs font-medium text-muted-foreground uppercase tracking-wider">
            Hostname
          </th>
          <th className="px-5 py-3 text-right text-xs font-medium text-muted-foreground uppercase tracking-wider">
            Actions
          </th>
        </tr>
      </thead>
      <tbody>
        {containers.map((container) => {
          const action = actions[container.id];
          const isRunning = container.status === "running";
          const isStopped = container.status === "stopped";

          return (
            <tr
              key={container.id}
              className="border-b border-border/50 cursor-pointer hover:bg-accent/50 transition-colors"
              onClick={() => onSelect?.(container)}
            >
              <td className="px-5 py-3">
                <div className="min-w-0">
                  <span className="text-sm font-medium block truncate">{container.name}</span>
                  <span className="text-xs text-muted-foreground font-mono">{container.id.slice(0, 8)}</span>
                </div>
              </td>
              <td className="px-5 py-3">
                <StatusChip status={container.status} />
              </td>
              <td className="px-5 py-3" onClick={(e) => e.stopPropagation()}>
                {container.hostname ? (
                  <CopyableText text={container.hostname} mono className="text-sm" />
                ) : (
                  <span className="text-sm text-muted-foreground">&mdash;</span>
                )}
              </td>
              <td className="px-5 py-3" onClick={(e) => e.stopPropagation()}>
                <div className="flex gap-1 justify-end">
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
              </td>
            </tr>
          );
        })}
      </tbody>
    </table>
  );
}
