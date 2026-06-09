import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Plus } from "lucide-react";
import type { CreateCustomDomainData } from "@/types";

interface AddDomainFormProps {
  onAdd: (data: CreateCustomDomainData) => Promise<unknown>;
}

export function AddDomainForm({ onAdd }: AddDomainFormProps) {
  const [containerId, setContainerId] = useState("");
  const [domain, setDomain] = useState("");
  const [targetPort, setTargetPort] = useState(8000);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!containerId.trim() || !domain.trim()) return;

    setSubmitting(true);
    setError("");
    try {
      await onAdd({ container_id: containerId.trim(), domain: domain.trim(), target_port: targetPort });
      setContainerId("");
      setDomain("");
      setTargetPort(8000);
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <div className="grid gap-4 sm:grid-cols-3">
        <div className="space-y-1.5">
          <Label htmlFor="add-container-id">Container ID</Label>
          <Input
            id="add-container-id"
            placeholder="e.g. abc12345"
            value={containerId}
            onChange={(e) => setContainerId(e.target.value)}
            required
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="add-domain">Domain</Label>
          <Input
            id="add-domain"
            placeholder="e.g. app.example.com"
            value={domain}
            onChange={(e) => setDomain(e.target.value)}
            required
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="add-port">Target port</Label>
          <Input
            id="add-port"
            type="number"
            min={1}
            max={65535}
            value={targetPort}
            onChange={(e) => setTargetPort(Number(e.target.value))}
            required
          />
        </div>
      </div>

      {error && (
        <p className="text-sm text-destructive">{error}</p>
      )}

      <Button type="submit" disabled={submitting || !containerId.trim() || !domain.trim()}>
        <Plus className="w-4 h-4 mr-2" />
        {submitting ? "Adding..." : "Add domain"}
      </Button>
    </form>
  );
}
