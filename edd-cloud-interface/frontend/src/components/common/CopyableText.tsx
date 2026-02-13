import { copyToClipboard } from "@/lib/api";
import { cn } from "@/lib/utils";

interface CopyableTextProps {
  text: string;
  className?: string;
  mono?: boolean;
}

export function CopyableText({ text, className, mono = false }: CopyableTextProps) {
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
