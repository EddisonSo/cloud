import { copyToClipboard } from "@/lib/api";
import { cn } from "@/lib/utils";

export function CopyableText({ text, className, mono = false }) {
  return (
    <span
      onClick={() => copyToClipboard(text)}
      className={cn(
        "cursor-pointer rounded px-1 -mx-1 transition-colors hover:bg-accent",
        mono && "font-mono text-xs text-muted-foreground",
        className
      )}
      title="Click to copy"
    >
      {text}
    </span>
  );
}
