import { useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useAuth } from "@/contexts/AuthContext";
import { updateProfile, changePassword } from "@/lib/settings-api";

export function ProfileSettings() {
  const { displayName, checkSession } = useAuth();

  const [name, setName] = useState(displayName || "");
  const [nameSaving, setNameSaving] = useState(false);
  const [nameSuccess, setNameSuccess] = useState(false);
  const [nameError, setNameError] = useState("");

  const [currentPw, setCurrentPw] = useState("");
  const [newPw, setNewPw] = useState("");
  const [confirmPw, setConfirmPw] = useState("");
  const [pwSaving, setPwSaving] = useState(false);
  const [pwSuccess, setPwSuccess] = useState(false);
  const [pwError, setPwError] = useState("");

  const handleNameSave = async (e: React.FormEvent) => {
    e.preventDefault();
    setNameError("");
    setNameSuccess(false);
    setNameSaving(true);
    try {
      await updateProfile(name);
      await checkSession();
      setNameSuccess(true);
      setTimeout(() => setNameSuccess(false), 3000);
    } catch (err) {
      setNameError((err as Error).message);
    } finally {
      setNameSaving(false);
    }
  };

  const handlePasswordSave = async (e: React.FormEvent) => {
    e.preventDefault();
    setPwError("");
    setPwSuccess(false);

    if (newPw !== confirmPw) {
      setPwError("New passwords do not match");
      return;
    }
    if (newPw.length < 4) {
      setPwError("Password must be at least 4 characters");
      return;
    }

    setPwSaving(true);
    try {
      await changePassword(currentPw, newPw);
      setCurrentPw("");
      setNewPw("");
      setConfirmPw("");
      setPwSuccess(true);
      setTimeout(() => setPwSuccess(false), 3000);
    } catch (err) {
      setPwError((err as Error).message);
    } finally {
      setPwSaving(false);
    }
  };

  return (
    <div className="space-y-6">
      {/* Display Name */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Display Name</CardTitle>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleNameSave} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="display-name">Name</Label>
              <Input
                id="display-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="Your display name"
                className="max-w-sm"
              />
            </div>
            {nameError && <p className="text-sm text-destructive">{nameError}</p>}
            {nameSuccess && <p className="text-sm text-success">Display name updated.</p>}
            <Button type="submit" disabled={nameSaving}>
              {nameSaving ? "Saving..." : "Save"}
            </Button>
          </form>
        </CardContent>
      </Card>

      {/* Change Password */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Change Password</CardTitle>
        </CardHeader>
        <CardContent>
          <form onSubmit={handlePasswordSave} className="space-y-4">
            <div className="grid gap-4 max-w-sm">
              <div className="space-y-2">
                <Label htmlFor="current-password">Current password</Label>
                <Input
                  id="current-password"
                  type="password"
                  value={currentPw}
                  onChange={(e) => setCurrentPw(e.target.value)}
                  autoComplete="current-password"
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="new-password">New password</Label>
                <Input
                  id="new-password"
                  type="password"
                  value={newPw}
                  onChange={(e) => setNewPw(e.target.value)}
                  autoComplete="new-password"
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="confirm-password">Confirm new password</Label>
                <Input
                  id="confirm-password"
                  type="password"
                  value={confirmPw}
                  onChange={(e) => setConfirmPw(e.target.value)}
                  autoComplete="new-password"
                />
              </div>
            </div>
            {pwError && <p className="text-sm text-destructive">{pwError}</p>}
            {pwSuccess && (
              <p className="text-sm text-success">
                Password changed. Other sessions have been revoked.
              </p>
            )}
            <Button type="submit" disabled={pwSaving}>
              {pwSaving ? "Changing..." : "Change password"}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
