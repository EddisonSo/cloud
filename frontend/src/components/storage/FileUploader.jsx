import { Button } from "@/components/ui/button";
import { Progress } from "@/components/ui/progress";
import { progressPercent } from "@/lib/formatters";
import { Upload } from "lucide-react";

export function FileUploader({
  fileInputRef,
  selectedFileName,
  setSelectedFileName,
  uploading,
  uploadProgress,
  onUpload,
}) {
  const percent = progressPercent(uploadProgress.bytes, uploadProgress.total);

  return (
    <div className="space-y-4">
      <form onSubmit={onUpload} className="flex flex-col gap-4">
        <div className="flex items-center gap-3 p-3 bg-secondary border border-dashed border-border rounded-md">
          <input
            ref={fileInputRef}
            type="file"
            className="sr-only"
            id="file-upload"
            onChange={(e) => {
              const file = e.target.files?.[0];
              setSelectedFileName(file ? file.name : "No file selected");
            }}
          />
          <label
            htmlFor="file-upload"
            className="inline-flex items-center justify-center px-3 py-1.5 text-xs font-medium rounded-md bg-accent border border-border cursor-pointer hover:bg-primary hover:text-primary-foreground hover:border-primary transition-colors"
          >
            Choose file
          </label>
          <span className={`text-sm flex-1 truncate ${selectedFileName !== "No file selected" ? "text-foreground" : "text-muted-foreground"}`}>
            {selectedFileName}
          </span>
        </div>
        <Button type="submit" disabled={uploading || selectedFileName === "No file selected"}>
          <Upload className="w-4 h-4 mr-2" />
          {uploading ? "Uploading..." : "Upload"}
        </Button>
      </form>

      {uploadProgress.active && (
        <div className="space-y-1">
          <Progress value={percent} />
          <p className="text-xs text-muted-foreground text-center">{percent}%</p>
        </div>
      )}
    </div>
  );
}
