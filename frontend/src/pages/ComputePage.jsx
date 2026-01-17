import { useState, useEffect } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { Header } from "@/components/layout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import {
  ContainerList,
  CreateContainerForm,
  SshKeyList,
  ContainerDetail,
  TerminalView,
} from "@/components/compute";
import { TAB_COPY } from "@/lib/constants";
import { useContainers, useSshKeys, useContainerAccess, useTerminal } from "@/hooks";
import { useAuth } from "@/contexts/AuthContext";
import { Plus, Server, Key } from "lucide-react";

export function ComputePage() {
  const copy = TAB_COPY.compute;
  const { containerId } = useParams();
  const navigate = useNavigate();
  const { user } = useAuth();

  const {
    containers,
    setContainers,
    loading: containersLoading,
    error: containersError,
    setError: setContainersError,
    actions,
    loadContainers,
    createContainer,
    containerAction,
  } = useContainers(user);

  const {
    sshKeys,
    loading: sshKeysLoading,
    loadSshKeys,
    addSshKey,
    deleteSshKey,
  } = useSshKeys();

  const access = useContainerAccess();

  const {
    container: terminalContainer,
    connecting: terminalConnecting,
    error: terminalError,
    terminalRef,
    openTerminal,
    closeTerminal,
  } = useTerminal();

  const [view, setView] = useState("containers"); // containers, ssh-keys, detail, terminal
  const [showCreate, setShowCreate] = useState(false);
  const [selectedContainer, setSelectedContainer] = useState(null);
  const [creating, setCreating] = useState(false);

  useEffect(() => {
    if (user) {
      loadContainers();
      loadSshKeys();
    }
  }, [user, loadContainers, loadSshKeys]);

  // Handle URL param for container detail
  useEffect(() => {
    if (containerId && containers.length > 0) {
      const container = containers.find((c) => c.id === containerId);
      if (container) {
        setSelectedContainer(container);
        access.openAccess(container);
        setView("detail");
      }
    }
  }, [containerId, containers]);

  const handleCreateContainer = async (data) => {
    setCreating(true);
    try {
      await createContainer(data);
      setShowCreate(false);
    } catch (err) {
      setContainersError(err.message);
    } finally {
      setCreating(false);
    }
  };

  const handleContainerAction = async (id, action) => {
    try {
      await containerAction(id, action);
    } catch (err) {
      setContainersError(err.message);
    }
  };

  const handleSelectContainer = (container) => {
    setSelectedContainer(container);
    access.openAccess(container);
    setView("detail");
    navigate(`/compute/${container.id}`);
  };

  const handleBackToList = () => {
    setSelectedContainer(null);
    access.closeAccess();
    setView("containers");
    navigate("/compute");
  };

  const handleOpenTerminal = (container) => {
    openTerminal(container);
    setView("terminal");
  };

  const handleCloseTerminal = () => {
    closeTerminal();
    setView("detail");
  };

  const runningCount = containers.filter((c) => c.status === "running").length;

  if (!user) {
    return (
      <div>
        <Header eyebrow={copy.eyebrow} title={copy.title} description={copy.lead} />
        <p className="text-muted-foreground">Sign in to manage containers.</p>
      </div>
    );
  }

  // Terminal View
  if (view === "terminal" && terminalContainer) {
    return (
      <div>
        <Header eyebrow={copy.eyebrow} title={copy.title} description={copy.lead} />
        <TerminalView
          container={terminalContainer}
          terminalRef={terminalRef}
          connecting={terminalConnecting}
          error={terminalError}
          onClose={handleCloseTerminal}
        />
      </div>
    );
  }

  // Container Detail View
  if (view === "detail" && selectedContainer) {
    return (
      <div>
        <Header eyebrow={copy.eyebrow} title={copy.title} description={copy.lead} />
        <ContainerDetail
          container={selectedContainer}
          access={access}
          actions={actions}
          onBack={handleBackToList}
          onStart={(id) => handleContainerAction(id, "starting")}
          onStop={(id) => handleContainerAction(id, "stopping")}
          onDelete={(id) => handleContainerAction(id, "deleting")}
          onTerminal={handleOpenTerminal}
        />
      </div>
    );
  }

  return (
    <div>
      <Header eyebrow={copy.eyebrow} title={copy.title} description={copy.lead} />

      {/* Summary Cards */}
      <div className="grid grid-cols-3 gap-4 mb-6">
        <Card>
          <CardContent className="pt-6">
            <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground mb-1">
              Total Containers
            </p>
            <span className="text-2xl font-semibold">{containers.length}</span>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="pt-6">
            <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground mb-1">
              Running
            </p>
            <span className="text-2xl font-semibold text-green-400">{runningCount}</span>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="pt-6">
            <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground mb-1">
              SSH Keys
            </p>
            <span className="text-2xl font-semibold">{sshKeys.length}</span>
          </CardContent>
        </Card>
      </div>

      {/* View Toggle */}
      <div className="flex gap-2 mb-4">
        <Button
          variant={view === "containers" ? "default" : "outline"}
          onClick={() => setView("containers")}
        >
          <Server className="w-4 h-4 mr-2" />
          Containers
        </Button>
        <Button
          variant={view === "ssh-keys" ? "default" : "outline"}
          onClick={() => setView("ssh-keys")}
        >
          <Key className="w-4 h-4 mr-2" />
          SSH Keys
        </Button>
      </div>

      {containersError && (
        <p className="text-destructive text-sm mb-4">{containersError}</p>
      )}

      {/* Containers View */}
      {view === "containers" && (
        <>
          {showCreate ? (
            <CreateContainerForm
              sshKeys={sshKeys}
              onCreate={handleCreateContainer}
              onCancel={() => setShowCreate(false)}
              creating={creating}
            />
          ) : (
            <Button variant="outline" className="mb-4" onClick={() => setShowCreate(true)}>
              <Plus className="w-4 h-4 mr-2" />
              Create Container
            </Button>
          )}

          <Card>
            <CardHeader>
              <CardTitle>Containers</CardTitle>
            </CardHeader>
            <CardContent>
              <ContainerList
                containers={containers}
                actions={actions}
                onStart={(id) => handleContainerAction(id, "starting")}
                onStop={(id) => handleContainerAction(id, "stopping")}
                onDelete={(id) => handleContainerAction(id, "deleting")}
                onAccess={handleSelectContainer}
                onTerminal={handleOpenTerminal}
                onSelect={handleSelectContainer}
                loading={containersLoading}
              />
            </CardContent>
          </Card>
        </>
      )}

      {/* SSH Keys View */}
      {view === "ssh-keys" && (
        <Card>
          <CardHeader>
            <CardTitle>SSH Keys</CardTitle>
          </CardHeader>
          <CardContent>
            <SshKeyList
              sshKeys={sshKeys}
              onAdd={addSshKey}
              onDelete={deleteSshKey}
              loading={sshKeysLoading}
            />
          </CardContent>
        </Card>
      )}
    </div>
  );
}
