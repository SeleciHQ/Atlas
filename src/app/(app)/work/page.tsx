import { getWorkData } from "@/lib/queries";
import { WorkView, JeanApprovalView } from "@/components/work/work-view";

export const dynamic = "force-dynamic";

export default async function WorkPage() {
  const data = await getWorkData();
  if (data.user.role === "CONTROLLER") {
    return <JeanApprovalView approvals={data.approvals} totalProposed={data.totalProposed} name={data.user.name} />;
  }
  return (
    <WorkView
      groups={data.groups}
      stats={{ open: data.stats.open, needsDecision: data.stats.needsDecision }}
      role={data.user.role}
      name={data.user.name}
    />
  );
}
