import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Switch } from "@/components/ui/switch";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { X, Plus, Trash2 } from "lucide-react";

export function CreateContainerForm({
  sshKeys,
  onCreate,
  onCancel,
  creating,
}) {
  const [name, setName] = useState("");
  const [memoryMb, setMemoryMb] = useState(512);
  const [storageGb, setStorageGb] = useState(5);
  const [instanceType, setInstanceType] = useState("nano");
  const [selectedKeyIds, setSelectedKeyIds] = useState([]);
  const [enableSsh, setEnableSsh] = useState(true);
  const [ingressRules, setIngressRules] = useState([]);
  const [newPort, setNewPort] = useState("");
  const [newTargetPort, setNewTargetPort] = useState("");
  const [error, setError] = useState("");

  const handleSubmit = async (e) => {
    e.preventDefault();
    if (!name.trim()) {
      setError("Container name is required");
      return;
    }
    if (selectedKeyIds.length === 0) {
      setError("Select at least one SSH key");
      return;
    }
    setError("");
    await onCreate?.({
      name: name.trim(),
      memory_mb: memoryMb,
      storage_gb: storageGb,
      instance_type: instanceType,
      ssh_key_ids: selectedKeyIds,
      enable_ssh: enableSsh,
      ingress_rules: ingressRules,
    });
  };

  const toggleKey = (id) => {
    setSelectedKeyIds((prev) =>
      prev.includes(id) ? prev.filter((k) => k !== id) : [...prev, id]
    );
  };

  const handleAddRule = (e) => {
    e.preventDefault();
    if (!newPort) return;
    const port = parseInt(newPort, 10);
    const targetPort = newTargetPort ? parseInt(newTargetPort, 10) : port;
    if (ingressRules.some((r) => r.port === port)) {
      setError(`Port ${port} already added`);
      return;
    }
    setIngressRules((prev) => [...prev, { port, target_port: targetPort }]);
    setNewPort("");
    setNewTargetPort("");
    setError("");
  };

  const handleRemoveRule = (port) => {
    setIngressRules((prev) => prev.filter((r) => r.port !== port));
  };

  return (
    <Card className="mb-4">
      <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-4">
        <CardTitle className="text-sm font-semibold">Create Container</CardTitle>
        <Button variant="ghost" size="icon" className="h-8 w-8" onClick={onCancel}>
          <X className="w-4 h-4" />
        </Button>
      </CardHeader>
      <CardContent>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="grid grid-cols-3 gap-4">
            <div className="space-y-2">
              <Label htmlFor="c-name">Name</Label>
              <Input
                id="c-name"
                placeholder="my-container"
                value={name}
                onChange={(e) => setName(e.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="c-memory">Memory (MB)</Label>
              <Input
                id="c-memory"
                type="number"
                value={memoryMb}
                onChange={(e) => setMemoryMb(parseInt(e.target.value, 10))}
                min={128}
                max={8192}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="c-storage">Storage (GB)</Label>
              <Input
                id="c-storage"
                type="number"
                value={storageGb}
                onChange={(e) => setStorageGb(parseInt(e.target.value, 10))}
                min={1}
                max={100}
              />
            </div>
          </div>

          <div className="space-y-2">
            <Label htmlFor="c-instance-type">Instance Type</Label>
            <Select value={instanceType} onValueChange={setInstanceType}>
              <SelectTrigger id="c-instance-type" className="w-full">
                <SelectValue placeholder="Select instance type" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="nano">Nano (ARM64, 0.5 CPU)</SelectItem>
                <SelectItem value="micro">Micro (ARM64, 1 CPU)</SelectItem>
                <SelectItem value="mini">Mini (ARM64, 2 CPU)</SelectItem>
                <SelectItem value="tiny">Tiny (AMD64, 1 CPU)</SelectItem>
                <SelectItem value="small">Small (AMD64, 2 CPU)</SelectItem>
                <SelectItem value="medium">Medium (AMD64, 4 CPU)</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-2">
            <Label>SSH Keys</Label>
            <div className="max-h-48 overflow-y-auto p-3 bg-background border border-border rounded-md space-y-2">
              {sshKeys.length === 0 ? (
                <p className="text-sm text-muted-foreground">No SSH keys. Add one first.</p>
              ) : (
                sshKeys.map((key) => (
                  <label
                    key={key.id}
                    className={`flex items-center gap-3 p-2 rounded-md border cursor-pointer transition-colors ${
                      selectedKeyIds.includes(key.id)
                        ? "border-primary bg-primary/10"
                        : "border-border hover:border-primary"
                    }`}
                  >
                    <input
                      type="checkbox"
                      checked={selectedKeyIds.includes(key.id)}
                      onChange={() => toggleKey(key.id)}
                      className="w-4 h-4 accent-primary"
                    />
                    <span className="text-sm font-medium">{key.name}</span>
                    <span className="text-xs text-muted-foreground font-mono ml-auto truncate max-w-[200px]">
                      {key.fingerprint}
                    </span>
                  </label>
                ))
              )}
            </div>
          </div>

          {/* Access Control */}
          <div className="space-y-4 pt-2 border-t border-border">
            <h4 className="text-sm font-semibold">Access Control</h4>

            {/* SSH Toggle */}
            <div className="flex items-center justify-between p-3 bg-secondary rounded-md">
              <div>
                <span className="font-medium text-sm">SSH Access</span>
                <p className="text-xs text-muted-foreground">Enable SSH on port 22</p>
              </div>
              <Switch checked={enableSsh} onCheckedChange={setEnableSsh} />
            </div>

            {/* HTTP Ingress Rules */}
            <div className="space-y-3">
              <Label>HTTP Ingress Rules</Label>
              <p className="text-xs text-muted-foreground">
                Expose ports to the internet via the external IP.
              </p>

              {/* Add Rule */}
              <div className="flex items-center gap-2">
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
                <Button type="button" variant="outline" size="sm" onClick={handleAddRule} disabled={!newPort}>
                  <Plus className="w-4 h-4 mr-1" />
                  Add
                </Button>
              </div>

              {/* Rules List */}
              <div className="space-y-2">
                {ingressRules.length === 0 ? (
                  <p className="text-sm text-muted-foreground py-1">No ingress rules configured</p>
                ) : (
                  ingressRules.map((rule) => (
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
                        type="button"
                        variant="ghost"
                        size="icon"
                        className="h-8 w-8 text-destructive hover:text-destructive hover:bg-destructive/10"
                        onClick={() => handleRemoveRule(rule.port)}
                      >
                        <Trash2 className="w-4 h-4" />
                      </Button>
                    </div>
                  ))
                )}
              </div>
            </div>
          </div>

          {error && <p className="text-destructive text-sm">{error}</p>}

          <div className="flex justify-end gap-2">
            <Button type="button" variant="outline" onClick={onCancel}>
              Cancel
            </Button>
            <Button type="submit" disabled={creating}>
              {creating ? "Creating..." : "Create Container"}
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  );
}
