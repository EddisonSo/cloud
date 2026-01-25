import { useState, useEffect } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { Header } from "@/components/layout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Modal } from "@/components/common";
import { NamespaceCard, FileList, FileUploader } from "@/components/storage";
import { TAB_COPY } from "@/lib/constants";
import { useNamespaces, useFiles } from "@/hooks";
import { useAuth } from "@/contexts/AuthContext";
import { buildStorageBase } from "@/lib/api";
import { ArrowLeft, Plus, Settings, Eye, EyeOff, Link, Trash2, Server } from "lucide-react";

export function StoragePage() {
  const copy = TAB_COPY.storage;
  const { namespace: namespaceParam } = useParams();
  const navigate = useNavigate();
  const { user } = useAuth();
  const {
    namespaces,
    activeNamespace,
    setActiveNamespace,
    loading: namespacesLoading,
    loadNamespaces,
    createNamespace,
    deleteNamespace,
    updateNamespaceVisibility,
  } = useNamespaces();
  const {
    files,
    loading: filesLoading,
    uploading,
    uploadProgress,
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

  // Sync URL param with active namespace
  const showNamespaceView = !!namespaceParam;

  // Check if namespace from URL exists in loaded namespaces
  const namespaceNotFound = showNamespaceView && !namespacesLoading && namespaces.length > 0 &&
    !namespaces.some((ns) => ns.name === namespaceParam);

  useEffect(() => {
    if (namespaceParam) {
      setActiveNamespace(namespaceParam);
    }
  }, [namespaceParam, setActiveNamespace]);

  const [showCreateModal, setShowCreateModal] = useState(false);
  const [showSettingsModal, setShowSettingsModal] = useState(false);
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [showVisibilityModal, setShowVisibilityModal] = useState(false);
  const [namespaceInput, setNamespaceInput] = useState("");
  const [namespaceVisibility, setNamespaceVisibility] = useState(2); // 0=private, 1=unlisted, 2=public
  const [creating, setCreating] = useState(false);
  const [deletingNs, setDeletingNs] = useState(false);
  const [updatingVisibility, setUpdatingVisibility] = useState(false);
  const [namespaceError, setNamespaceError] = useState("");
  const [showOverwriteConfirm, setShowOverwriteConfirm] = useState(false);
  const [overwriteFileName, setOverwriteFileName] = useState("");
  const [storageStatus, setStorageStatus] = useState(null);

  useEffect(() => {
    loadNamespaces();
    // Fetch storage status (chunkserver count)
    fetch(`${buildStorageBase()}/storage/status`)
      .then((res) => res.json())
      .then(setStorageStatus)
      .catch(console.error);
  }, [loadNamespaces]);

  useEffect(() => {
    if (showNamespaceView && activeNamespace) {
      loadFiles(activeNamespace);
    }
  }, [showNamespaceView, activeNamespace, loadFiles]);

  const handleOpenNamespace = (name) => {
    navigate(`/storage/${encodeURIComponent(name)}`);
  };

  const handleCloseNamespace = () => {
    navigate("/storage");
  };

  const handleCreateNamespace = async () => {
    if (!namespaceInput.trim()) return;
    setCreating(true);
    setNamespaceError("");
    try {
      await createNamespace(namespaceInput.trim(), namespaceVisibility);
      setNamespaceInput("");
      setNamespaceVisibility(2);
      setShowCreateModal(false);
    } catch (err) {
      setNamespaceError(err.message);
    } finally {
      setCreating(false);
    }
  };

  const handleDeleteNamespace = async () => {
    if (!activeNamespace) return;
    setDeletingNs(true);
    try {
      await deleteNamespace(activeNamespace);
      setShowDeleteConfirm(false);
      setShowSettingsModal(false);
      handleCloseNamespace();
    } catch (err) {
      setStatus(err.message);
    } finally {
      setDeletingNs(false);
    }
  };

  const handleUpdateVisibility = async (newVisibility) => {
    if (!activeNamespace) return;
    setUpdatingVisibility(true);
    try {
      await updateNamespaceVisibility(activeNamespace, newVisibility);
      setShowVisibilityModal(false);
    } catch (err) {
      setStatus(err.message);
    } finally {
      setUpdatingVisibility(false);
    }
  };

  const currentNamespace = namespaces.find((ns) => ns.name === activeNamespace);

  const handleUpload = async (e, { overwrite = false } = {}) => {
    e?.preventDefault?.();
    const result = await uploadFile(activeNamespace, () => {
      loadFiles(activeNamespace);
      loadNamespaces();
    }, { overwrite });

    if (result?.fileExists) {
      setOverwriteFileName(result.fileName);
      setShowOverwriteConfirm(true);
    }
  };

  const handleConfirmOverwrite = async () => {
    setShowOverwriteConfirm(false);
    await handleUpload(null, { overwrite: true });
  };

  const handleDelete = async (file) => {
    await deleteFile(file, activeNamespace, () => {
      loadFiles(activeNamespace);
      loadNamespaces();
    });
  };

  return (
    <div>
      <Header eyebrow={copy.eyebrow} title={copy.title} description={copy.lead} />

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
              {namespacesLoading ? (
                <>
                  {[...Array(4)].map((_, i) => (
                    <div key={i} className="p-4 rounded-lg border border-border bg-card">
                      <Skeleton className="h-5 w-24 mb-2" />
                      <Skeleton className="h-4 w-16" />
                    </div>
                  ))}
                </>
              ) : (
                namespaces.map((ns) => (
                  <NamespaceCard
                    key={ns.name}
                    namespace={ns}
                    isActive={activeNamespace === ns.name}
                    onSelect={handleOpenNamespace}
                  />
                ))
              )}
            </div>

            {/* Chunkserver count */}
            {storageStatus && (
              <div className="mt-6 pt-4 border-t border-border flex items-center gap-2 text-sm text-muted-foreground">
                <Server className="w-4 h-4" />
                <span>{storageStatus.chunkserver_count} chunkserver{storageStatus.chunkserver_count !== 1 ? "s" : ""} online</span>
              </div>
            )}

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

          {/* Namespace Not Found */}
          {namespaceNotFound ? (
            <Card>
              <CardContent className="py-12 text-center">
                <p className="text-muted-foreground">
                  Namespace <code className="px-1.5 py-0.5 rounded bg-secondary font-mono text-sm">{namespaceParam}</code> does not exist.
                </p>
              </CardContent>
            </Card>
          ) : (
            <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
              {/* Files */}
              <Card className="lg:col-span-2">
                <CardHeader className="flex flex-row items-center justify-between space-y-0">
                  <div>
                    <div className="flex items-center gap-2">
                      <CardTitle>{activeNamespace}</CardTitle>
                      {currentNamespace?.visibility === 0 && (
                        <span className="text-[10px] px-2 py-0.5 rounded-full bg-muted text-muted-foreground">
                          Private
                        </span>
                      )}
                      {currentNamespace?.visibility === 1 && (
                        <span className="text-[10px] px-2 py-0.5 rounded-full bg-yellow-500/20 text-yellow-500">
                          Unlisted
                        </span>
                      )}
                    </div>
                    <p className="text-sm text-muted-foreground mt-1">
                      {files.length} {files.length === 1 ? "file" : "files"}
                    </p>
                  </div>
                  {user && (
                    <Button variant="ghost" size="icon" onClick={() => setShowSettingsModal(true)}>
                      <Settings className="w-4 h-4" />
                    </Button>
                  )}
                </CardHeader>
                <CardContent>
                  <FileList
                    files={files}
                    namespace={activeNamespace}
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
          )}
        </div>
      )}

      {/* Create Namespace Modal */}
      <Modal
        open={showCreateModal}
        onClose={() => {
          setShowCreateModal(false);
          setNamespaceInput("");
          setNamespaceVisibility(2);
          setNamespaceError("");
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
          <div className="space-y-2">
            <Label>Visibility</Label>
            <div className="space-y-2">
              <label
                className={`flex items-center gap-3 p-3 rounded-md cursor-pointer transition-colors ${
                  namespaceVisibility === 0 ? "bg-primary/10 border border-primary" : "bg-secondary hover:bg-secondary/80"
                }`}
              >
                <input
                  type="radio"
                  name="visibility"
                  checked={namespaceVisibility === 0}
                  onChange={() => setNamespaceVisibility(0)}
                  className="w-4 h-4 accent-primary"
                />
                <EyeOff className="w-4 h-4 text-muted-foreground" />
                <div>
                  <span className="text-sm font-medium">Private</span>
                  <p className="text-xs text-muted-foreground">Only you can see and access</p>
                </div>
              </label>
              <label
                className={`flex items-center gap-3 p-3 rounded-md cursor-pointer transition-colors ${
                  namespaceVisibility === 1 ? "bg-primary/10 border border-primary" : "bg-secondary hover:bg-secondary/80"
                }`}
              >
                <input
                  type="radio"
                  name="visibility"
                  checked={namespaceVisibility === 1}
                  onChange={() => setNamespaceVisibility(1)}
                  className="w-4 h-4 accent-primary"
                />
                <Link className="w-4 h-4 text-yellow-400" />
                <div>
                  <span className="text-sm font-medium">Unlisted</span>
                  <p className="text-xs text-muted-foreground">Not shown in list, but accessible via URL</p>
                </div>
              </label>
              <label
                className={`flex items-center gap-3 p-3 rounded-md cursor-pointer transition-colors ${
                  namespaceVisibility === 2 ? "bg-primary/10 border border-primary" : "bg-secondary hover:bg-secondary/80"
                }`}
              >
                <input
                  type="radio"
                  name="visibility"
                  checked={namespaceVisibility === 2}
                  onChange={() => setNamespaceVisibility(2)}
                  className="w-4 h-4 accent-primary"
                />
                <Eye className="w-4 h-4 text-green-400" />
                <div>
                  <span className="text-sm font-medium">Public</span>
                  <p className="text-xs text-muted-foreground">Shown in list, anyone can view</p>
                </div>
              </label>
            </div>
          </div>
          {namespaceError && <p className="text-sm text-destructive">{namespaceError}</p>}
          <div className="flex justify-end gap-2 pt-2">
            <Button variant="outline" onClick={() => setShowCreateModal(false)}>Cancel</Button>
            <Button onClick={handleCreateNamespace} disabled={!namespaceInput.trim() || creating}>
              {creating ? "Creating..." : "Create"}
            </Button>
          </div>
        </div>
      </Modal>

      {/* Namespace Settings Modal */}
      <Modal
        open={showSettingsModal}
        onClose={() => setShowSettingsModal(false)}
        title="Namespace Settings"
        description={<>Settings for <code className="px-1.5 py-0.5 rounded bg-secondary font-mono text-sm">{activeNamespace}</code></>}
      >
        <div className="space-y-4">
          <button
            onClick={() => setShowVisibilityModal(true)}
            className="w-full flex items-center gap-3 p-3 rounded-md bg-secondary hover:bg-secondary/80 transition-colors text-left"
          >
            {currentNamespace?.visibility === 0 ? (
              <EyeOff className="w-4 h-4 text-muted-foreground" />
            ) : currentNamespace?.visibility === 1 ? (
              <Link className="w-4 h-4 text-yellow-400" />
            ) : (
              <Eye className="w-4 h-4 text-green-400" />
            )}
            <div>
              <span className="text-sm font-medium">Change Visibility</span>
              <p className="text-xs text-muted-foreground">
                Currently: {currentNamespace?.visibility === 0 ? "Private" : currentNamespace?.visibility === 1 ? "Unlisted" : "Public"}
              </p>
            </div>
          </button>
          <button
            onClick={() => setShowDeleteConfirm(true)}
            className="w-full flex items-center gap-3 p-3 rounded-md bg-destructive/10 hover:bg-destructive/20 transition-colors text-left text-destructive"
          >
            <Trash2 className="w-4 h-4" />
            <div>
              <span className="text-sm font-medium">Delete Namespace</span>
              <p className="text-xs opacity-80">
                Permanently delete this namespace and all its files
              </p>
            </div>
          </button>
        </div>
      </Modal>

      {/* Change Visibility Modal */}
      <Modal
        open={showVisibilityModal}
        onClose={() => setShowVisibilityModal(false)}
        title="Change Visibility"
        description={<>Set visibility for <code className="px-1.5 py-0.5 rounded bg-secondary font-mono text-sm">{activeNamespace}</code></>}
      >
        <div className="space-y-4">
          <div className="space-y-2">
            <button
              onClick={() => handleUpdateVisibility(0)}
              disabled={updatingVisibility}
              className={`w-full flex items-center gap-3 p-3 rounded-md transition-colors text-left ${
                currentNamespace?.visibility === 0 ? "bg-primary/10 border border-primary" : "bg-secondary hover:bg-secondary/80"
              }`}
            >
              <EyeOff className="w-4 h-4 text-muted-foreground" />
              <div className="flex-1">
                <span className="text-sm font-medium">Private</span>
                <p className="text-xs text-muted-foreground">Only you can see and access</p>
              </div>
              {currentNamespace?.visibility === 0 && <span className="text-xs text-primary">Current</span>}
            </button>
            <button
              onClick={() => handleUpdateVisibility(1)}
              disabled={updatingVisibility}
              className={`w-full flex items-center gap-3 p-3 rounded-md transition-colors text-left ${
                currentNamespace?.visibility === 1 ? "bg-primary/10 border border-primary" : "bg-secondary hover:bg-secondary/80"
              }`}
            >
              <Link className="w-4 h-4 text-yellow-400" />
              <div className="flex-1">
                <span className="text-sm font-medium">Unlisted</span>
                <p className="text-xs text-muted-foreground">Not shown in list, but accessible via URL</p>
              </div>
              {currentNamespace?.visibility === 1 && <span className="text-xs text-primary">Current</span>}
            </button>
            <button
              onClick={() => handleUpdateVisibility(2)}
              disabled={updatingVisibility}
              className={`w-full flex items-center gap-3 p-3 rounded-md transition-colors text-left ${
                currentNamespace?.visibility === 2 ? "bg-primary/10 border border-primary" : "bg-secondary hover:bg-secondary/80"
              }`}
            >
              <Eye className="w-4 h-4 text-green-400" />
              <div className="flex-1">
                <span className="text-sm font-medium">Public</span>
                <p className="text-xs text-muted-foreground">Shown in list, anyone can view</p>
              </div>
              {currentNamespace?.visibility === 2 && <span className="text-xs text-primary">Current</span>}
            </button>
          </div>
          {updatingVisibility && <p className="text-sm text-muted-foreground text-center">Updating...</p>}
        </div>
      </Modal>

      {/* Delete Namespace Confirmation Modal */}
      <Modal
        open={showDeleteConfirm}
        onClose={() => setShowDeleteConfirm(false)}
        title="Delete Namespace"
        description={<>Are you sure you want to delete <code className="px-1.5 py-0.5 rounded bg-secondary font-mono text-sm">{activeNamespace}</code>?</>}
      >
        <div className="space-y-4">
          <p className="text-sm text-muted-foreground">
            This will permanently delete the namespace and all its files. This action cannot be undone.
          </p>
          <div className="flex justify-end gap-2 pt-2">
            <Button variant="outline" onClick={() => setShowDeleteConfirm(false)}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={handleDeleteNamespace} disabled={deletingNs}>
              {deletingNs ? "Deleting..." : "Delete"}
            </Button>
          </div>
        </div>
      </Modal>

      {/* Overwrite File Confirmation Modal */}
      <Modal
        open={showOverwriteConfirm}
        onClose={() => setShowOverwriteConfirm(false)}
        title="File Already Exists"
        description={<>A file named <code className="px-1.5 py-0.5 rounded bg-secondary font-mono text-sm">{overwriteFileName}</code> already exists.</>}
      >
        <div className="space-y-4">
          <p className="text-sm text-muted-foreground">
            Do you want to replace the existing file? This action cannot be undone.
          </p>
          <div className="flex justify-end gap-2 pt-2">
            <Button variant="outline" onClick={() => setShowOverwriteConfirm(false)}>
              Cancel
            </Button>
            <Button onClick={handleConfirmOverwrite}>
              Replace
            </Button>
          </div>
        </div>
      </Modal>
    </div>
  );
}
