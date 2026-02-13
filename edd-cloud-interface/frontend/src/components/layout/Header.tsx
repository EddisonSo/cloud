import React from "react";
import { cn } from "@/lib/utils";

interface HeaderProps {
  eyebrow?: string;
  title: string;
  description?: string;
  actions?: React.ReactNode;
  className?: string;
}

export function Header({ eyebrow, title, description, actions, className }: HeaderProps) {
  return (
    <header className={cn("flex items-start justify-between pb-6 mb-6 border-b border-border", className)}>
      <div className="flex-1">
        {eyebrow && (
          <p className="text-xs font-semibold uppercase tracking-wider text-primary mb-2">
            {eyebrow}
          </p>
        )}
        <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
        {description && (
          <p className="text-sm text-muted-foreground mt-1 max-w-lg">{description}</p>
        )}
      </div>
      {actions && <div className="flex gap-2">{actions}</div>}
    </header>
  );
}
