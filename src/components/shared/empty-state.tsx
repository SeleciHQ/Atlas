import * as React from "react";
import { FileSearch } from "lucide-react";
import { cn } from "@/lib/utils";

export function EmptyState({
  title,
  description,
  icon = FileSearch,
  action,
  className,
}: {
  title: string;
  description?: string;
  icon?: React.ElementType;
  action?: React.ReactNode;
  className?: string;
}) {
  const Icon = icon;
  return (
    <div
      className={cn(
        "flex flex-col items-center justify-center gap-2.5 rounded-lg border border-dashed border-border bg-surface px-6 py-12 text-center",
        className
      )}
    >
      <div className="flex h-9 w-9 items-center justify-center rounded-md bg-surface-subtle text-muted-foreground">
        <Icon className="h-4 w-4" strokeWidth={1.5} />
      </div>
      <h3 className="text-[13px] font-semibold text-foreground">{title}</h3>
      {description && <p className="max-w-md text-[12.5px] text-muted-foreground">{description}</p>}
      {action && <div className="mt-1.5">{action}</div>}
    </div>
  );
}
