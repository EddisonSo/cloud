import { cn } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
import { FolderOpen } from "lucide-react";
import type { Namespace, NamespaceVisibility } from "@/types";

interface NamespaceCardProps {
  namespace: Namespace;
  isActive: boolean;
  onSelect: (name: string) => void;
}

export function NamespaceCard({
  namespace,
  isActive,
  onSelect,
}: NamespaceCardProps) {
  // visibility: 0=private, 1=unlisted, 2=public
  const visibility: NamespaceVisibility = namespace.visibility ?? 2;

  return (
    <div
      onClick={() => onSelect?.(namespace.name)}
      className={cn(
        "flex items-center justify-between px-4 py-3 cursor-pointer transition-colors duration-150 hover:bg-popover sm:grid sm:grid-cols-[1fr_100px_100px] sm:gap-4",
        isActive && "border-l-2 border-primary bg-popover",
        visibility < 2 && "opacity-80"
      )}
    >
      <div className="flex items-center gap-2 min-w-0">
        <FolderOpen className="w-4 h-4 text-muted-foreground shrink-0" />
        <span className="font-medium truncate">{namespace.name}</span>
      </div>
      <div className="hidden sm:block text-center font-mono text-[12.5px] text-muted-foreground">
        {namespace.count} {namespace.count === 1 ? "file" : "files"}
      </div>
      <div className="flex sm:justify-center">
        {visibility === 0 && (
          <Badge variant="secondary">Private</Badge>
        )}
        {visibility === 1 && (
          <Badge variant="warning">Unlisted</Badge>
        )}
        {visibility === 2 && (
          <Badge variant="success">Public</Badge>
        )}
      </div>
    </div>
  );
}
