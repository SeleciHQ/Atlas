import { StatusBadge } from "@/components/shared/status-badge";
import { ActorBadge } from "@/components/shared/badges";
import { formatTime } from "@/lib/format";
import type { AuditEvent } from "@/generated/prisma/client";
import { cn } from "@/lib/utils";

function actionLabel(action: string): string {
  return action.replace(/[._]/g, " ").replace(/^./, (c) => c.toUpperCase());
}

export function CaseTimeline({ events, compact }: { events: AuditEvent[]; compact?: boolean }) {
  return (
    <ol className={cn("relative flex flex-col", compact ? "gap-0" : "gap-4")}>
      {events.map((event, i) => (
        <li key={event.id} className="relative flex gap-3 pl-1">
          <div className="flex flex-col items-center pt-[13px]">
            <span
              className={cn(
                "h-1.5 w-1.5 shrink-0 rounded-full ring-4",
                event.actorType === "HUMAN" && "bg-info ring-info/10",
                event.actorType === "AGENT" && "bg-brand ring-brand/10",
                event.actorType === "SYSTEM" && "bg-[#b9c0ca] ring-[#b9c0ca]/10"
              )}
              aria-hidden="true"
            />
            {i < events.length - 1 && <span className="mt-0.5 w-px flex-1 bg-border" aria-hidden="true" />}
          </div>
          <div className={cn("min-w-0 flex-1", compact ? "pb-2.5" : "pb-1")}>
            <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
              <p className="text-[12.5px] font-medium leading-snug text-foreground">{actionLabel(event.action)}</p>
              <ActorBadge type={event.actorType} />
              <span className="num text-[11.5px] text-muted-foreground">
                {event.actor} · {formatTime(event.createdAt)}
              </span>
            </div>
            <p className="mt-0.5 truncate text-[11.5px] text-muted-foreground">
              {event.objectType} · <span className="font-mono">{event.objectId}</span>
            </p>
          </div>
        </li>
      ))}
    </ol>
  );
}

export function CaseTimelineCard({ events }: { events: AuditEvent[] }) {
  return (
    <div className="rounded-lg border border-border bg-surface shadow-xs">
      <div className="flex h-9 items-center border-b px-4">
        <h3 className="text-[12px] font-semibold text-foreground">Case timeline</h3>
        <span className="ml-auto">
          <StatusBadge tone="neutral" dot={false}>
            {events.length} events
          </StatusBadge>
        </span>
      </div>
      <div className="px-4 py-3">
        <CaseTimeline events={events} compact />
      </div>
    </div>
  );
}
