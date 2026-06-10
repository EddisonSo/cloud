import { Link } from "react-router-dom";
import { cn } from "@/lib/utils";

export interface BreadcrumbItem {
  label: string;
  href?: string;
}

export function Breadcrumb({ items, className }: { items: BreadcrumbItem[]; className?: string }) {
  return (
    <nav
      className={cn(
        "flex items-center gap-2 font-mono text-[10.5px] font-medium uppercase tracking-[0.16em] mb-4",
        className
      )}
    >
      {items.map((item, i) => {
        const isLast = i === items.length - 1;
        return (
          <span key={i} className="flex items-center gap-2">
            {i > 0 && <span className="text-faint select-none">/</span>}
            {isLast || !item.href ? (
              <span className={cn(isLast ? "text-muted-foreground" : "text-faint")}>
                {item.label}
              </span>
            ) : (
              <Link
                to={item.href}
                className="text-faint hover:text-foreground transition-colors duration-150"
              >
                {item.label}
              </Link>
            )}
          </span>
        );
      })}
    </nav>
  );
}
