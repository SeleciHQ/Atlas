import { getSettingsData } from "@/lib/queries";
import { SettingsView } from "@/components/settings/settings-view";
import { PageHeader } from "@/components/app-shell/app-shell";

export const dynamic = "force-dynamic";

export default async function SettingsPage() {
  const data = await getSettingsData();
  return (
    <div>
      <PageHeader
        title="Settings"
        description="Organization profile, integrations, access, governance, and preferences."
      />
      <SettingsView
        policyVersion={data.policy.version}
        rules={data.rules}
        priceTolerance={data.policy.priceTolerancePct}
        approvalThreshold={data.policy.approvalThreshold}
        requiredEvidence={data.policy.requiredEvidence}
        dbUsers={data.users}
      />
    </div>
  );
}
