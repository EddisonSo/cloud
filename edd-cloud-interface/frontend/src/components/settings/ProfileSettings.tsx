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
            <div className="grid grid-cols-[140px_1fr] items-center gap-x-4 max-w-md">
              <Label htmlFor="display-name" className="text-right text-muted-foreground">Name</Label>
              <Input
                id="display-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="Your display name"
              />
            </div>
            {nameError && <p className="text-sm text-destructive ml-[156px]">{nameError}</p>}
            {nameSuccess && <p className="text-sm text-success ml-[156px]">Display name updated.</p>}
            <div className="ml-[156px]">
              <Button type="submit" disabled={nameSaving}>
                {nameSaving ? "Saving..." : "Save"}
              </Button>
            </div>
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
            <div className="grid grid-cols-[140px_1fr] items-center gap-4 max-w-md">
              <Label htmlFor="current-password" className="text-right text-muted-foreground">Current password</Label>
              <Input
                id="current-password"
                type="password"
                value={currentPw}
                onChange={(e) => setCurrentPw(e.target.value)}
                autoComplete="current-password"
              />
              <Label htmlFor="new-password" className="text-right text-muted-foreground">New password</Label>
              <Input
                id="new-password"
                type="password"
                value={newPw}
                onChange={(e) => setNewPw(e.target.value)}
                autoComplete="new-password"
              />
              <Label htmlFor="confirm-password" className="text-right text-muted-foreground">Confirm password</Label>
              <Input
                id="confirm-password"
                type="password"
                value={confirmPw}
                onChange={(e) => setConfirmPw(e.target.value)}
                autoComplete="new-password"
              />
            </div>
            {pwError && <p className="text-sm text-destructive ml-[156px]">{pwError}</p>}
            {pwSuccess && (
              <p className="text-sm text-success ml-[156px]">
                Password changed successfully.
              </p>
            )}
            <div className="ml-[156px]">
              <Button type="submit" disabled={pwSaving}>
                {pwSaving ? "Changing..." : "Change password"}
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
