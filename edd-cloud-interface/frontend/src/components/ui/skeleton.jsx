import { cn } from "@/lib/utils";

function Skeleton({ className, ...props }) {
  return (
    <div
      className={cn(
        "animate-pulse rounded-md bg-muted/50",
        className
      )}
      {...props}
    />
  );
}

function TextSkeleton({
  text = "00",
  className,
  ...props
}) {
  return (
    <span
      className={cn(
        "inline-block select-none blur-[6px] opacity-60 animate-pulse",
        className
      )}
      aria-hidden="true"
      {...props}
    >
      {text}
    </span>
  );
}

export { Skeleton, TextSkeleton };
