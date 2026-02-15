import { useAuth } from "@/contexts/AuthContext";
import { NotificationBell } from "@/components/notifications/NotificationBell";
import { ThemeToggle } from "./ThemeToggle";
import { Menu, LogOut } from "lucide-react";
import { Button } from "@/components/ui/button";

interface TopBarProps {
  onToggleSidebar?: () => void;
}

export function TopBar({ onToggleSidebar }: TopBarProps) {
  const { user, displayName, logout } = useAuth();

  return (
    <header className="fixed top-0 left-0 right-0 z-50 h-14 bg-card border-b border-border flex items-center px-4 gap-4">
      {/* Left: menu + brand */}
      <Button
        variant="ghost"
        size="icon"
        className="h-9 w-9 shrink-0"
        onClick={onToggleSidebar}
      >
        <Menu className="w-5 h-5" />
      </Button>
      <span className="text-[15px] font-semibold tracking-tight select-none">
        Edd Cloud
      </span>

      {/* Spacer */}
      <div className="flex-1" />

      {/* Right: actions */}
      <div className="flex items-center gap-1">
        <ThemeToggle />
        {user && <NotificationBell />}
        {user && (
          <>
            <div className="w-px h-6 bg-border mx-2" />
            <div className="flex items-center gap-2">
              <div className="w-7 h-7 rounded-full bg-primary/20 text-primary flex items-center justify-center text-xs font-semibold">
                {(displayName || user).charAt(0).toUpperCase()}
              </div>
              <span className="text-sm font-medium hidden sm:block">
                {displayName || user}
              </span>
              <Button
                variant="ghost"
                size="icon"
                className="h-8 w-8 text-muted-foreground hover:text-foreground"
                onClick={logout}
                title="Sign out"
              >
                <LogOut className="w-4 h-4" />
              </Button>
            </div>
          </>
        )}
      </div>
    </header>
  );
}
