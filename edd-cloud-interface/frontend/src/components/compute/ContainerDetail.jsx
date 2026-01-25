import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { StatusBadge, CopyableText } from "@/components/common";
import { ArrowLeft, Plus, Trash2, Terminal, Play, Square } from "lucide-react";
import { formatBytes } from "@/lib/formatters";

export function ContainerDetail({
  container,
  access,
  onBack,
  onStart,
  onStop,
  onDelete,
  onTerminal,
  actions,
}) {
  const [newPort, setNewPort] = useState("");
  const [newTargetPort, setNewTargetPort] = useState("");
  const [newMountPath, setNewMountPath] = useState("");

  const action = actions?.[container.id];
  const isRunning = container.status === "running";
  const isStopped = container.status === "stopped";

  const handleAddRule = async (e) => {
    e.preventDefault();
    if (!newPort) return;
    const port = parseInt(newPort, 10);
    const targetPort = newTargetPort ? parseInt(newTargetPort, 10) : port;
    await access.addIngressRule(port, targetPort);
    setNewPort("");
    setNewTargetPort("");
  };

  const handleAddMountPath = async (e) => {
    e.preventDefault();
    if (!newMountPath) return;
    const path = newMountPath.startsWith("/") ? newMountPath : "/" + newMountPath;
    if (access.mountPaths.includes(path)) return;
    await access.updateMountPaths([...access.mountPaths, path]);
    setNewMountPath("");
  };

  const handleRemoveMountPath = async (path) => {
    const updated = access.mountPaths.filter((p) => p !== path);
    await access.updateMountPaths(updated);
  };

  return (
    <div className="max-w-3xl">
      {/* Header */}
      <div className="flex items-center gap-4 mb-6">
        <Button variant="outline" size="sm" onClick={onBack}>
          <ArrowLeft className="w-4 h-4 mr-2" />
          Back
        </Button>
        <h2 className="text-xl font-semibold">{container.name}</h2>
        <StatusBadge status={container.status} />
      </div>

      {/* Info Section */}
      <Card className="mb-4">
        <CardHeader>
          <CardTitle className="text-sm">Container Info</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-3 gap-6 mb-4">
            <div>
              <span className="text-xs font-medium uppercase tracking-wider text-muted-foreground block mb-1">
                ID
              </span>
              <CopyableText text={container.id.slice(0, 8)} mono />
            </div>
            <div>
              <span className="text-xs font-medium uppercase tracking-wider text-muted-foreground block mb-1">
                Memory
              </span>
              <span className="text-sm">{container.memory_mb} MB</span>
            </div>
            <div>
              <span className="text-xs font-medium uppercase tracking-wider text-muted-foreground block mb-1">
                Storage
              </span>
              <span className="text-sm">{container.storage_gb} GB</span>
            </div>
          </div>
          <div>
            <span className="text-xs font-medium uppercase tracking-wider text-muted-foreground block mb-1">
              Hostname
            </span>
            {container.hostname ? (
              <CopyableText text={container.hostname} mono className="text-sm" />
            ) : (
              <span className="font-mono text-sm">—</span>
            )}
          </div>

          {/* Actions */}
          <div className="flex gap-2 mt-6 pt-4 border-t border-border">
            {isRunning && (
              <>
                <Button variant="outline" onClick={() => onTerminal?.(container)}>
                  <Terminal className="w-4 h-4 mr-2" />
                  Terminal
                </Button>
                <Button
                  variant="outline"
                  onClick={() => onStop?.(container.id)}
                  disabled={action === "stopping"}
                >
                  <Square className="w-4 h-4 mr-2" />
                  {action === "stopping" ? "Stopping..." : "Stop"}
                </Button>
              </>
            )}
            {isStopped && (
              <Button
                variant="outline"
                onClick={() => onStart?.(container.id)}
                disabled={action === "starting"}
              >
                <Play className="w-4 h-4 mr-2" />
                {action === "starting" ? "Starting..." : "Start"}
              </Button>
            )}
            <Button
              variant="destructive"
              onClick={() => onDelete?.(container.id)}
              disabled={action === "deleting"}
            >
              <Trash2 className="w-4 h-4 mr-2" />
              {action === "deleting" ? "Deleting..." : "Delete"}
            </Button>
          </div>
        </CardContent>
      </Card>

      {/* Access Section */}
      <Card className="mb-4">
        <CardHeader>
          <CardTitle className="text-sm">Access Control</CardTitle>
        </CardHeader>
        <CardContent className="space-y-6">
          {/* SSH Toggle */}
          <div className="p-4 bg-secondary rounded-md">
            <div className="flex items-center justify-between">
              <div>
                <span className="font-medium">SSH Access</span>
                <p className="text-sm text-muted-foreground font-mono">Port 22</p>
              </div>
              <Switch
                checked={access.sshEnabled}
                onCheckedChange={() => access.toggleSSH()}
                disabled={access.savingSSH || !isRunning}
              />
            </div>
            {access.sshEnabled && (
              <div className="mt-3 pt-3 border-t border-border">
                <CopyableText
                  text={`ssh ${container.id}@compute.cloud.eddisonso.com`}
                  mono
                  className="text-sm"
                />
              </div>
            )}
          </div>

          {/* HTTP Ingress Rules */}
          <div>
            <h4 className="text-sm font-semibold mb-3">HTTP Ingress Rules</h4>
            <p className="text-xs text-muted-foreground mb-3">
              Expose ports to the internet via the hostname.
            </p>

            {/* Add Rule */}
            <form onSubmit={handleAddRule} className="flex items-center gap-2 mb-4">
              <Input
                type="number"
                placeholder="Port"
                value={newPort}
                onChange={(e) => setNewPort(e.target.value)}
                className="w-24"
                min={1}
                max={65535}
              />
              <span className="text-muted-foreground">→</span>
              <Input
                type="number"
                placeholder="Target (opt)"
                value={newTargetPort}
                onChange={(e) => setNewTargetPort(e.target.value)}
                className="w-28"
                min={1}
                max={65535}
              />
              <Button type="submit" size="sm" disabled={!newPort || access.addingRule}>
                <Plus className="w-4 h-4 mr-1" />
                Add
              </Button>
            </form>

            {/* Rules List */}
            <div className="space-y-2">
              {access.ingressRules.length === 0 ? (
                <p className="text-sm text-muted-foreground py-2">No ingress rules configured</p>
              ) : (
                access.ingressRules.map((rule) => (
                  <div
                    key={rule.port}
                    className="flex items-center justify-between p-3 bg-secondary rounded-md"
                  >
                    <div className="flex items-center gap-3">
                      <span className="font-medium">:{rule.port}</span>
                      <span className="text-xs text-muted-foreground">→</span>
                      <span className="text-muted-foreground">:{rule.target_port || rule.port}</span>
                    </div>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-8 w-8 text-destructive hover:text-destructive hover:bg-destructive/10"
                      onClick={() => access.removeIngressRule(rule.port)}
                    >
                      <Trash2 className="w-4 h-4" />
                    </Button>
                  </div>
                ))
              )}
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Persistent Storage Section */}
      <Card className="mb-4">
        <CardHeader>
          <CardTitle className="text-sm">Persistent Storage</CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-xs text-muted-foreground mb-3">
            Directories to persist across container restarts. Changes require a container restart.
          </p>

          {/* Add Mount Path */}
          <form onSubmit={handleAddMountPath} className="flex items-center gap-2 mb-4">
            <Input
              type="text"
              placeholder="/path/to/persist"
              value={newMountPath}
              onChange={(e) => setNewMountPath(e.target.value)}
              className="flex-1"
            />
            <Button type="submit" size="sm" disabled={!newMountPath || access.savingMounts}>
              <Plus className="w-4 h-4 mr-1" />
              Add
            </Button>
          </form>

          {/* Mount Paths List */}
          <div className="space-y-2">
            {access.mountPaths.length === 0 ? (
              <p className="text-sm text-muted-foreground py-2">No persistent paths configured</p>
            ) : (
              access.mountPaths.map((path) => (
                <div
                  key={path}
                  className="flex items-center justify-between p-3 bg-secondary rounded-md"
                >
                  <span className="font-mono text-sm">{path}</span>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-8 w-8 text-destructive hover:text-destructive hover:bg-destructive/10"
                    onClick={() => handleRemoveMountPath(path)}
                    disabled={access.savingMounts || access.mountPaths.length <= 1}
                    title={access.mountPaths.length <= 1 ? "At least one path required" : "Remove path"}
                  >
                    <Trash2 className="w-4 h-4" />
                  </Button>
                </div>
              ))
            )}
          </div>
          {access.savingMounts && (
            <p className="text-xs text-muted-foreground mt-3">Updating and restarting container...</p>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
