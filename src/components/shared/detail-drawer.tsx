"use client";

import * as React from "react";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { cn } from "@/lib/utils";

interface DetailDrawerProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: React.ReactNode;
  eyebrow?: React.ReactNode;
  description?: React.ReactNode;
  children: React.ReactNode;
  footer?: React.ReactNode;
  width?: "md" | "lg";
}

export function DetailDrawer({
  open,
  onOpenChange,
  title,
  eyebrow,
  description,
  children,
  footer,
  width = "md",
}: DetailDrawerProps) {
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side="right"
        className={cn(
          "flex w-full flex-col gap-0 p-0 data-[side=right]:border-border",
          width === "lg" ? "sm:max-w-xl" : "sm:max-w-md"
        )}
      >
        <SheetHeader className="gap-1.5 border-b bg-surface px-5 py-4">
          {eyebrow && (
            <div className="label-caps text-muted-foreground">{eyebrow}</div>
          )}
          <SheetTitle className="text-[15px] font-semibold leading-snug">{title}</SheetTitle>
          {description && (
            <SheetDescription className="text-[12.5px] text-muted-foreground">{description}</SheetDescription>
          )}
        </SheetHeader>
        <div className="flex-1 overflow-y-auto scroll-thin px-5 py-4">{children}</div>
        {footer && <div className="border-t bg-surface-subtle px-5 py-3">{footer}</div>}
      </SheetContent>
    </Sheet>
  );
}
