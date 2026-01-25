import { cn } from "@/lib/utils";
import { FolderOpen } from "lucide-react";

export function NamespaceCard({
  namespace,
  isActive,
  onSelect,
}) {
  // visibility: 0=private, 1=unlisted, 2=public
  const visibility = namespace.visibility ?? 2;

  return (
    <div
      onClick={() => onSelect?.(namespace.name)}
      className={cn(
        "flex flex-col gap-3 p-4 rounded-lg border cursor-pointer transition-all",
        "bg-secondary hover:border-primary",
        isActive && "border-primary bg-primary/10",
        visibility < 2 && "opacity-80"
      )}
    >
      <div className="flex items-start justify-between">
        <div className="flex items-center gap-2">
          <FolderOpen className="w-4 h-4 text-muted-foreground" />
          <h3 className="font-semibold text-sm">{namespace.name}</h3>
        </div>
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
      </div>
      <p className="text-xs text-muted-foreground">
        {namespace.count} {namespace.count === 1 ? "file" : "files"}
      </p>
    </div>
  );
}
