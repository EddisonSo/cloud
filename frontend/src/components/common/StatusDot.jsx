import { cn } from "@/lib/utils";

export function StatusDot({ status, className }) {
  const statusColors = {
    ok: "bg-green-500",
    running: "bg-green-500",
    success: "bg-green-500",
    down: "bg-red-500",
    stopped: "bg-red-500",
    error: "bg-red-500",
    pending: "bg-orange-500 animate-pulse-slow",
    warning: "bg-orange-500",
    initializing: "bg-blue-500 animate-pulse-slow",
    provisioning: "bg-blue-500 animate-pulse-slow",
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
