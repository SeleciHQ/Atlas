import Link from "next/link";
import { cn } from "@/lib/utils";
import { AppNav } from "./app-nav";
import { SearchButton } from "./search-dialog";
import { UserMenu } from "./user-menu";
import { OrgSwitcher } from "./org-switcher";

interface AppShellProps {
  children: React.ReactNode;
  searchIndex: { label: string; sub: string; href: string; kind: string }[];
  session: { name: string; role: string; demoRole: string };
  counts?: { work?: number; approvals?: number };
}

export function AppShell({ children, searchIndex, session, counts }: AppShellProps) {
  return (
    <div className="flex min-h-dvh">
      {/* Sidebar */}
      <aside className="fixed inset-y-0 left-0 z-30 hidden w-[248px] flex-col border-r border-sidebar-border bg-sidebar lg:flex">
        {/* Wordmark */}
        <div className="flex h-14 items-center gap-2.5 px-4">
          <Link href="/work" className="flex items-center gap-2.5" aria-label="Atlas home">
            <span className="flex h-7 w-7 items-center justify-center rounded-md bg-brand text-brand-foreground">
              <svg width="13" height="13" viewBox="0 0 16 16" fill="none" aria-hidden="true">
                <path d="M8 1.5L14.5 14H1.5L8 1.5Z" stroke="currentColor" strokeWidth="1.75" strokeLinejoin="round" />
              </svg>
            </span>
            <span className="text-[15px] font-semibold tracking-tight text-foreground">Atlas</span>
          </Link>
        </div>

        {/* Org switcher */}
        <div className="px-3 py-2">
          <OrgSwitcher />
        </div>

        {/* Primary nav */}
        <AppNav counts={counts} />

        {/* Settings (separate at the bottom per spec) */}
        <div className="px-3 pb-1">
          <SettingsLink />
        </div>

        {/* Search + user menu */}
        <div className="flex flex-col gap-0.5 border-t border-sidebar-border p-3">
          <SearchButton index={searchIndex} />
          <UserMenu name={session.name} role={session.role} demoRole={session.demoRole} />
        </div>
      </aside>

      {/* Main column */}
      <div className="flex min-h-dvh flex-1 lg:pl-[248px]">
        <main className="mx-auto w-full max-w-[1280px] px-8 py-8 lg:px-10">{children}</main>
      </div>
    </div>
  );
}

function SettingsLink() {
  // Lightweight client-only active trigger; pathname hook
  return (
    <Link
      href="/settings"
      className={cn(
        "flex h-9 items-center gap-2.5 rounded-md px-2.5 text-[13px] font-medium text-sidebar-foreground hover:bg-surface-subtle hover:text-foreground"
      )}
    >
      <svg className="h-[18px] w-[18px] text-[#8f97a3]" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
        <path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.18.09a2 2 0 0 1-2 0L6.4 5.83a2 2 0 0 0-2.83 0l-.31.31a2 2 0 0 0 0 2.83l.09.09a2 2 0 0 1 0 2l.09.18a2 2 0 0 0 1 1.73l.18.09a2 2 0 0 1 2 0l.18-.09a2 2 0 0 1 2 0L12.22 16H12.7a2 2 0 0 1 2-2v-.18a2 2 0 0 1 1-1.73l.18-.09a2 2 0 0 1 2 0l.09.09a2 2 0 0 1 2.83 0l.31.31a2 2 0 0 0 0-2.83l-.09-.09a2 2 0 0 1 0-2l-.09-.18a2 2 0 0 0-1-1.73l-.18-.09a2 2 0 0 1-2 0l-.18.09a2 2 0 0 1-2 0L12.22 2Z"/>
        <circle cx="12" cy="12" r="3"/>
      </svg>
      Settings
    </Link>
  );
}

export function PageHeader({
  title,
  description,
  actions,
  breadcrumbs,
  backHref,
  backLabel,
}: {
  title: React.ReactNode;
  description?: React.ReactNode;
  actions?: React.ReactNode;
  breadcrumbs?: { label: string; href: string }[];
  backHref?: string;
  backLabel?: string;
}) {
  return (
    <header className="mb-7">
      {breadcrumbs && (
        <nav aria-label="Breadcrumb" className="mb-2.5 flex items-center gap-1.5 text-[12px] text-muted-foreground">
          {breadcrumbs.map((c, i) => (
            <span key={i} className="flex items-center gap-1.5">
              <Link href={c.href} className="hover:text-foreground">
                {c.label}
              </Link>
              {i < breadcrumbs.length - 1 && <span aria-hidden="true">/</span>}
            </span>
          ))}
        </nav>
      )}
      {backHref && backLabel && (
        <Link href={backHref} className="mb-2.5 inline-flex items-center gap-1 text-[12px] text-muted-foreground hover:text-foreground">
          <span aria-hidden="true">&larr;</span> {backLabel}
        </Link>
      )}
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div className="min-w-0">
          <h1 className="text-[22px] font-semibold leading-7 tracking-[-0.01em] text-foreground">{title}</h1>
          {description && <p className="mt-1.5 text-[13px] text-muted-foreground">{description}</p>}
        </div>
        {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
      </div>
    </header>
  );
}

export { cn };
