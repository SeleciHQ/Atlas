"use client";

import * as React from "react";
import { ChevronsUpDown, Check } from "lucide-react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

const ORGS = [{ value: "acme-logistics", label: "Acme Logistics", sub: "Finance Operations" }];

export function OrgSwitcher() {
  const [org, setOrg] = React.useState("acme-logistics");
  const current = ORGS.find((o) => o.value === org)!;
  return (
    <DropdownMenu>
      <DropdownMenuTrigger className="inline-flex w-full items-center justify-between gap-2 rounded-md border border-input bg-surface px-2.5 py-2 text-left shadow-xs hover:bg-surface-subtle focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/30">
        <span className="flex min-w-0 flex-col leading-tight">
          <span className="truncate text-[13px] font-medium text-foreground">{current.label}</span>
          <span className="truncate text-[11px] text-muted-foreground">{current.sub}</span>
        </span>
        <ChevronsUpDown className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
      </DropdownMenuTrigger>
      <DropdownMenuContent className="w-[224px]" side="bottom" align="start">
        <DropdownMenuLabel className="text-[11px] uppercase tracking-wider text-muted-foreground">
          Organization
        </DropdownMenuLabel>
        <DropdownMenuRadioGroup value={org} onValueChange={setOrg}>
          {ORGS.map((o) => (
            <DropdownMenuRadioItem key={o.value} value={o.value}>
              <span className="flex flex-1 flex-col">
                <span className="font-medium">{o.label}</span>
                <span className="text-[12px] text-muted-foreground">{o.sub}</span>
              </span>
              {o.value === org && <Check className="h-4 w-4 text-brand" />}
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
        <div className="border-t px-3 py-2 text-[11px] text-muted-foreground">
          Single-organization demo workspace
        </div>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
