import { cn } from "@/lib/utils";
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
        "flex items-center justify-between px-4 py-3 bg-secondary rounded-md cursor-pointer transition-all hover:bg-secondary/80 sm:grid sm:grid-cols-[1fr_100px_100px] sm:gap-4",
        isActive && "border border-primary bg-primary/10",
        visibility < 2 && "opacity-80"
      )}
    >
      <div className="flex items-center gap-2 min-w-0">
        <FolderOpen className="w-4 h-4 text-muted-foreground shrink-0" />
        <span className="font-medium truncate">{namespace.name}</span>
      </div>
      <div className="hidden sm:block text-center text-sm text-muted-foreground">
        {namespace.count} {namespace.count === 1 ? "file" : "files"}
      </div>
      <div className="text-center">
        {visibility === 0 && (
          <span className="text-[10px] px-2 py-0.5 rounded-full bg-muted text-muted-foreground">
            Private
          </span>
        )}
        {visibility === 1 && (
          <span className="text-[10px] px-2 py-0.5 rounded-full bg-yellow-500/20 text-yellow-500">
            Unlisted
          </span>
        )}
        {visibility === 2 && (
          <span className="text-[10px] px-2 py-0.5 rounded-full bg-green-500/20 text-green-500">
            Public
          </span>
        )}
      </div>
    </div>
  );
}
