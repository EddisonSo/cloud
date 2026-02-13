import { cn } from "@/lib/utils";

const statusConfig: Record<string, { color: string; dot: string }> = {
  running: { color: "bg-green-500/20 text-green-400", dot: "bg-green-500" },
  stopped: { color: "bg-muted text-muted-foreground", dot: "bg-muted-foreground" },
  pending: { color: "bg-orange-500/20 text-orange-400", dot: "bg-orange-500 animate-pulse-slow" },
  initializing: { color: "bg-orange-500/20 text-orange-400", dot: "bg-orange-500 animate-pulse-slow" },
  provisioning: { color: "bg-blue-500/20 text-blue-400", dot: "bg-blue-500 animate-pulse-slow" },
  error: { color: "bg-red-500/20 text-red-400", dot: "bg-red-500" },
  ok: { color: "bg-green-500/20 text-green-400", dot: "bg-green-500" },
  down: { color: "bg-red-500/20 text-red-400", dot: "bg-red-500" },
};

interface StatusBadgeProps {
  status: string;
  className?: string;
}

export function StatusBadge({ status, className }: StatusBadgeProps) {
  const config = statusConfig[status?.toLowerCase()] || statusConfig.stopped;

  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium capitalize",
        config.color,
        className
      )}
    >
      <span className={cn("w-2 h-2 rounded-full", config.dot)} />
      {status}
    </span>
  );
}
