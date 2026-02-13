import * as React from "react";
import { cn } from "@/lib/utils";

interface SkeletonProps extends React.HTMLAttributes<HTMLDivElement> {}

function Skeleton({ className, ...props }: SkeletonProps) {
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

interface TextSkeletonProps extends React.HTMLAttributes<HTMLSpanElement> {
  text?: string;
}

function TextSkeleton({
  text = "00",
  className,
  ...props
}: TextSkeletonProps) {
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
