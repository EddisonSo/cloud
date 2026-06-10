import { cn } from "@/lib/utils";

/*
 * Status = dot + mono uppercase label. Never color alone; never pill washes.
 * Live states glow faintly; idle states don't.
 */
const statusStyles: Record<string, { text: string; dot: string }> = {
  running:      { text: "text-success",          dot: "bg-success shadow-[0_0_6px_rgba(62,207,110,0.55)]" },
  healthy:      { text: "text-success",          dot: "bg-success shadow-[0_0_6px_rgba(62,207,110,0.55)]" },
  ok:           { text: "text-success",          dot: "bg-success shadow-[0_0_6px_rgba(62,207,110,0.55)]" },
  stopped:      { text: "text-muted-foreground", dot: "bg-faint" },
  pending:      { text: "text-warning",          dot: "bg-warning animate-pulse-slow shadow-[0_0_6px_rgba(232,180,58,0.5)]" },
  initializing: { text: "text-warning",          dot: "bg-warning animate-pulse-slow shadow-[0_0_6px_rgba(232,180,58,0.5)]" },
  provisioning: { text: "text-primary",          dot: "bg-primary animate-pulse-slow shadow-[0_0_6px_rgba(183,217,242,0.5)]" },
  warning:      { text: "text-warning",          dot: "bg-warning shadow-[0_0_6px_rgba(232,180,58,0.5)]" },
  pressure:     { text: "text-warning",          dot: "bg-warning shadow-[0_0_6px_rgba(232,180,58,0.5)]" },
  error:        { text: "text-destructive",      dot: "bg-destructive shadow-[0_0_6px_rgba(229,84,75,0.5)]" },
  down:         { text: "text-destructive",      dot: "bg-destructive shadow-[0_0_6px_rgba(229,84,75,0.5)]" },
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
        "inline-flex items-center gap-2 font-mono text-[11px] font-medium uppercase tracking-[0.14em]",
        style.text,
        className,
      )}
    >
      <span className={cn("w-[7px] h-[7px] rounded-full shrink-0", style.dot)} />
      {status}
    </span>
  );
}
