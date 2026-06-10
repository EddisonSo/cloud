import * as React from "react";
import { cn } from "@/lib/utils";

interface SwitchProps
  extends Omit<React.ButtonHTMLAttributes<HTMLButtonElement>, "onChange"> {
  checked?: boolean;
  onCheckedChange?: (checked: boolean) => void;
}

const Switch = React.forwardRef<HTMLButtonElement, SwitchProps>(
  ({ className, checked, onCheckedChange, disabled, ...props }, ref) => {
    return (
      <button
        type="button"
        role="switch"
        aria-checked={checked}
        disabled={disabled}
        // Machined track: 2px radius max, hairline border, square-ish
        className={cn(
          "peer inline-flex h-5 w-9 shrink-0 cursor-pointer items-center border transition-colors duration-150",
          "focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring focus-visible:ring-offset-1 focus-visible:ring-offset-background",
          "disabled:cursor-not-allowed disabled:opacity-40",
          checked ? "bg-primary border-primary" : "bg-muted border-border",
          className
        )}
        style={{ borderRadius: "2px" }}
        onClick={() => onCheckedChange?.(!checked)}
        ref={ref}
        {...props}
      >
        {/* Square thumb */}
        <span
          className={cn(
            "pointer-events-none block h-3.5 w-3.5 shrink-0 transition-transform duration-150",
            checked ? "translate-x-[18px] bg-primary-foreground" : "translate-x-[2px] bg-foreground"
          )}
          style={{ borderRadius: "1px" }}
        />
      </button>
    );
  }
);
Switch.displayName = "Switch";

export { Switch };
