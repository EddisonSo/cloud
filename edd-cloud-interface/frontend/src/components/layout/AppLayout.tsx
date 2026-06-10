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

  // Login / 2FA gate — brand moment
  if (!authLoading && !user) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-background p-6">
        {/* Theme toggle — top-right */}
        <div className="absolute top-4 right-4">
          <ThemeToggle />
        </div>

        {/*
         * Centered flat panel: bg-card, 1px border, NO radius, NO shadow.
         * Brand mark up top: EDD/CLOUD mono, slash in ice primary.
         */}
        <div className="w-full max-w-sm bg-card border border-border p-8">
          {/* Brand mark */}
          <div className="text-center mb-8">
            <div className="font-mono text-[22px] font-semibold tracking-[0.2em] select-none leading-none">
              EDD<span className="text-primary">/</span>CLOUD
            </div>
            <p className="font-mono text-[10px] font-medium uppercase tracking-[0.22em] text-faint mt-2">
              Dashboard
            </p>
          </div>

          {challengeToken ? (
            /* 2FA flow */
            <div className="flex flex-col items-center gap-3 py-2">
              {/* Square icon container — flat, no pill */}
              <div className="w-10 h-10 border border-border flex items-center justify-center text-primary">
                <Shield className="w-5 h-5" />
              </div>

              {twoFAState === "waiting" ? (
                <>
                  <p className="text-[13.5px] font-medium text-center">
                    Touch your security key
                  </p>
                  <p className="font-mono text-[11px] text-muted-foreground text-center">
                    Waiting for verification...
                  </p>
                </>
              ) : twoFAState === "error" ? (
                <>
                  <p className="text-[13.5px] font-medium text-destructive text-center">
                    Verification failed
                  </p>
                  <p className="font-mono text-[11px] text-muted-foreground text-center">
                    {twoFAError}
                  </p>
                  <Button size="sm" onClick={handleVerify2FA} className="mt-2">
                    Retry
                  </Button>
                </>
              ) : (
                <>
                  <p className="text-[13.5px] font-medium text-center">
                    Security key required
                  </p>
                  <p className="font-mono text-[11px] text-muted-foreground text-center">
                    Click below to verify with your key
                  </p>
                  <Button onClick={handleVerify2FA} className="mt-2">
                    Verify with security key
                  </Button>
                </>
              )}

              <Button
                variant="ghost"
                size="sm"
                onClick={handleCancel2FA}
                className="mt-1"
              >
                Cancel
              </Button>
            </div>
          ) : (
            /* Credentials form */
            <form onSubmit={handleLogin} className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="login-username">Username</Label>
                <Input
                  id="login-username"
                  type="text"
                  value={loginForm.username}
                  onChange={(e) =>
                    setLoginForm((p) => ({ ...p, username: e.target.value }))
                  }
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
                  onChange={(e) =>
                    setLoginForm((p) => ({ ...p, password: e.target.value }))
                  }
                  autoComplete="current-password"
                />
              </div>
              <Button
                type="submit"
                className="w-full"
                disabled={loggingIn}
              >
                {loggingIn ? "Signing in..." : "Sign in"}
              </Button>
              {loginError && (
                <p className="font-mono text-[11px] text-destructive text-center">
                  {loginError}
                </p>
              )}
            </form>
          )}
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-background">
      <TopBar onToggleSidebar={() => setSidebarCollapsed((c) => !c)} />
      <Sidebar
        healthOk={health.cluster_ok}
        collapsed={sidebarCollapsed}
        onClose={() => setSidebarCollapsed(true)}
      />
      <main
        className={`pt-14 min-h-screen bg-background transition-[margin] duration-150 ${
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
