"use client";

import * as React from "react";
import { ChevronDown } from "lucide-react";
import { cn } from "@/lib/utils";

export function ReasoningPanel({ steps }: { steps: string[] }) {
  const [open, setOpen] = React.useState(false);
  return (
    <div className="mt-3 border-t border-brand/15 pt-3">
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        aria-expanded={open}
        className="flex items-center gap-1.5 text-[12.5px] font-medium text-muted-foreground transition-colors hover:text-foreground"
      >
        View reasoning
        <ChevronDown className={cn("h-3.5 w-3.5 transition-transform", open && "rotate-180")} aria-hidden="true" />
      </button>
      {open && (
        <ol className="mt-3 flex flex-col gap-2">
          {steps.map((step, i) => (
            <li key={i} className="flex gap-2.5 text-[12.5px] leading-relaxed text-foreground">
              <span className="num mt-px flex h-[18px] w-[18px] shrink-0 items-center justify-center rounded-full bg-white text-[10px] font-semibold text-muted-foreground ring-1 ring-border">
                {i + 1}
              </span>
              <span>{step}</span>
            </li>
          ))}
        </ol>
      )}
    </div>
  );
}
