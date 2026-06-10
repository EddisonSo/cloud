import { Button } from "@/components/ui/button";
import { CopyableText } from "@/components/common";
import { formatBytes, formatTimestamp } from "@/lib/formatters";
import { Download, Trash2 } from "lucide-react";
import type { FileEntry } from "@/types";

interface FileListProps {
  files: FileEntry[];
  namespace: string;
  deleting: Record<string, boolean>;
  onDownload: (file: FileEntry) => void;
  onDelete: (file: FileEntry) => void;
  loading: boolean;
}

export function FileList({
  files,
  namespace,
  deleting,
  onDownload,
  onDelete,
  loading,
}: FileListProps) {
  if (loading) {
    return <p className="text-muted-foreground py-8 text-center">Loading files...</p>;
  }

  if (files.length === 0) {
    return (
      <p className="text-muted-foreground py-8 text-center">
        No files yet. Upload your first file to share it.
      </p>
    );
  }

  const buildFileUrl = (fileName: string): string => {
    const ns = namespace || "default";
    return `https://storage.cloud.eddisonso.com/storage/${encodeURIComponent(ns)}/${encodeURIComponent(fileName)}`;
  };

  return (
    <div className="divide-y divide-border">
      {/* Header - hidden on mobile */}
      <div className="hidden md:grid grid-cols-[2fr_3fr_1fr_1.5fr_100px] gap-4 px-4 py-2 border-b border-border">
        <div className="font-mono text-[10px] font-medium uppercase tracking-[0.2em] text-faint text-center">Name</div>
        <div className="font-mono text-[10px] font-medium uppercase tracking-[0.2em] text-faint text-center">Link</div>
        <div className="font-mono text-[10px] font-medium uppercase tracking-[0.2em] text-faint text-center">Size</div>
        <div className="font-mono text-[10px] font-medium uppercase tracking-[0.2em] text-faint text-center">Modified</div>
        <div className="font-mono text-[10px] font-medium uppercase tracking-[0.2em] text-faint text-center">Actions</div>
      </div>
      {/* Rows */}
      {files.map((file) => {
        const fileKey = `${file.namespace || "default"}:${file.name}`;
        const isDeleting = deleting[fileKey];
        const fileUrl = buildFileUrl(file.name);

        return (
          <div
            key={fileKey}
            className="flex items-center justify-between px-4 py-3 hover:bg-popover transition-colors duration-150 md:grid md:grid-cols-[2fr_3fr_1fr_1.5fr_100px] md:gap-4"
          >
            <div className="min-w-0 md:text-center">
              <span className="font-medium truncate block">{file.name}</span>
            </div>
            <div className="hidden md:flex justify-center min-w-0">
              <CopyableText text={fileUrl} mono className="text-xs truncate" />
            </div>
            <div className="hidden md:block font-mono text-[12.5px] text-muted-foreground text-center">{formatBytes(file.size)}</div>
            <div className="hidden md:block font-mono text-[12.5px] text-muted-foreground text-center">{formatTimestamp(file.modified)}</div>
            <div className="flex gap-2 justify-center">
              <Button
                variant="ghost"
                size="sm"
                onClick={() => onDownload?.(file)}
                className="text-primary hover:text-primary hover:bg-popover"
              >
                <Download className="w-4 h-4" />
              </Button>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => onDelete?.(file)}
                disabled={isDeleting}
                className="text-destructive hover:text-destructive hover:bg-destructive/10"
              >
                <Trash2 className="w-4 h-4" />
              </Button>
            </div>
          </div>
        );
      })}
    </div>
  );
}
