import * as React from "react";
import { cn } from "@/lib/utils";

/**
 * StatusBadge — Atlas's consistent semantic status indicator.
 * Renders a small dot + label in one of: neutral, brand, success, warning, danger, info.
 * Not a shadcn Badge (which is heavier); this is deliberately inline and compact.
 */

export type StatusTone =
  | "neutral"
  | "brand"
  | "success"
  | "warning"
  | "danger"
  | "info";

const TONE_STYLES: Record<StatusTone, { container: string; dot: string }> = {
  neutral: {
    container: "border-border bg-surface-subtle text-muted-foreground",
    dot: "bg-[#8f97a3]",
  },
  brand: {
    container: "border-brand/20 bg-brand-soft text-brand-soft-foreground",
    dot: "bg-brand",
  },
  success: {
    container: "border-success/20 bg-success-bg text-success",
    dot: "bg-success",
  },
  warning: {
    container: "border-warning/20 bg-warning-bg text-warning",
    dot: "bg-warning",
  },
  danger: {
    container: "border-danger/20 bg-danger-bg text-danger",
    dot: "bg-danger",
  },
  info: {
    container: "border-info/20 bg-info-bg text-info",
    dot: "bg-info",
  },
};

export function StatusBadge({
  children,
  tone = "neutral",
  className,
  dot = true,
}: {
  children: React.ReactNode;
  tone?: StatusTone;
  className?: string;
  dot?: boolean;
}) {
  const s = TONE_STYLES[tone];
  return (
    <span
      className={cn(
        "inline-flex h-[22px] items-center gap-1.5 rounded-md border px-1.5 text-[11.5px] font-medium leading-none",
        s.container,
        className
      )}
    >
      {dot && (
        <span
          className={cn("h-1.5 w-1.5 shrink-0 rounded-full", s.dot)}
          aria-hidden="true"
        />
      )}
      <span className="leading-none">{children}</span>
    </span>
  );
}
