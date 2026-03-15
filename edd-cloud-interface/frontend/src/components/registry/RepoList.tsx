import { Package } from "lucide-react";
import { EmptyState } from "@/components/ui/empty-state";
import { formatBytes, formatTimestamp } from "@/lib/formatters";
import type { RepoInfo } from "@/hooks/useRegistry";

interface RepoListProps {
  repos: RepoInfo[];
  onSelect: (name: string) => void;
}

function VisibilityBadge({ visibility }: { visibility: number }) {
  const isPublic = visibility > 0;
  return (
    <span
      className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${
        isPublic
          ? "bg-green-500/10 text-green-600 dark:text-green-400"
          : "bg-muted text-muted-foreground"
      }`}
    >
      {isPublic ? "Public" : "Private"}
    </span>
  );
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
      <div className="hidden md:grid grid-cols-[2fr_1fr_1fr_1fr_1fr] gap-4 px-5 py-3 border-b border-border">
        <div className="text-xs font-medium text-muted-foreground uppercase tracking-wider">Name</div>
        <div className="text-xs font-medium text-muted-foreground uppercase tracking-wider">Tags</div>
        <div className="text-xs font-medium text-muted-foreground uppercase tracking-wider">Size</div>
        <div className="text-xs font-medium text-muted-foreground uppercase tracking-wider">Last Pushed</div>
        <div className="text-xs font-medium text-muted-foreground uppercase tracking-wider">Visibility</div>
      </div>
      {/* Rows */}
      <div className="divide-y divide-border/50">
        {repos.map((repo) => (
          <div
            key={repo.name}
            className="flex flex-col gap-2 px-4 md:px-5 py-3 cursor-pointer hover:bg-accent/50 transition-colors md:grid md:grid-cols-[2fr_1fr_1fr_1fr_1fr] md:gap-4 md:items-center"
            onClick={() => onSelect(repo.name)}
          >
            <div className="flex items-center justify-between md:block min-w-0">
              <span className="text-sm font-medium font-mono truncate">{repo.name}</span>
              <div className="md:hidden">
                <VisibilityBadge visibility={repo.visibility} />
              </div>
            </div>
            <div className="flex items-center gap-2 md:block">
              <span className="md:hidden text-xs text-muted-foreground">Tags:</span>
              <span className="text-sm">{repo.tag_count}</span>
            </div>
            <div className="flex items-center gap-2 md:block">
              <span className="md:hidden text-xs text-muted-foreground">Size:</span>
              <span className="text-sm">{formatBytes(repo.total_size)}</span>
            </div>
            <div className="flex items-center gap-2 md:block">
              <span className="md:hidden text-xs text-muted-foreground">Last pushed:</span>
              <span className="text-sm text-muted-foreground">
                {repo.last_pushed ? formatTimestamp(parseInt(repo.last_pushed, 10)) : "-"}
              </span>
            </div>
            <div className="hidden md:block">
              <VisibilityBadge visibility={repo.visibility} />
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
