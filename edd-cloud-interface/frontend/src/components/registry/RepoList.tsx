import { Package } from "lucide-react";
import { EmptyState } from "@/components/ui/empty-state";
import { formatBytes, formatTimestamp } from "@/lib/formatters";
import type { RepoInfo } from "@/hooks/useRegistry";

interface RepoListProps {
  repos: RepoInfo[];
  onSelect: (name: string) => void;
}

export function RepoList({ repos, onSelect }: RepoListProps) {
  if (repos.length === 0) {
    return (
      <EmptyState
        icon={Package}
        title="No repositories"
        description="Push an image to create a repository."
      />
    );
  }

  return (
    <div className="w-full">
      {/* Header - hidden on mobile */}
      <div className="hidden md:grid grid-cols-[2fr_1fr_1fr_1fr] gap-4 px-5 py-3 border-b border-border">
        <div className="font-mono text-[10px] font-medium uppercase tracking-[0.2em] text-faint">Name</div>
        <div className="font-mono text-[10px] font-medium uppercase tracking-[0.2em] text-faint">Tags</div>
        <div className="font-mono text-[10px] font-medium uppercase tracking-[0.2em] text-faint">Size</div>
        <div className="font-mono text-[10px] font-medium uppercase tracking-[0.2em] text-faint">Last Pushed</div>
      </div>
      {/* Rows */}
      <div className="divide-y divide-border">
        {repos.map((repo) => (
          <div
            key={repo.name}
            className="flex flex-col gap-2 px-4 md:px-5 py-3 cursor-pointer hover:bg-popover transition-colors duration-150 md:grid md:grid-cols-[2fr_1fr_1fr_1fr] md:gap-4 md:items-center"
            onClick={() => onSelect(repo.name)}
          >
            <div className="min-w-0">
              <span className="font-mono text-[12.5px] text-muted-foreground truncate">{repo.name}</span>
            </div>
            <div className="flex items-center gap-2 md:block">
              <span className="md:hidden text-xs text-muted-foreground">Tags:</span>
              <span className="font-mono text-[12.5px] text-muted-foreground">{repo.tag_count}</span>
            </div>
            <div className="flex items-center gap-2 md:block">
              <span className="md:hidden text-xs text-muted-foreground">Size:</span>
              <span className="font-mono text-[12.5px] text-muted-foreground">{formatBytes(repo.total_size)}</span>
            </div>
            <div className="flex items-center gap-2 md:block">
              <span className="md:hidden text-xs text-muted-foreground">Last pushed:</span>
              <span className="font-mono text-[12.5px] text-muted-foreground">
                {repo.last_pushed ? formatTimestamp(Math.floor(new Date(repo.last_pushed).getTime() / 1000)) : "—"}
              </span>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
