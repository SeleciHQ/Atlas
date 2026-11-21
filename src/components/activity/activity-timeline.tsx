"use client";

import * as React from "react";
import { Search, ShieldCheck } from "lucide-react";
import { Input } from "@/components/ui/input";
import { ActorBadge } from "@/components/shared/badges";
import { StatusBadge } from "@/components/shared/status-badge";
import { EmptyState } from "@/components/shared/empty-state";
import { DetailDrawer } from "@/components/shared/detail-drawer";
import { EventInspector } from "@/components/activity/event-inspector";
import { formatTime, formatDate } from "@/lib/format";
import { cn } from "@/lib/utils";
import type { AuditEventView } from "@/types/workflow";

const FILTERS = [
  { key: "all", label: "All events" },
  { key: "HUMAN", label: "Human" },
  { key: "AGENT", label: "Agent" },
  { key: "SYSTEM", label: "System" },
  { key: "policy", label: "Policy & security" },
] as const;

function actionLabel(action: string): string {
  return action.replace(/[._]/g, " ").replace(/^./, (c) => c.toUpperCase());
}

function isFilterMatch(event: AuditEventView, filter: string): boolean {
  if (filter === "all") return true;
  if (filter === "policy") return event.action.includes("policy");
  return event.actorType === filter;
}

export function ActivityTimeline({ events }: { events: AuditEventView[] }) {
  const [filter, setFilter] = React.useState<string>("all");
  const [query, setQuery] = React.useState("");
  const [selected, setSelected] = React.useState<AuditEventView | null>(null);

  const visible = React.useMemo(() => {
    let list = events.filter((e) => isFilterMatch(e, filter));
    const q = query.trim().toLowerCase();
    if (q) {
      list = list.filter((e) =>
        `${e.actor} ${e.action} ${e.objectType} ${e.objectId}`.toLowerCase().includes(q)
      );
    }
    return list;
  }, [events, filter, query]);

  const byDay = React.useMemo(() => {
    const map = new Map<string, AuditEventView[]>();
    for (const e of visible) {
      const key = formatDate(e.createdAt).toUpperCase();
      const list = map.get(key) ?? [];
      list.push(e);
      map.set(key, list);
    }
    return Array.from(map.entries());
  }, [visible]);

  return (
    <div>
      {/* Filters */}
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <div className="flex flex-wrap items-center gap-1.5">
          {FILTERS.map((f) => (
            <button
              key={f.key}
              role="tab"
              aria-selected={filter === f.key}
              onClick={() => setFilter(f.key)}
              className={cn(
                "inline-flex h-7 items-center gap-1.5 rounded-md border px-2.5 text-[12.5px] font-medium",
                filter === f.key
                  ? "border-brand/30 bg-brand-soft text-brand"
                  : "border-border bg-surface text-muted-foreground hover:border-foreground/25 hover:text-foreground"
              )}
            >
              {f.key === "policy" && <ShieldCheck className="h-3.5 w-3.5" aria-hidden="true" />}
              {f.label}
            </button>
          ))}
        </div>
        <div className="relative ml-auto w-64">
          <Search className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" aria-hidden="true" />
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search actors, actions, objects…"
            aria-label="Search activity"
            className="h-8 pl-8 text-[12.5px]"
          />
        </div>
      </div>

      {byDay.length === 0 ? (
        <EmptyState title="No events" description={query ? `No activity matches “${query}”.` : "No activity matches this filter."} />
      ) : (
        <div className="flex flex-col gap-7">
          {byDay.map(([day, dayEvents]) => (
            <section key={day} aria-label={day}>
              <h2 className="label-caps mb-2">{day}</h2>
              <div className="overflow-hidden rounded-lg border border-border bg-surface shadow-xs">
                <ol className="divide-y divide-border">
                  {dayEvents.map((event) => (
                    <li key={event.id}>
                      <button
                        type="button"
                        onClick={() => setSelected(event)}
                        className="group flex w-full items-center gap-3 px-4 py-2.5 text-left transition-colors hover:bg-surface-subtle focus-visible:bg-brand-soft focus-visible:outline-none"
                      >
                        <span className="num w-11 shrink-0 font-mono text-[12px] text-muted-foreground">
                          {formatTime(event.createdAt)}
                        </span>
                        <span
                          className={cn(
                            "h-1.5 w-1.5 shrink-0 rounded-full",
                            event.actorType === "HUMAN" && "bg-info",
                            event.actorType === "AGENT" && "bg-brand",
                            event.actorType === "SYSTEM" && "bg-[#b9c0ca]"
                          )}
                          aria-hidden="true"
                        />
                        <span className="min-w-0 flex-1 truncate text-[13px] text-foreground">
                          <span className="font-medium">{actionLabel(event.action)}</span>
                          <span className="text-muted-foreground">
                            {" — "}
                            {event.objectType}{" "}
                            <span className="font-mono text-[12px]">{event.objectId}</span>
                          </span>
                        </span>
                        <ActorBadge type={event.actorType} />
                      </button>
                    </li>
                  ))}
                </ol>
              </div>
            </section>
          ))}
        </div>
      )}

      {/* Inspector */}
      <DetailDrawer
        open={selected !== null}
        onOpenChange={(o) => !o && setSelected(null)}
        width="md"
        eyebrow={<StatusBadge tone="neutral">Event inspection</StatusBadge>}
        title={selected ? actionLabel(selected.action) : ""}
        description={
          selected ? (
            <span>
              {formatDate(selected.createdAt)} · {formatTime(selected.createdAt)}
            </span>
          ) : null
        }
      >
        {selected && <EventInspector event={selected} />}
      </DetailDrawer>
    </div>
  );
}
