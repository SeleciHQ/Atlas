"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
} from "@/components/ui/dropdown-menu";
import { UserRoundIcon, ChevronsUpDownIcon } from "lucide-react";
import { switchRoleAction } from "@/lib/actions";
import { cn } from "@/lib/utils";

const ROLES: { value: string; label: string; description: string; initials: string }[] = [
  { value: "amina", label: "Amina Diallo", description: "Finance Ops", initials: "AD" },
  { value: "jean", label: "Jean Baptiste", description: "Controller", initials: "JB" },
];

export function UserMenu({ name, role, demoRole }: { name: string; role: string; demoRole: string }) {
  const router = useRouter();
  const [pending, startTransition] = React.useTransition();

  const initials = name
    .split(" ")
    .map((p) => p[0])
    .join("")
    .slice(0, 2)
    .toUpperCase();

  function switchTo(value: string) {
    if (value === demoRole) return;
    startTransition(async () => {
      const result = await switchRoleAction(value);
      if (result.ok) {
        toast.success(result.message);
        router.push("/work");
      } else {
        toast.error(result.message);
      }
    });
  }

  return (
    <DropdownMenu>
      <button
        type="button"
        data-slot="dropdown-menu-trigger"
        className={cn(
          "group inline-flex w-full items-center gap-2.5 rounded-md px-2 py-2 text-left transition-colors hover:bg-surface-subtle focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/30"
        )}
        aria-label={`User menu — signed in as ${name}`}
      >
        <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-brand-soft text-[11px] font-semibold text-brand">
          {initials}
        </span>
        <span className="flex min-w-0 flex-col leading-tight">
          <span className="truncate text-[13px] font-medium text-foreground">{name}</span>
          <span className="truncate text-[11.5px] text-muted-foreground">
            {role === "CONTROLLER" ? "Controller" : "Finance Ops"}
          </span>
        </span>
        <ChevronsUpDownIcon className="ml-auto h-3.5 w-3.5 shrink-0 text-[#9299a5]" />
      </button>
      <DropdownMenuContent side="top" align="start" className="w-64">
        <DropdownMenuLabel className="text-[11px] uppercase tracking-wider text-muted-foreground">
          Switch role
        </DropdownMenuLabel>
        <DropdownMenuRadioGroup value={demoRole} onValueChange={switchTo}>
          {ROLES.map((r) => (
            <DropdownMenuRadioItem
              key={r.value}
              value={r.value}
              disabled={pending}
              className={cn(r.value === demoRole && "font-medium")}
            >
              <span className="flex items-center gap-2">
                <UserRoundIcon className="h-3.5 w-3.5 text-muted-foreground" />
              </span>
              <span className="flex min-w-0 flex-col">
                <span>{r.label}</span>
                <span className="text-[11.5px] text-muted-foreground">{r.description}</span>
              </span>
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
        <DropdownMenuSeparator />
        <DropdownMenuItem disabled className="text-[12px] text-muted-foreground">
          Authentication disabled in the demo
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
