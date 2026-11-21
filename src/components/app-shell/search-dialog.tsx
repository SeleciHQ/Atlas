"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { Search } from "lucide-react";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";

interface SearchItem {
  label: string;
  sub: string;
  href: string;
  kind: string;
}

export function SearchButton({ index }: { index: SearchItem[] }) {
  const router = useRouter();
  const [open, setOpen] = React.useState(false);
  const [query, setQuery] = React.useState("");

  React.useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setOpen((o) => !o);
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const results = React.useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return index.slice(0, 8);
    return index.filter((i) => `${i.label} ${i.sub}`.toLowerCase().includes(q)).slice(0, 10);
  }, [index, query]);

  return (
    <>
      <button
        type="button"
        onClick={() => setOpen(true)}
        className="flex w-full items-center gap-2 rounded-md px-2 py-2 text-[13px] text-sidebar-foreground transition-colors hover:bg-surface-subtle hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/30"
        aria-label="Search cases, invoices and suppliers"
      >
        <Search className="h-4 w-4 text-[#8f97a3]" aria-hidden="true" />
        <span>Search</span>
        <kbd className="ml-auto rounded border border-sidebar-border px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground">
          ⌘K
        </kbd>
      </button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="top-[20%] translate-y-0 gap-0 p-0 sm:max-w-lg">
          <DialogHeader className="border-b px-4 py-3">
            <DialogTitle className="text-sm">Search</DialogTitle>
            <DialogDescription className="sr-only">Find cases, invoices, suppliers and approvals</DialogDescription>
          </DialogHeader>
          <input
            autoFocus
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search invoices, suppliers, approvals…"
            aria-label="Search query"
            className="w-full border-b bg-transparent px-4 py-3 text-sm outline-none placeholder:text-muted-foreground"
          />
          <ul className="max-h-80 overflow-y-auto p-2" role="listbox" aria-label="Search results">
            {results.length === 0 && (
              <li className="px-3 py-6 text-center text-sm text-muted-foreground">No results for “{query}”</li>
            )}
            {results.map((item) => (
              <li key={`${item.kind}-${item.label}`}>
                <button
                  type="button"
                  onClick={() => {
                    setOpen(false);
                    setQuery("");
                    router.push(item.href);
                  }}
                  className="flex w-full items-center justify-between gap-3 rounded-md px-3 py-2 text-left text-[13px] transition-colors hover:bg-brand-soft"
                >
                  <span className="flex flex-col min-w-0">
                    <span className="truncate font-medium">{item.label}</span>
                    <span className="truncate text-xs text-muted-foreground">{item.sub}</span>
                  </span>
                  <span className="shrink-0 text-[11px] uppercase tracking-wide text-muted-foreground">{item.kind}</span>
                </button>
              </li>
            ))}
          </ul>
        </DialogContent>
      </Dialog>
    </>
  );
}
