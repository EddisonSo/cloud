import { useState, useEffect } from "react";
import { Header } from "@/components/layout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Modal } from "@/components/common";
import { NamespaceCard, FileList, FileUploader } from "@/components/storage";
import { TAB_COPY } from "@/lib/constants";
import { useNamespaces, useFiles } from "@/hooks";
import { useAuth } from "@/contexts/AuthContext";
import { ArrowLeft, Plus } from "lucide-react";

export function StoragePage() {
  const copy = TAB_COPY.storage;
  const { user } = useAuth();
  const {
    namespaces,
    activeNamespace,
    setActiveNamespace,
    loadNamespaces,
    createNamespace,
    deleteNamespace,
    toggleNamespaceHidden,
  } = useNamespaces();
  const {
    files,
    loading: filesLoading,
    uploading,
    uploadProgress,
    downloadProgress,
    deleting,
    status,
    setStatus,
    fileInputRef,
    selectedFileName,
    setSelectedFileName,
    loadFiles,
    uploadFile,
    downloadFile,
    deleteFile,
  } = useFiles();

  const [showNamespaceView, setShowNamespaceView] = useState(false);
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [namespaceInput, setNamespaceInput] = useState("");
  const [namespaceHidden, setNamespaceHidden] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState(null);
  const [creating, setCreating] = useState(false);

  useEffect(() => {
    loadNamespaces();
  }, [loadNamespaces]);

  useEffect(() => {
    if (showNamespaceView && activeNamespace) {
      loadFiles(activeNamespace);
    }
  }, [showNamespaceView, activeNamespace, loadFiles]);

  const handleOpenNamespace = (name) => {
    setActiveNamespace(name);
    setShowNamespaceView(true);
  };

  const handleCloseNamespace = () => {
    setShowNamespaceView(false);
    setActiveNamespace("");
  };

  const handleCreateNamespace = async () => {
    if (!namespaceInput.trim()) return;
    setCreating(true);
    try {
      await createNamespace(namespaceInput.trim(), namespaceHidden);
      setNamespaceInput("");
      setNamespaceHidden(false);
      setShowCreateModal(false);
    } catch (err) {
      setStatus(err.message);
    } finally {
      setCreating(false);
    }
  };

  const handleDeleteNamespace = async () => {
    if (!deleteTarget) return;
    try {
      await deleteNamespace(deleteTarget);
      setDeleteTarget(null);
    } catch (err) {
      setStatus(err.message);
    }
  };

  const handleUpload = async (e) => {
    e.preventDefault();
    await uploadFile(activeNamespace, () => {
      loadFiles(activeNamespace);
      loadNamespaces();
    });
  };

  const handleDelete = async (file) => {
    await deleteFile(file, activeNamespace, () => {
      loadFiles(activeNamespace);
      loadNamespaces();
    });
  };

  const totalFiles = namespaces.reduce((sum, ns) => sum + (ns.count || 0), 0);

  return (
    <div>
      <Header eyebrow={copy.eyebrow} title={copy.title} description={copy.lead} />

      {/* Summary Cards */}
      <div className="grid grid-cols-3 gap-4 mb-6">
        <Card>
          <CardContent className="pt-6">
            <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground mb-1">
              Total Files
            </p>
            <span className="text-2xl font-semibold">{totalFiles}</span>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="pt-6">
            <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground mb-1">
              Namespaces
            </p>
            <span className="text-2xl font-semibold">{namespaces.length}</span>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="pt-6">
            <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground mb-1">
              Active
            </p>
            <span className="text-2xl font-semibold">{activeNamespace || "—"}</span>
            <span className="text-xs text-muted-foreground block mt-1">current namespace</span>
          </CardContent>
        </Card>
      </div>

      {/* Namespace List View */}
      {!showNamespaceView && (
        <Card>
          <CardHeader>
            <CardTitle>Namespaces</CardTitle>
            <p className="text-sm text-muted-foreground">
              Choose a namespace to view its files and activity.
            </p>
          </CardHeader>
          <CardContent>
            {/* Create Namespace Button */}
            {user && (
              <Button variant="outline" className="mb-6" onClick={() => setShowCreateModal(true)}>
                <Plus className="w-4 h-4 mr-2" />
                Create Namespace
              </Button>
            )}

            {/* Namespace Grid */}
            <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-3">
              {namespaces.map((ns) => (
                <NamespaceCard
                  key={ns.name}
                  namespace={ns}
                  isActive={activeNamespace === ns.name}
                  onSelect={handleOpenNamespace}
                  onToggleHidden={toggleNamespaceHidden}
                  onDelete={setDeleteTarget}
                  showActions={!!user}
                />
              ))}
            </div>

            {status && <p className="text-sm text-muted-foreground mt-4">{status}</p>}
          </CardContent>
        </Card>
      )}

      {/* Namespace Detail View */}
      {showNamespaceView && (
        <div>
          <Button variant="ghost" className="mb-4" onClick={handleCloseNamespace}>
            <ArrowLeft className="w-4 h-4 mr-2" />
            Back to namespaces
          </Button>

          <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
            {/* Files */}
            <Card className="lg:col-span-2">
              <CardHeader className="flex flex-row items-center justify-between space-y-0">
                <div>
                  <CardTitle>{activeNamespace}</CardTitle>
                  <p className="text-sm text-muted-foreground mt-1">
                    {files.length} {files.length === 1 ? "file" : "files"}
                  </p>
                </div>
              </CardHeader>
              <CardContent>
                <FileList
                  files={files}
                  downloadProgress={downloadProgress}
                  deleting={deleting}
                  onDownload={(file) => downloadFile(file, user)}
                  onDelete={handleDelete}
                  loading={filesLoading}
                />
              </CardContent>
            </Card>

            {/* Upload */}
            {user && (
              <Card>
                <CardHeader>
                  <CardTitle>Upload File</CardTitle>
                </CardHeader>
                <CardContent>
                  <FileUploader
                    fileInputRef={fileInputRef}
                    selectedFileName={selectedFileName}
                    setSelectedFileName={setSelectedFileName}
                    uploading={uploading}
                    uploadProgress={uploadProgress}
                    onUpload={handleUpload}
                  />
                  {status && <p className="text-sm text-muted-foreground mt-4">{status}</p>}
                </CardContent>
              </Card>
            )}
          </div>
        </div>
      )}

      {/* Create Namespace Modal */}
      <Modal
        open={showCreateModal}
        onClose={() => {
          setShowCreateModal(false);
          setNamespaceInput("");
          setNamespaceHidden(false);
        }}
        title="Create Namespace"
        description="Create a new namespace to organize your files."
      >
        <div className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="ns-name">Name</Label>
            <Input
              id="ns-name"
              placeholder="eg. team-alpha"
              value={namespaceInput}
              onChange={(e) => setNamespaceInput(e.target.value)}
              autoFocus
            />
          </div>
          <label className="flex items-center gap-3 p-3 rounded-md bg-secondary cursor-pointer">
            <input
              type="checkbox"
              checked={namespaceHidden}
              onChange={(e) => setNamespaceHidden(e.target.checked)}
              className="w-4 h-4 accent-primary"
            />
            <div>
              <span className="text-sm font-medium">Hidden</span>
              <p className="text-xs text-muted-foreground">Hidden namespaces are not visible to guests</p>
            </div>
          </label>
          {status && <p className="text-sm text-destructive">{status}</p>}
          <div className="flex justify-end gap-2 pt-2">
            <Button variant="outline" onClick={() => setShowCreateModal(false)}>Cancel</Button>
            <Button onClick={handleCreateNamespace} disabled={!namespaceInput.trim() || creating}>
              {creating ? "Creating..." : "Create"}
            </Button>
          </div>
        </div>
      </Modal>

      {/* Delete Confirmation Modal */}
      <Modal
        open={!!deleteTarget}
        onClose={() => setDeleteTarget(null)}
        title="Delete Namespace"
        description={<>Are you sure you want to delete <code className="px-1.5 py-0.5 rounded bg-secondary font-mono text-sm">{deleteTarget}</code>? This will delete all files in this namespace.</>}
      >
        <div className="flex justify-end gap-2 mt-4">
          <Button variant="outline" onClick={() => setDeleteTarget(null)}>Cancel</Button>
          <Button variant="destructive" onClick={handleDeleteNamespace}>Delete</Button>
        </div>
      </Modal>
    </div>
  );
}
