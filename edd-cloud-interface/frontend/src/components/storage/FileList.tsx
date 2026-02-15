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
    <div className="space-y-2">
      {/* Header - hidden on mobile */}
      <div className="hidden md:grid grid-cols-[2fr_3fr_1fr_1.5fr_100px] gap-4 px-4 py-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
        <div className="text-center">Name</div>
        <div className="text-center">Link</div>
        <div className="text-center">Size</div>
        <div className="text-center">Modified</div>
        <div className="text-center">Actions</div>
      </div>
      {/* Rows */}
      {files.map((file) => {
        const fileKey = `${file.namespace || "default"}:${file.name}`;
        const isDeleting = deleting[fileKey];
        const fileUrl = buildFileUrl(file.name);

        return (
          <div
            key={fileKey}
            className="flex items-center justify-between px-4 py-3 bg-secondary rounded-md md:grid md:grid-cols-[2fr_3fr_1fr_1.5fr_100px] md:gap-4"
          >
            <div className="min-w-0 md:text-center">
              <span className="font-medium truncate block">{file.name}</span>
            </div>
            <div className="hidden md:flex justify-center min-w-0">
              <CopyableText text={fileUrl} mono className="text-xs truncate" />
            </div>
            <div className="hidden md:block text-sm text-muted-foreground text-center">{formatBytes(file.size)}</div>
            <div className="hidden md:block text-sm text-muted-foreground text-center">{formatTimestamp(file.modified)}</div>
            <div className="flex gap-2 justify-center">
              <Button
                variant="ghost"
                size="sm"
                onClick={() => onDownload?.(file)}
                className="text-primary hover:text-primary hover:bg-primary/10"
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
