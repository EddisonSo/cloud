import { useState, useEffect, useRef } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { Header } from "@/components/layout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { TextSkeleton } from "@/components/ui/skeleton";
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
import { Plus } from "lucide-react";
import type { Container, ContainerAction, CreateContainerData } from "@/types";

interface ComputePageProps {
  view?: string;
}

export function ComputePage({ view: routeView = "containers" }: ComputePageProps) {
  const copy = TAB_COPY.compute;
  const { containerId } = useParams();
  const navigate = useNavigate();
  const { user } = useAuth();

  const {
    containers,
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

  const [selectedContainer, setSelectedContainer] = useState<Container | null>(null);
  const [creating, setCreating] = useState(false);
  const [showTerminal, setShowTerminal] = useState(false);
  const accessOpenedFor = useRef<string | null>(null);

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
        if (accessOpenedFor.current !== containerId) {
          accessOpenedFor.current = containerId;
          access.openAccess(container);
        }
      }
    }
    if (!containerId) {
      accessOpenedFor.current = null;
    }
  }, [containerId, containers]);

  // Close terminal when route changes
  useEffect(() => {
    if (showTerminal) {
      handleCloseTerminal();
    }
  }, [routeView]);

  const handleCreateContainer = async (data: CreateContainerData) => {
    setCreating(true);
    try {
      await createContainer(data);
      navigate("/compute/containers");
    } catch (err) {
      setContainersError((err as Error).message);
    } finally {
      setCreating(false);
    }
  };

  const handleContainerAction = async (id: string, action: ContainerAction) => {
    try {
      await containerAction(id, action);
    } catch (err) {
      setContainersError((err as Error).message);
    }
  };

  const handleSelectContainer = (container: Container) => {
    setSelectedContainer(container);
    access.openAccess(container);
    navigate(`/compute/containers/${container.id}`);
  };

  const handleBackToList = () => {
    setSelectedContainer(null);
    access.closeAccess();
    navigate("/compute/containers");
  };

  const handleOpenTerminal = (container: Container) => {
    openTerminal(container);
    setShowTerminal(true);
  };

  const handleCloseTerminal = () => {
    closeTerminal();
    setShowTerminal(false);
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

  // Terminal View (overlay on detail)
  if (showTerminal && terminalContainer) {
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
  if (routeView === "detail" && selectedContainer) {
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

  // Create Container View
  if (routeView === "create") {
    return (
      <div>
        <Header eyebrow={copy.eyebrow} title="Create Container" description="Configure your new stateful container." />
        <CreateContainerForm
          sshKeys={sshKeys}
          onCreate={handleCreateContainer}
          onCancel={() => navigate("/compute/containers")}
          creating={creating}
        />
      </div>
    );
  }

  // SSH Keys View
  if (routeView === "ssh-keys") {
    return (
      <div>
        <Header eyebrow={copy.eyebrow} title="SSH Keys" description="Manage SSH keys for container access." />

        {/* Summary Cards */}
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 mb-6">
          <Card className="min-w-0">
            <CardContent className="pt-6">
              <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground mb-1 truncate">
                Total Keys
              </p>
              {sshKeysLoading ? (
                <TextSkeleton text="0" className="text-2xl font-semibold" />
              ) : (
                <span className="text-2xl font-semibold">{sshKeys.length}</span>
              )}
            </CardContent>
          </Card>
          <Card className="min-w-0">
            <CardContent className="pt-6">
              <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground mb-1 truncate">
                Containers
              </p>
              {containersLoading ? (
                <TextSkeleton text="0" className="text-2xl font-semibold" />
              ) : (
                <span className="text-2xl font-semibold">{containers.length}</span>
              )}
            </CardContent>
          </Card>
        </div>

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
      </div>
    );
  }

  // Containers View (default)
  return (
    <div>
      <Header eyebrow={copy.eyebrow} title={copy.title} description={copy.lead} />

      {/* Summary Cards */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 mb-6">
        <Card className="min-w-0">
          <CardContent className="pt-6">
            <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground mb-1 truncate">
              Total Containers
            </p>
            {containersLoading ? (
              <TextSkeleton text="0" className="text-2xl font-semibold" />
            ) : (
              <span className="text-2xl font-semibold">{containers.length}</span>
            )}
          </CardContent>
        </Card>
        <Card className="min-w-0">
          <CardContent className="pt-6">
            <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground mb-1 truncate">
              Running
            </p>
            {containersLoading ? (
              <TextSkeleton text="0" className="text-2xl font-semibold text-green-400" />
            ) : (
              <span className="text-2xl font-semibold text-green-400">{runningCount}</span>
            )}
          </CardContent>
        </Card>
        <Card className="min-w-0">
          <CardContent className="pt-6">
            <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground mb-1 truncate">
              SSH Keys
            </p>
            {sshKeysLoading ? (
              <TextSkeleton text="0" className="text-2xl font-semibold" />
            ) : (
              <span className="text-2xl font-semibold">{sshKeys.length}</span>
            )}
          </CardContent>
        </Card>
      </div>

      {containersError && (
        <p className="text-destructive text-sm mb-4">{containersError}</p>
      )}

      <Button variant="outline" className="mb-4" onClick={() => navigate("/compute/containers/new")}>
        <Plus className="w-4 h-4 mr-2" />
        Create Container
      </Button>

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
    </div>
  );
}
