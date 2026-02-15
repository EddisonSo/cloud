import React, { useState } from "react";
import { Outlet } from "react-router-dom";
import { TopBar } from "./TopBar";
import { Sidebar } from "./Sidebar";
import { ThemeToggle } from "./ThemeToggle";
import { useHealth } from "@/hooks";
import { useAuth } from "@/contexts/AuthContext";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Shield } from "lucide-react";

export function AppLayout() {
  const { user, loading: authLoading, login, challengeToken, complete2FA, cancel2FA } = useAuth();
  const { health } = useHealth(user, true);
  const isMobile = typeof window !== "undefined" && window.innerWidth < 768;
  const [sidebarCollapsed, setSidebarCollapsed] = useState(isMobile);
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

  // Login / 2FA screen
  if (!authLoading && !user) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-background p-6">
        <div className="absolute top-4 right-4">
          <ThemeToggle />
        </div>
        <div className="w-full max-w-sm bg-card border border-border rounded-lg p-8">
          <h1 className="text-xl font-semibold text-center mb-6">Sign in to Edd Cloud</h1>

          {challengeToken ? (
            <div className="flex flex-col items-center gap-3 py-4">
              <div className="w-12 h-12 rounded-full bg-primary/10 flex items-center justify-center">
                <Shield className="w-6 h-6 text-primary" />
              </div>
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
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen">
      <TopBar onToggleSidebar={() => setSidebarCollapsed((c) => !c)} />
      <Sidebar healthOk={health.cluster_ok} collapsed={sidebarCollapsed} onClose={() => setSidebarCollapsed(true)} />
      <main
        className={`pt-14 min-h-screen transition-[margin] ${
          sidebarCollapsed ? "ml-0" : "md:ml-[240px]"
        }`}
      >
        <div className="p-4 md:p-6">
          <Outlet />
        </div>
      </main>
    </div>
  );
}
