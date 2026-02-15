import { useState, useEffect } from "react";
import { NavLink, useLocation } from "react-router-dom";
import { cn } from "@/lib/utils";
import { useAuth } from "@/contexts/AuthContext";
import { NAV_ITEMS, ADMIN_NAV_ITEM } from "@/lib/constants";
import { StatusDot } from "@/components/common";
import { ChevronDown } from "lucide-react";
import type { NavItem } from "@/types";

interface SidebarProps {
  healthOk?: boolean;
  collapsed?: boolean;
  onClose?: () => void;
}

export function Sidebar({ healthOk = true, collapsed = false, onClose }: SidebarProps) {
  const location = useLocation();
  const { isAdmin } = useAuth();
  const [expandedSections, setExpandedSections] = useState<Set<string>>(new Set());

  const navItems: NavItem[] = isAdmin ? [...NAV_ITEMS, ADMIN_NAV_ITEM] : NAV_ITEMS;

  // Auto-expand active section
  useEffect(() => {
    for (const item of navItems) {
      if (item.subItems && location.pathname.startsWith(item.path)) {
        setExpandedSections((prev) => new Set(prev).add(item.id));
      }
    }
  }, [location.pathname]);

  const toggleSection = (id: string) => {
    setExpandedSections((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  if (collapsed) return null;

  return (
    <>
      {/* Mobile backdrop */}
      <div
        className="fixed inset-0 bg-black/50 z-40 md:hidden"
        onClick={onClose}
      />
    <aside className="fixed top-14 left-0 bottom-0 w-[240px] bg-card border-r border-border overflow-y-auto z-50 md:z-40">
      <nav className="flex flex-col py-3">
        {navItems.map((item) => {
          const Icon = item.icon;
          const isActive = location.pathname.startsWith(item.path);
          const hasSubItems = item.subItems && item.subItems.length > 0;
          const isExpanded = expandedSections.has(item.id);

          if (hasSubItems) {
            return (
              <div key={item.id}>
                {/* Section header */}
                <button
                  onClick={() => toggleSection(item.id)}
                  className={cn(
                    "flex items-center gap-3 w-full px-5 py-2 text-[13px] font-medium transition-colors text-left",
                    "text-muted-foreground hover:text-foreground hover:bg-accent/50",
                    isActive && "text-foreground",
                  )}
                >
                  <Icon className="w-[18px] h-[18px] shrink-0 opacity-70" />
                  <span className="flex-1">{item.label}</span>
                  <ChevronDown
                    className={cn(
                      "w-3.5 h-3.5 transition-transform opacity-50",
                      isExpanded && "rotate-180",
                    )}
                  />
                </button>

                {/* Sub-items */}
                {isExpanded && (
                  <div className="mt-0.5 mb-1">
                    {item.subItems!.map((subItem) => {
                      const isSubActive = location.pathname === subItem.path;
                      return (
                        <NavLink
                          key={subItem.id}
                          to={subItem.path}
                          onClick={onClose}
                          className={cn(
                            "flex items-center gap-3 pl-12 pr-5 py-1.5 text-[13px] transition-colors relative",
                            "text-muted-foreground hover:text-foreground hover:bg-accent/50",
                            isSubActive && "text-primary bg-primary/5 font-medium",
                          )}
                        >
                          {isSubActive && (
                            <span className="absolute left-0 top-1/2 -translate-y-1/2 w-[3px] h-5 bg-primary rounded-r" />
                          )}
                          {subItem.label}
                        </NavLink>
                      );
                    })}
                  </div>
                )}
              </div>
            );
          }

          return (
            <NavLink
              key={item.id}
              to={item.path}
              onClick={onClose}
              className={cn(
                "flex items-center gap-3 px-5 py-2 text-[13px] font-medium transition-colors relative",
                "text-muted-foreground hover:text-foreground hover:bg-accent/50",
                isActive && "text-primary bg-primary/5",
              )}
            >
              {isActive && (
                <span className="absolute left-0 top-1/2 -translate-y-1/2 w-[3px] h-5 bg-primary rounded-r" />
              )}
              <Icon className="w-[18px] h-[18px] shrink-0 opacity-70" />
              {item.label}
              {item.id === "health" && (
                <StatusDot status={healthOk ? "ok" : "down"} className="ml-auto" />
              )}
            </NavLink>
          );
        })}
      </nav>
    </aside>
    </>
  );
}
