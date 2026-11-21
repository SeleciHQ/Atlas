import { getActivityEvents } from "@/lib/queries";
import { ActivityTimeline } from "@/components/activity/activity-timeline";
import { PageHeader } from "@/components/app-shell/app-shell";

export const dynamic = "force-dynamic";

export default async function ActivityPage() {
  const events = await getActivityEvents();
  return (
    <div>
      <PageHeader
        title="Activity"
        description="The organization's chronological audit trail — every agent, human, and system event."
      />
      <ActivityTimeline events={events} />
    </div>
  );
}
