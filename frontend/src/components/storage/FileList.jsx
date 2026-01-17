import { Button } from "@/components/ui/button";
import { formatBytes, formatTimestamp } from "@/lib/formatters";
import { Download, Trash2 } from "lucide-react";

export function FileList({
  files,
  deleting,
  onDownload,
  onDelete,
  loading,
}) {
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

  return (
    <div className="space-y-2">
      {/* Header */}
      <div className="grid grid-cols-[1fr_100px_120px_auto] gap-4 px-4 py-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
        <div>Name</div>
        <div>Size</div>
        <div>Modified</div>
        <div className="text-right">Actions</div>
      </div>
      {/* Rows */}
      {files.map((file) => {
        const fileKey = `${file.namespace || "default"}:${file.name}`;
        const isDeleting = deleting[fileKey];

        return (
          <div
            key={fileKey}
            className="grid grid-cols-[1fr_100px_120px_auto] gap-4 px-4 py-3 bg-secondary rounded-md items-center"
          >
            <div className="min-w-0">
              <span className="font-medium truncate block">{file.name}</span>
            </div>
            <div className="text-sm text-muted-foreground">{formatBytes(file.size)}</div>
            <div className="text-sm text-muted-foreground">{formatTimestamp(file.modified)}</div>
            <div className="flex gap-2 justify-end">
              <Button
                variant="ghost"
                size="sm"
                onClick={() => onDownload?.(file)}
                className="text-primary hover:text-primary"
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
