import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { X } from "lucide-react";

export function CreateContainerForm({
  sshKeys,
  onCreate,
  onCancel,
  creating,
}) {
  const [name, setName] = useState("");
  const [memoryMb, setMemoryMb] = useState(512);
  const [storageGb, setStorageGb] = useState(5);
  const [selectedKeyIds, setSelectedKeyIds] = useState([]);
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
      ssh_key_ids: selectedKeyIds,
    });
  };

  const toggleKey = (id) => {
    setSelectedKeyIds((prev) =>
      prev.includes(id) ? prev.filter((k) => k !== id) : [...prev, id]
    );
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
