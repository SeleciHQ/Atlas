"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  ClipboardCheck,
  FileText,
  Inbox,
  ListOrdered,
  Settings,
  Building2,
} from "lucide-react";
import { cn } from "@/lib/utils";

const MAIN_LINKS = [
  { href: "/work", label: "Work", icon: Inbox },
  { href: "/invoices", label: "Invoices", icon: FileText },
  { href: "/suppliers", label: "Suppliers", icon: Building2 },
  { href: "/approvals", label: "Approvals", icon: ClipboardCheck },
  { href: "/activity", label: "Activity", icon: ListOrdered },
];


interface NavCounts {
  work?: number;
  approvals?: number;
}

export function AppNav({ counts }: { counts?: NavCounts }) {
  const pathname = usePathname();

  function isActive(href: string) {
    if (href === "/work") return pathname === "/work" || pathname.startsWith("/work/");
    return pathname === href || pathname.startsWith(`${href}/`);
  }

  function countFor(href: string): number | undefined {
    if (!counts) return undefined;
    if (href === "/work") return counts.work;
    if (href === "/approvals") return counts.approvals;
    return undefined;
  }

  return (
    <nav aria-label="Main navigation" className="flex flex-1 flex-col gap-1 px-3 py-3">
      {MAIN_LINKS.map((link) => {
        const active = isActive(link.href);
        const count = countFor(link.href);
        return (
          <Link
            key={link.href}
            href={link.href}
            aria-current={active ? "page" : undefined}
            className={cn(
              "group flex h-9 items-center gap-2.5 rounded-md px-2.5 text-[13px] font-medium transition-colors",
              active
                ? "bg-sidebar-accent text-sidebar-accent-foreground"
                : "text-sidebar-foreground hover:bg-surface-subtle hover:text-foreground"
            )}
          >
            <link.icon
              className={cn("h-[18px] w-[18px] shrink-0 stroke-[1.75]", active ? "text-brand" : "text-[#8f97a3]")}
              strokeWidth={1.75}
              aria-hidden="true"
            />
            <span className={active ? "font-semibold" : ""}>{link.label}</span>
            {typeof count === "number" && count > 0 && (
              <span
                className={cn(
                  "ml-auto rounded px-1.5 py-0.5 text-[11px] font-semibold tabular-nums",
                  active ? "bg-white text-brand" : "bg-surface-subtle text-muted-foreground"
                )}
              >
                {count > 99 ? "99+" : count}
              </span>
            )}
          </Link>
        );
      })}
    </nav>
  );
}

export const NavIconFor = { Inbox, FileText, Building2, ClipboardCheck, ListOrdered, Settings };
