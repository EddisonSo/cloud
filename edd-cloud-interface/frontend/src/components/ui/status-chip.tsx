import { cn } from "@/lib/utils";

const statusStyles: Record<string, { text: string; bg: string; dot: string }> = {
  running:      { text: "text-success",       bg: "bg-success/10",      dot: "bg-success" },
  healthy:      { text: "text-success",       bg: "bg-success/10",      dot: "bg-success" },
  ok:           { text: "text-success",       bg: "bg-success/10",      dot: "bg-success" },
  stopped:      { text: "text-muted-foreground", bg: "bg-muted",        dot: "bg-muted-foreground" },
  pending:      { text: "text-warning",       bg: "bg-warning/10",      dot: "bg-warning animate-pulse-slow" },
  initializing: { text: "text-warning",       bg: "bg-warning/10",      dot: "bg-warning animate-pulse-slow" },
  provisioning: { text: "text-primary",       bg: "bg-primary/10",      dot: "bg-primary animate-pulse-slow" },
  warning:      { text: "text-warning",       bg: "bg-warning/10",      dot: "bg-warning" },
  pressure:     { text: "text-warning",       bg: "bg-warning/10",      dot: "bg-warning" },
  error:        { text: "text-destructive",   bg: "bg-destructive/10",  dot: "bg-destructive" },
  down:         { text: "text-destructive",   bg: "bg-destructive/10",  dot: "bg-destructive" },
};

interface StatusChipProps {
  status: string;
  className?: string;
}

export function StatusChip({ status, className }: StatusChipProps) {
  const style = statusStyles[status?.toLowerCase()] || statusStyles.stopped;

  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 px-2 py-0.5 rounded text-xs font-medium capitalize",
        style.bg,
        style.text,
        className,
      )}
    >
      <span className={cn("w-1.5 h-1.5 rounded-full shrink-0", style.dot)} />
      {status}
    </span>
  );
}
