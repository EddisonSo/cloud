import { useState, useEffect } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { StatusDot, ReAuthModal } from "@/components/common";
import { buildApiBase } from "@/lib/api";
import { Server, Terminal, Clock, RefreshCw } from "lucide-react";
import { NodeTerminal } from "./NodeTerminal";
import { CronJobManager } from "./CronJobManager";

export function ClusterManagerSection() {
  const [nodes, setNodes] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [privilegedToken, setPrivilegedToken] = useState(null);
  const [tokenExpiresAt, setTokenExpiresAt] = useState(null);
  const [showReAuth, setShowReAuth] = useState(false);
  const [selectedNode, setSelectedNode] = useState(null);
  const [view, setView] = useState("list"); // "list", "terminal", "cron"

  const isTokenValid = () => {
    if (!privilegedToken || !tokenExpiresAt) return false;
    return Date.now() / 1000 < tokenExpiresAt;
  };

  const loadNodes = async () => {
    if (!isTokenValid()) {
      setShowReAuth(true);
      return;
    }

    setLoading(true);
    setError("");
    try {
      const response = await fetch(`${buildApiBase()}/cluster-manager/nodes`, {
        credentials: "include",
        headers: {
          "X-Privileged-Token": privilegedToken,
        },
      });

      if (response.status === 403) {
        setShowReAuth(true);
        return;
      }

      if (!response.ok) {
        const text = await response.text();
        throw new Error(text || "Failed to load nodes");
      }

      const data = await response.json();
      setNodes(data || []);
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (isTokenValid()) {
      loadNodes();
    }
  }, [privilegedToken]);

  const handleReAuthSuccess = (token, expiresAt) => {
    setPrivilegedToken(token);
    setTokenExpiresAt(expiresAt);
    setShowReAuth(false);
  };

  const handleOpenTerminal = (node) => {
    setSelectedNode(node);
    setView("terminal");
  };

  const handleOpenCron = (node) => {
    setSelectedNode(node);
    setView("cron");
  };

  const handleBackToList = () => {
    setSelectedNode(null);
    setView("list");
  };

  // Terminal view
  if (view === "terminal" && selectedNode) {
    return (
      <NodeTerminal
        node={selectedNode}
        privilegedToken={privilegedToken}
        onBack={handleBackToList}
      />
    );
  }

  // Cron manager view
  if (view === "cron" && selectedNode) {
    return (
      <CronJobManager
        node={selectedNode}
        privilegedToken={privilegedToken}
        onBack={handleBackToList}
      />
    );
  }

  // Node list view
  return (
    <>
      <Card>
        <CardHeader className="flex flex-row items-center justify-between space-y-0">
          <CardTitle className="flex items-center gap-2">
            <Server className="w-5 h-5" />
            Cluster Nodes
          </CardTitle>
          <Button
            variant="outline"
            size="sm"
            onClick={loadNodes}
            disabled={loading}
          >
            <RefreshCw className={`w-4 h-4 mr-2 ${loading ? "animate-spin" : ""}`} />
            Refresh
          </Button>
        </CardHeader>
        <CardContent>
          {!isTokenValid() ? (
            <div className="text-center py-8">
              <p className="text-muted-foreground mb-4">
                Re-authentication required to access cluster management.
              </p>
              <Button onClick={() => setShowReAuth(true)}>
                Authenticate
              </Button>
            </div>
          ) : error ? (
            <p className="text-destructive text-sm">{error}</p>
          ) : loading ? (
            <p className="text-muted-foreground py-4">Loading nodes...</p>
          ) : nodes.length === 0 ? (
            <p className="text-muted-foreground py-4">No nodes found</p>
          ) : (
            <div className="space-y-2">
              {/* Header - hidden on mobile */}
              <div className="hidden md:grid md:grid-cols-[2fr_1fr_1fr_1fr_auto] gap-4 px-4 py-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                <div>Node</div>
                <div className="text-center">Hostname</div>
                <div className="text-center">Uptime</div>
                <div className="text-center">Cron Jobs</div>
                <div className="text-center min-w-[180px]">Actions</div>
              </div>
              {nodes.map((node) => (
                <div
                  key={node.name}
                  className="flex flex-col md:grid md:grid-cols-[2fr_1fr_1fr_1fr_auto] gap-2 md:gap-4 px-4 py-3 bg-secondary rounded-md md:items-center"
                >
                  <div className="flex items-center gap-2">
                    <StatusDot status="running" />
                    <span className="font-medium truncate">{node.name}</span>
                  </div>
                  <div className="flex justify-between md:block md:text-center">
                    <span className="text-xs text-muted-foreground md:hidden">Hostname:</span>
                    <span className="text-sm text-muted-foreground truncate">
                      {node.hostname || node.name}
                    </span>
                  </div>
                  <div className="flex justify-between md:block md:text-center">
                    <span className="text-xs text-muted-foreground md:hidden">Uptime:</span>
                    <span className="text-sm text-muted-foreground">
                      {node.uptime || "—"}
                    </span>
                  </div>
                  <div className="flex justify-between md:block md:text-center">
                    <span className="text-xs text-muted-foreground md:hidden">Cron Jobs:</span>
                    <span className="text-sm text-muted-foreground">
                      {node.cron_count ?? "—"}
                    </span>
                  </div>
                  <div className="flex gap-2 justify-center mt-2 md:mt-0 min-w-[180px]">
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => handleOpenTerminal(node)}
                    >
                      <Terminal className="w-4 h-4 mr-1" />
                      Terminal
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => handleOpenCron(node)}
                    >
                      <Clock className="w-4 h-4 mr-1" />
                      Cron
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      <ReAuthModal
        open={showReAuth}
        onClose={() => setShowReAuth(false)}
        onSuccess={handleReAuthSuccess}
      />
    </>
  );
}
