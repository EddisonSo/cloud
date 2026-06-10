import React from "react";
import { cn } from "@/lib/utils";
import type { LucideIcon } from "lucide-react";

interface EmptyStateProps {
  icon?: LucideIcon;
  title: string;
  description?: string;
  action?: React.ReactNode;
  className?: string;
}

export function EmptyState({
  icon: Icon,
  title,
  description,
  action,
  className,
}: EmptyStateProps) {
  return (
    <div
      className={cn(
        "flex flex-col items-center justify-center py-16 text-center",
        className,
      )}
    >
      {/* Icon — bare, no pill container */}
      {Icon && (
        <Icon className="w-6 h-6 text-faint mb-4" />
      )}

      {/* Microlabel title */}
      <p className="font-mono text-[10.5px] font-medium uppercase tracking-[0.2em] text-muted-foreground mb-2">
        {title}
      </p>

      {/* Dim description body */}
      {description && (
        <p className="text-[13px] text-muted-foreground/70 max-w-sm mb-4">
          {description}
        </p>
      )}

      {action}
    </div>
  );
}
