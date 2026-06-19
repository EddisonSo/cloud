import { useState, useEffect } from "react";
import { ArrowLeft, Trash2, Package } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { CopyableText } from "@/components/common";
import { EmptyState } from "@/components/ui/empty-state";
import { formatBytes, formatTimestamp } from "@/lib/formatters";
import type { TagInfo } from "@/hooks/useRegistry";

interface RepoDetailProps {
  repoName: string;
  ownerId: string;
  currentUserId?: string;
  onBack: () => void;
  onLoadTags: (repoName: string) => Promise<TagInfo[]>;
  onDeleteTag: (repoName: string, tag: string) => Promise<void>;
}

export function RepoDetail({
  repoName,
  ownerId,
  currentUserId,
  onBack,
  onLoadTags,
  onDeleteTag,
}: RepoDetailProps) {
  const [tags, setTags] = useState<TagInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [deletingTag, setDeletingTag] = useState<string | null>(null);

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

  return (
    <div>
      {/* Header */}
      <div className="flex items-center gap-4 mb-6">
        <Button variant="outline" size="sm" onClick={onBack}>
          <ArrowLeft className="w-4 h-4 mr-2" />
          Back
        </Button>
        <h2 className="text-xl font-semibold font-mono">{repoName}</h2>
        <Badge variant="secondary">Private</Badge>
      </div>

      {/* Tags */}
      <div className="bg-card border border-border">
        <div className="px-5 py-4 border-b border-border">
          <h2 className="font-mono text-[10.5px] font-medium uppercase tracking-[0.2em] text-muted-foreground">Tags</h2>
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
              <div className="font-mono text-[10px] font-medium uppercase tracking-[0.2em] text-faint">Tag</div>
              <div className="font-mono text-[10px] font-medium uppercase tracking-[0.2em] text-faint">Digest</div>
              <div className="font-mono text-[10px] font-medium uppercase tracking-[0.2em] text-faint">Size</div>
              <div className="font-mono text-[10px] font-medium uppercase tracking-[0.2em] text-faint">Pushed</div>
              {isOwner && <div className="w-8" />}
            </div>
            <div className="divide-y divide-border">
              {tags.map((tag) => (
                <div
                  key={tag.name}
                  className="flex flex-col gap-2 px-4 md:px-5 py-3 hover:bg-popover transition-colors duration-150 md:grid md:grid-cols-[1.5fr_2fr_1fr_1fr_auto] md:gap-4 md:items-center"
                >
                  <div className="flex items-center justify-between md:block">
                    <span className="font-mono text-[12.5px] text-muted-foreground">{tag.name}</span>
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
                    <span className="font-mono text-[12.5px] text-muted-foreground">{formatBytes(tag.size)}</span>
                  </div>
                  <div className="flex items-center gap-2 md:block">
                    <span className="md:hidden text-xs text-muted-foreground">Pushed:</span>
                    <span className="font-mono text-[12.5px] text-muted-foreground">
                      {tag.pushed_at ? formatTimestamp(Math.floor(new Date(tag.pushed_at).getTime() / 1000)) : "—"}
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
