import { useState, useEffect } from "react";
import { ArrowLeft, Trash2, Package } from "lucide-react";
import { Button } from "@/components/ui/button";
import { CopyableText } from "@/components/common";
import { EmptyState } from "@/components/ui/empty-state";
import { formatBytes, formatTimestamp } from "@/lib/formatters";
import type { TagInfo } from "@/hooks/useRegistry";

interface RepoDetailProps {
  repoName: string;
  ownerId: string;
  visibility: number;
  currentUserId?: string;
  onBack: () => void;
  onLoadTags: (repoName: string) => Promise<TagInfo[]>;
  onDeleteTag: (repoName: string, tag: string) => Promise<void>;
  onSetVisibility: (repoName: string, visibility: number) => Promise<void>;
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

export function RepoDetail({
  repoName,
  ownerId,
  visibility,
  currentUserId,
  onBack,
  onLoadTags,
  onDeleteTag,
  onSetVisibility,
}: RepoDetailProps) {
  const [tags, setTags] = useState<TagInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [deletingTag, setDeletingTag] = useState<string | null>(null);
  const [togglingVisibility, setTogglingVisibility] = useState(false);

  const isOwner = currentUserId === ownerId;

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    onLoadTags(repoName).then((t) => {
      if (!cancelled) {
        setTags(t);
        setLoading(false);
      }
    });
    return () => { cancelled = true; };
  }, [repoName, onLoadTags]);

  const handleDeleteTag = async (tag: string) => {
    setDeletingTag(tag);
    try {
      await onDeleteTag(repoName, tag);
      setTags((prev) => prev.filter((t) => t.name !== tag));
    } finally {
      setDeletingTag(null);
    }
  };

  const handleToggleVisibility = async () => {
    setTogglingVisibility(true);
    try {
      await onSetVisibility(repoName, visibility > 0 ? 0 : 1);
    } finally {
      setTogglingVisibility(false);
    }
  };

  return (
    <div>
      {/* Header */}
      <div className="flex items-center gap-4 mb-6">
        <Button variant="outline" size="sm" onClick={onBack}>
          <ArrowLeft className="w-4 h-4 mr-2" />
          Back
        </Button>
        <h2 className="text-xl font-semibold font-mono">{repoName}</h2>
        <VisibilityBadge visibility={visibility} />
      </div>

      {/* Actions (owner only) */}
      {isOwner && (
        <div className="bg-card border border-border rounded-lg mb-4">
          <div className="px-5 py-4 border-b border-border">
            <h2 className="text-sm font-semibold">Repository Settings</h2>
          </div>
          <div className="p-5">
            <div className="flex items-center justify-between">
              <div>
                <span className="text-sm font-medium">Visibility</span>
                <p className="text-xs text-muted-foreground mt-0.5">
                  {visibility > 0
                    ? "This repository is public and visible to all users."
                    : "This repository is private and only visible to you."}
                </p>
              </div>
              <Button
                variant="outline"
                size="sm"
                onClick={handleToggleVisibility}
                disabled={togglingVisibility}
              >
                {togglingVisibility
                  ? "Saving..."
                  : visibility > 0
                  ? "Make Private"
                  : "Make Public"}
              </Button>
            </div>
          </div>
        </div>
      )}

      {/* Tags */}
      <div className="bg-card border border-border rounded-lg">
        <div className="px-5 py-4 border-b border-border">
          <h2 className="text-sm font-semibold">Tags</h2>
        </div>

        {loading ? (
          <div className="flex items-center justify-center py-12 text-sm text-muted-foreground">
            Loading tags...
          </div>
        ) : tags.length === 0 ? (
          <EmptyState
            icon={Package}
            title="No tags"
            description="Push a tagged image to populate this repository."
          />
        ) : (
          <div className="w-full">
            {/* Header */}
            <div className="hidden md:grid grid-cols-[1.5fr_2fr_1fr_1fr_auto] gap-4 px-5 py-3 border-b border-border">
              <div className="text-xs font-medium text-muted-foreground uppercase tracking-wider">Tag</div>
              <div className="text-xs font-medium text-muted-foreground uppercase tracking-wider">Digest</div>
              <div className="text-xs font-medium text-muted-foreground uppercase tracking-wider">Size</div>
              <div className="text-xs font-medium text-muted-foreground uppercase tracking-wider">Pushed</div>
              {isOwner && <div className="w-8" />}
            </div>
            <div className="divide-y divide-border/50">
              {tags.map((tag) => (
                <div
                  key={tag.name}
                  className="flex flex-col gap-2 px-4 md:px-5 py-3 md:grid md:grid-cols-[1.5fr_2fr_1fr_1fr_auto] md:gap-4 md:items-center"
                >
                  <div className="flex items-center justify-between md:block">
                    <span className="text-sm font-medium font-mono">{tag.name}</span>
                  </div>
                  <div className="flex items-center gap-2 md:block" onClick={(e) => e.stopPropagation()}>
                    <span className="md:hidden text-xs text-muted-foreground">Digest:</span>
                    <CopyableText
                      text={tag.digest.replace("sha256:", "sha256:").slice(0, 19) + "..."}
                      mono
                      className="text-xs"
                    />
                  </div>
                  <div className="flex items-center gap-2 md:block">
                    <span className="md:hidden text-xs text-muted-foreground">Size:</span>
                    <span className="text-sm">{formatBytes(tag.size)}</span>
                  </div>
                  <div className="flex items-center gap-2 md:block">
                    <span className="md:hidden text-xs text-muted-foreground">Pushed:</span>
                    <span className="text-sm text-muted-foreground">
                      {tag.pushed_at ? formatTimestamp(Math.floor(new Date(tag.pushed_at).getTime() / 1000)) : "-"}
                    </span>
                  </div>
                  {isOwner && (
                    <div>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-8 w-8 text-destructive hover:text-destructive hover:bg-destructive/10"
                        onClick={() => handleDeleteTag(tag.name)}
                        disabled={deletingTag === tag.name}
                        title="Delete tag"
                      >
                        <Trash2 className="w-4 h-4" />
                      </Button>
                    </div>
                  )}
                </div>
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
