import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Eye, EyeOff, Trash2, FolderOpen } from "lucide-react";

export function NamespaceCard({
  namespace,
  isActive,
  onSelect,
  onToggleHidden,
  onDelete,
  showActions = true,
}) {
  return (
    <div
      onClick={() => onSelect?.(namespace.name)}
      className={cn(
        "flex flex-col gap-3 p-4 rounded-lg border cursor-pointer transition-all",
        "bg-secondary hover:border-primary",
        isActive && "border-primary bg-primary/10",
        namespace.hidden && "opacity-60"
      )}
    >
      <div className="flex items-start justify-between">
        <div className="flex items-center gap-2">
          <FolderOpen className="w-4 h-4 text-muted-foreground" />
          <h3 className="font-semibold text-sm">{namespace.name}</h3>
        </div>
        {namespace.hidden && (
          <span className="text-[10px] px-2 py-0.5 rounded-full bg-muted text-muted-foreground">
            Hidden
          </span>
        )}
      </div>
      <p className="text-xs text-muted-foreground">
        {namespace.count} {namespace.count === 1 ? "file" : "files"}
      </p>
      {showActions && (
        <div className="flex gap-2 mt-auto" onClick={(e) => e.stopPropagation()}>
          <Button
            variant="ghost"
            size="sm"
            className="h-7 px-2"
            onClick={() => onToggleHidden?.(namespace.name, !namespace.hidden)}
            title={namespace.hidden ? "Show namespace" : "Hide namespace"}
          >
            {namespace.hidden ? <Eye className="w-3 h-3" /> : <EyeOff className="w-3 h-3" />}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            className="h-7 px-2 text-destructive hover:text-destructive hover:bg-destructive/10"
            onClick={() => onDelete?.(namespace.name)}
          >
            <Trash2 className="w-3 h-3" />
          </Button>
        </div>
      )}
    </div>
  );
}
