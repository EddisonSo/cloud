import { useState } from "react";
import { NavLink, useLocation } from "react-router-dom";
import { cn } from "@/lib/utils";
import { useAuth } from "@/contexts/AuthContext";
import { NAV_ITEMS, ADMIN_NAV_ITEM } from "@/lib/constants";
import { ThemeToggle } from "./ThemeToggle";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { StatusDot } from "@/components/common";

export function Sidebar({ healthOk = true }) {
  const location = useLocation();
  const { user, isAdmin, login, logout } = useAuth();
  const [loginForm, setLoginForm] = useState({ username: "", password: "" });
  const [loginError, setLoginError] = useState("");
  const [loggingIn, setLoggingIn] = useState(false);

  const navItems = isAdmin ? [...NAV_ITEMS, ADMIN_NAV_ITEM] : NAV_ITEMS;

  const handleLogin = async (e) => {
    e.preventDefault();
    setLoginError("");
    setLoggingIn(true);
    try {
      await login(loginForm.username, loginForm.password);
      setLoginForm({ username: "", password: "" });
    } catch (err) {
      setLoginError(err.message);
    } finally {
      setLoggingIn(false);
    }
  };

  return (
    <aside className="fixed top-0 left-0 h-screen w-[220px] flex flex-col bg-card border-r border-border p-4 overflow-y-auto">
      {/* Brand */}
      <div className="flex items-center gap-3 px-2 py-1 mb-4">
        <span className="flex items-center justify-center w-8 h-8 rounded-md bg-gradient-to-br from-primary to-purple-500 text-white text-sm font-bold">
          E
        </span>
        <span className="font-semibold text-[15px]">Edd Cloud</span>
      </div>

      {/* Navigation */}
      <nav className="flex flex-col gap-1 flex-1">
        {navItems.map((item) => {
          const Icon = item.icon;
          const isActive = location.pathname.startsWith(item.path);

          return (
            <NavLink
              key={item.id}
              to={item.path}
              className={cn(
                "flex items-center gap-3 px-3 py-2 rounded-md text-[13px] font-medium transition-colors relative",
                "text-muted-foreground hover:bg-accent hover:text-foreground",
                isActive && "bg-primary/10 text-primary"
              )}
            >
              {isActive && (
                <span className="absolute left-0 top-1/2 -translate-y-1/2 w-[3px] h-4 bg-primary rounded-r-sm" />
              )}
              <Icon className="w-4 h-4 opacity-80" />
              {item.label}
              {item.id === "health" && (
                <StatusDot status={healthOk ? "ok" : "down"} className="ml-auto" />
              )}
            </NavLink>
          );
        })}
      </nav>

      {/* Footer */}
      <div className="pt-4 mt-auto border-t border-border">
        <div className="flex items-center justify-between mb-3">
          <span className="text-xs text-muted-foreground">Theme</span>
          <ThemeToggle />
        </div>

        {user ? (
          <>
            <div className="flex items-center gap-3 p-3 bg-secondary rounded-lg">
              <div className="w-8 h-8 rounded-full bg-accent flex items-center justify-center text-xs font-semibold">
                {user.charAt(0).toUpperCase()}
              </div>
              <div className="flex-1 min-w-0">
                <div className="text-sm font-medium truncate">{user}</div>
              </div>
            </div>
            <Button
              variant="ghost"
              className="w-full mt-3 justify-center"
              onClick={logout}
            >
              Sign out
            </Button>
          </>
        ) : (
          <form onSubmit={handleLogin} className="space-y-3">
            <div className="space-y-2">
              <Label htmlFor="sidebar-username" className="text-xs">Username</Label>
              <Input
                id="sidebar-username"
                type="text"
                value={loginForm.username}
                onChange={(e) => setLoginForm((p) => ({ ...p, username: e.target.value }))}
                autoComplete="username"
                className="h-8"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="sidebar-password" className="text-xs">Password</Label>
              <Input
                id="sidebar-password"
                type="password"
                value={loginForm.password}
                onChange={(e) => setLoginForm((p) => ({ ...p, password: e.target.value }))}
                autoComplete="current-password"
                className="h-8"
              />
            </div>
            <Button type="submit" className="w-full" disabled={loggingIn}>
              {loggingIn ? "Signing in..." : "Sign in"}
            </Button>
            {loginError && (
              <p className="text-xs text-destructive">{loginError}</p>
            )}
          </form>
        )}
      </div>
    </aside>
  );
}
