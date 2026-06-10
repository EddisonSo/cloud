import * as React from "react";
import { cn } from "@/lib/utils";

interface ProgressProps extends React.HTMLAttributes<HTMLDivElement> {
  value?: number;
}

const Progress = React.forwardRef<HTMLDivElement, ProgressProps>(
  ({ className, value, ...props }, ref) => (
    // Flat track — h-1.5, no radius, no border, bg-muted
    <div
      ref={ref}
      className={cn("relative h-1.5 w-full overflow-hidden bg-muted", className)}
      {...props}
    >
      {/* Fill — ice primary, square ends, 150ms ease */}
      <div
        className="h-full bg-primary transition-all duration-150 ease-out"
        style={{ width: `${Math.max(0, Math.min(100, value ?? 0))}%` }}
      />
    </div>
  )
);
Progress.displayName = "Progress";

export { Progress };
