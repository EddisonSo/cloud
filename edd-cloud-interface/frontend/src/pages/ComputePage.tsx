import { useState, useEffect, useRef } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { PageHeader } from "@/components/ui/page-header";
import { Breadcrumb } from "@/components/ui/breadcrumb";
import { Button } from "@/components/ui/button";

import {
  ContainerList,
  CreateContainerForm,
  SshKeyList,
  ContainerDetail,
  TerminalView,
} from "@/components/compute";
import { useContainers, useSshKeys, useContainerAccess, useTerminal } from "@/hooks";
import { useAuth } from "@/contexts/AuthContext";
import { Plus } from "lucide-react";
import type { Container, ContainerAction, CreateContainerData } from "@/types";

interface ComputePageProps {
  view?: string;
}

export function ComputePage({ view: routeView = "containers" }: ComputePageProps) {
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
  const [showAddKey, setShowAddKey] = useState(false);
  const accessOpenedFor = useRef<string | null>(null);

  useEffect(() => {
    if (user) {
      loadContainers();
      loadSshKeys();
    }
  }, [user, loadContainers, loadSshKeys]);

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


  if (!user) {
    return (
      <div>
        <Breadcrumb items={[{ label: "Compute" }]} />
        <PageHeader title="Containers" description="Sign in to manage containers." />
      </div>
    );
  }

  // Terminal View
  if (showTerminal && terminalContainer) {
    return (
      <div>
        <Breadcrumb
          items={[
            { label: "Compute", href: "/compute/containers" },
            { label: "Containers", href: "/compute/containers" },
            { label: terminalContainer.name },
            { label: "Terminal" },
          ]}
        />
        <PageHeader title={`Terminal — ${terminalContainer.name}`} />
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
        <Breadcrumb
          items={[
            { label: "Compute", href: "/compute/containers" },
            { label: "Containers", href: "/compute/containers" },
            { label: selectedContainer.name },
          ]}
        />
        <PageHeader title={selectedContainer.name} />
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

  // Create Container
  if (routeView === "create") {
    return (
      <div>
        <Breadcrumb
          items={[
            { label: "Compute", href: "/compute/containers" },
            { label: "Containers", href: "/compute/containers" },
            { label: "Create" },
          ]}
        />
        <PageHeader title="Create container" description="Configure a new stateful container." />
        <CreateContainerForm
          sshKeys={sshKeys}
          onCreate={handleCreateContainer}
          onCancel={() => navigate("/compute/containers")}
          creating={creating}
        />
      </div>
    );
  }

  // SSH Keys
  if (routeView === "ssh-keys") {
    return (
      <div>
        <Breadcrumb
          items={[
            { label: "Compute", href: "/compute/containers" },
            { label: "SSH Keys" },
          ]}
        />
        <PageHeader
          title="SSH Keys"
          description="Manage SSH keys for container access."
          actions={
            !showAddKey ? (
              <Button onClick={() => setShowAddKey(true)}>
                <Plus className="w-4 h-4 mr-2" />
                Add SSH key
              </Button>
            ) : undefined
          }
        />

        <div className="bg-card border border-border rounded-lg">
          <div className="px-5 py-4 border-b border-border">
            <h2 className="text-sm font-semibold">SSH Keys</h2>
          </div>
          <div className="p-5">
            <SshKeyList
              sshKeys={sshKeys}
              onAdd={addSshKey}
              onDelete={deleteSshKey}
              loading={sshKeysLoading}
              showAdd={showAddKey}
              onCloseAdd={() => setShowAddKey(false)}
            />
          </div>
        </div>
      </div>
    );
  }

  // Containers (default)
  return (
    <div>
      <Breadcrumb
        items={[
          { label: "Compute", href: "/compute/containers" },
          { label: "Containers" },
        ]}
      />
      <PageHeader
        title="Containers"
        description="Stateful containers with persistent storage and dedicated IPs."
        actions={
          <Button onClick={() => navigate("/compute/containers/new")}>
            <Plus className="w-4 h-4 mr-2" />
            Create container
          </Button>
        }
      />

      {containersError && (
        <div className="bg-destructive/10 border border-destructive/20 rounded-lg px-4 py-3 mb-4">
          <p className="text-sm text-destructive">{containersError}</p>
        </div>
      )}

      {/* Container table */}
      <div className="bg-card border border-border rounded-lg">
        <div className="px-5 py-4 border-b border-border">
          <h2 className="text-sm font-semibold">Containers</h2>
        </div>
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
      </div>
    </div>
  );
}
