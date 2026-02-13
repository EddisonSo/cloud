import React, { useState } from "react";
import { Outlet } from "react-router-dom";
import { Sidebar } from "./Sidebar";
import { ThemeToggle } from "./ThemeToggle";
import { useHealth } from "@/hooks";
import { useAuth } from "@/contexts/AuthContext";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { NotificationBell } from "@/components/notifications/NotificationBell";
import { Shield } from "lucide-react";

export function AppLayout() {
  const { user, loading: authLoading, login, challengeToken, complete2FA, cancel2FA } = useAuth();
  const { health } = useHealth(user, true);
  const [loginForm, setLoginForm] = useState({ username: "", password: "" });
  const [loginError, setLoginError] = useState("");
  const [loggingIn, setLoggingIn] = useState(false);
  const [twoFAState, setTwoFAState] = useState<"idle" | "waiting" | "error">("idle");
  const [twoFAError, setTwoFAError] = useState("");

  const handleLogin = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    setLoginError("");
    setLoggingIn(true);
    try {
      await login(loginForm.username, loginForm.password);
      setLoginForm({ username: "", password: "" });
    } catch (err) {
      setLoginError((err as Error).message);
    } finally {
      setLoggingIn(false);
    }
  };

  const handleVerify2FA = async () => {
    setTwoFAState("waiting");
    setTwoFAError("");
    try {
      await complete2FA();
      setTwoFAState("idle");
    } catch (err) {
      setTwoFAState("error");
      setTwoFAError((err as Error).message);
    }
  };

  const handleCancel2FA = () => {
    cancel2FA();
    setTwoFAState("idle");
    setTwoFAError("");
  };

  // Show login or 2FA only after auth check confirms user is not logged in
  if (!authLoading && !user) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-background p-6">
        <div className="absolute top-4 right-4">
          <ThemeToggle />
        </div>
        <Card className="w-full max-w-sm">
          <CardHeader className="text-center">
            <CardTitle>Sign in to Edd Cloud</CardTitle>
          </CardHeader>
          <CardContent>
            {challengeToken ? (
              <div className="flex flex-col items-center gap-3 py-4">
                <Shield className="w-10 h-10 text-primary" />
                {twoFAState === "waiting" ? (
                  <>
                    <p className="text-sm font-medium">Touch your security key</p>
                    <p className="text-xs text-muted-foreground">Waiting for verification...</p>
                  </>
                ) : twoFAState === "error" ? (
                  <>
                    <p className="text-sm font-medium text-destructive">Verification failed</p>
                    <p className="text-xs text-muted-foreground">{twoFAError}</p>
                    <Button size="sm" onClick={handleVerify2FA} className="mt-2">
                      Retry
                    </Button>
                  </>
                ) : (
                  <>
                    <p className="text-sm font-medium">Security key required</p>
                    <p className="text-xs text-muted-foreground">Click below to verify with your key</p>
                    <Button onClick={handleVerify2FA} className="mt-2">
                      Verify with security key
                    </Button>
                  </>
                )}
                <Button variant="ghost" size="sm" onClick={handleCancel2FA} className="mt-2">
                  Cancel
                </Button>
              </div>
            ) : (
            <form onSubmit={handleLogin} className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="login-username">Username</Label>
                <Input
                  id="login-username"
                  type="text"
                  value={loginForm.username}
                  onChange={(e) => setLoginForm((p) => ({ ...p, username: e.target.value }))}
                  autoComplete="username"
                  autoFocus
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="login-password">Password</Label>
                <Input
                  id="login-password"
                  type="password"
                  value={loginForm.password}
                  onChange={(e) => setLoginForm((p) => ({ ...p, password: e.target.value }))}
                  autoComplete="current-password"
                />
              </div>
              <Button type="submit" className="w-full" disabled={loggingIn}>
                {loggingIn ? "Signing in..." : "Sign in"}
              </Button>
              {loginError && (
                <p className="text-sm text-destructive text-center">{loginError}</p>
              )}
            </form>
            )}
          </CardContent>
        </Card>
      </div>
    );
  }

  return (
    <div className="min-h-screen overflow-hidden">
      <Sidebar healthOk={health.cluster_ok} />
      <div className="fixed top-4 right-6 z-40">
        <NotificationBell />
      </div>
      <main className="ml-[220px] p-6 min-h-screen overflow-x-hidden">
        <Outlet />
      </main>
    </div>
  );
}
