import { cn } from "@/lib/utils";

interface StatusDotProps {
  status: string;
  className?: string;
}

export function StatusDot({ status, className }: StatusDotProps) {
  const statusColors: Record<string, string> = {
    ok: "bg-success",
    running: "bg-success",
    success: "bg-success",
    down: "bg-destructive",
    stopped: "bg-destructive",
    error: "bg-destructive",
    pending: "bg-warning animate-pulse-slow",
    warning: "bg-warning",
    initializing: "bg-primary animate-pulse-slow",
    provisioning: "bg-primary animate-pulse-slow",
  };

  return (
    <span
      className={cn(
        "w-2 h-2 rounded-full flex-shrink-0",
        statusColors[status?.toLowerCase()] || "bg-muted-foreground",
        className
      )}
    />
  );
}
