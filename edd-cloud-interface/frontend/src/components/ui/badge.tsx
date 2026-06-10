import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

const badgeVariants = cva(
  "inline-flex items-center border px-2 py-0.5 font-mono text-[10.5px] font-medium uppercase tracking-[0.12em] transition-colors focus:outline-none focus:ring-1 focus:ring-ring",
  {
    variants: {
      variant: {
        default: "border-primary/40 bg-transparent text-primary",
        secondary: "border-border bg-transparent text-muted-foreground",
        destructive: "border-destructive/50 bg-transparent text-destructive",
        outline: "border-border text-foreground",
        success: "border-success/40 bg-transparent text-success",
        warning: "border-warning/40 bg-transparent text-warning",
      },
    },
    defaultVariants: {
      variant: "default",
    },
  }
);

interface BadgeProps
  extends React.HTMLAttributes<HTMLDivElement>,
    VariantProps<typeof badgeVariants> {}

function Badge({ className, variant, ...props }: BadgeProps) {
  return <div className={cn(badgeVariants({ variant }), className)} {...props} />;
}

export { Badge, badgeVariants };
