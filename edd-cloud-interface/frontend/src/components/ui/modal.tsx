import React, { useEffect, useRef } from "react";
import { cn } from "@/lib/utils";
import { X } from "lucide-react";
import { Button } from "./button";

interface ModalProps {
  open: boolean;
  onClose: () => void;
  title: string;
  description?: string;
  children: React.ReactNode;
  footer?: React.ReactNode;
  size?: "sm" | "md" | "lg";
  className?: string;
}

const sizeClasses = {
  sm: "max-w-[400px]",
  md: "max-w-[520px]",
  lg: "max-w-[680px]",
};

export function Modal({
  open,
  onClose,
  title,
  description,
  children,
  footer,
  size = "md",
  className,
}: ModalProps) {
  const overlayRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const handleEsc = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", handleEsc);
    document.body.style.overflow = "hidden";
    return () => {
      document.removeEventListener("keydown", handleEsc);
      document.body.style.overflow = "";
    };
  }, [open, onClose]);

  if (!open) return null;

  return (
    <div
      ref={overlayRef}
      className="fixed inset-0 z-[100] flex items-center justify-center p-4"
      onClick={(e) => {
        if (e.target === overlayRef.current) onClose();
      }}
    >
      {/* Flat overlay — no blur, no gradient */}
      <div className="absolute inset-0 bg-black/60" />

      {/* Dialog panel — flat card, 1px border, no radius, no shadow */}
      <div
        className={cn(
          "relative w-full bg-card border border-border",
          sizeClasses[size],
          className,
        )}
      >
        {/* Header */}
        <div className="flex items-start justify-between px-6 pt-5 pb-0">
          <div>
            <h2 className="text-[14px] font-semibold leading-snug tracking-[-0.01em]">
              {title}
            </h2>
            {description && (
              <p className="text-[13px] text-muted-foreground mt-1">
                {description}
              </p>
            )}
          </div>
          <Button
            variant="ghost"
            size="icon"
            className="h-8 w-8 -mr-2 -mt-1 text-muted-foreground hover:text-foreground shrink-0"
            onClick={onClose}
          >
            <X className="w-4 h-4" />
          </Button>
        </div>

        {/* Body */}
        <div className="px-6 py-4 max-h-[60vh] overflow-y-auto">
          {children}
        </div>

        {/* Footer */}
        {footer && (
          <div className="flex items-center justify-end gap-2 px-6 pb-5 pt-2 border-t border-border">
            {footer}
          </div>
        )}
      </div>
    </div>
  );
}
