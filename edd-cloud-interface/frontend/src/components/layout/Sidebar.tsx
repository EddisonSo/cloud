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
                    "flex items-center gap-3 w-full px-5 py-2 text-[13.5px] transition-colors duration-150 text-left border-l-2 border-transparent",
                    "text-muted-foreground hover:text-foreground hover:bg-popover",
                    isActive && "text-foreground",
                  )}
                >
                  <Icon className="w-[17px] h-[17px] shrink-0 opacity-60" />
                  <span className="flex-1">{item.label}</span>
                  <ChevronDown
                    className={cn(
                      "w-3.5 h-3.5 transition-transform duration-150 opacity-40",
                      isExpanded && "rotate-180",
                    )}
                  />
                </button>

                {/* Sub-items hang on a hairline rail; active segment becomes an ice tick */}
                {isExpanded && (
                  <div className="mt-0.5 mb-1">
                    {item.subItems!.map((subItem) => {
                      const isSubActive = location.pathname === subItem.path;
                      return (
                        <NavLink
                          key={subItem.id}
                          to={subItem.path}
                          onClick={() => { if (window.innerWidth < 768) onClose?.(); }}
                          className={cn(
                            "flex items-center pl-[46px] pr-5 py-1.5 text-[12.5px] transition-colors duration-150 relative",
                            "text-faint hover:text-foreground",
                            isSubActive && "text-foreground",
                          )}
                        >
                          <span
                            className={cn(
                              "absolute left-[26px] top-0 bottom-0 w-px bg-border",
                              isSubActive && "w-[2px] bg-primary left-[25.5px]",
                            )}
                          />
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
              onClick={() => { if (window.innerWidth < 768) onClose?.(); }}
              className={cn(
                "flex items-center gap-3 px-5 py-2 text-[13.5px] transition-colors duration-150 border-l-2 border-transparent",
                "text-muted-foreground hover:text-foreground hover:bg-popover",
                isActive && "text-foreground border-primary bg-popover",
              )}
            >
              <Icon className="w-[17px] h-[17px] shrink-0 opacity-60" />
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
