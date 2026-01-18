import { useState } from "react";
import { Modal } from "./Modal";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { buildApiBase } from "@/lib/api";
import { Shield } from "lucide-react";

export function ReAuthModal({ open, onClose, onSuccess }) {
  const [password, setPassword] = useState("");
  const [verifying, setVerifying] = useState(false);
  const [error, setError] = useState("");

  const handleSubmit = async (e) => {
    e.preventDefault();
    if (!password) return;

    setVerifying(true);
    setError("");

    try {
      const response = await fetch(`${buildApiBase()}/api/verify-password`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify({ password }),
      });

      if (!response.ok) {
        const text = await response.text();
        throw new Error(text || "Invalid password");
      }

      const data = await response.json();
      setPassword("");
      onSuccess?.(data.token, data.expires_at);
      onClose?.();
    } catch (err) {
      setError(err.message);
    } finally {
      setVerifying(false);
    }
  };

  const handleClose = () => {
    setPassword("");
    setError("");
    onClose?.();
  };

  return (
    <Modal
      open={open}
      onClose={handleClose}
      title="Confirm Identity"
      description="Enter your password to access privileged operations."
    >
      <form onSubmit={handleSubmit} className="space-y-4">
        <div className="flex items-center gap-3 p-3 bg-amber-500/10 border border-amber-500/20 rounded-lg text-amber-400 text-sm">
          <Shield className="w-5 h-5 flex-shrink-0" />
          <span>This action requires re-authentication for security.</span>
        </div>

        <div className="space-y-2">
          <Label htmlFor="reauth-password">Password</Label>
          <Input
            id="reauth-password"
            type="password"
            placeholder="Enter your password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoFocus
          />
        </div>

        {error && <p className="text-sm text-destructive">{error}</p>}

        <div className="flex justify-end gap-2 pt-2">
          <Button type="button" variant="outline" onClick={handleClose}>
            Cancel
          </Button>
          <Button type="submit" disabled={verifying || !password}>
            {verifying ? "Verifying..." : "Confirm"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
