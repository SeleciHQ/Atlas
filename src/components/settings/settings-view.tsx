"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { StatusBadge } from "@/components/shared/status-badge";
import { SummaryBlock, CardSection } from "@/components/case/summary-blocks";
import { Switch } from "@/components/ui/switch";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { resetDemoAction } from "@/lib/actions";
import { RefreshCw } from "lucide-react";

const CONNECTIONS = [
  { name: "Mock ERP", detail: "erp.demo://finance-suite", status: "connected", synced: "2 min ago" },
  { name: "SFTP invoice folder", detail: "sftp://invoices.supplier-hub.demo/incoming", status: "connected", synced: "14 min ago" },
  { name: "Email inbox", detail: "ap-invoices@atlas.demo", status: "connected", synced: "1 min ago" },
];

export function SettingsView({
  policyVersion,
  rules,
  priceTolerance,
  approvalThreshold,
  requiredEvidence,
  dbUsers,
}: {
  policyVersion: string;
  rules: string[];
  priceTolerance: number;
  approvalThreshold: number;
  requiredEvidence: string[];
  dbUsers: { name: string; email: string; role: string }[];
}) {
  const router = useRouter();
  const [confirmOpen, setConfirmOpen] = React.useState(false);
  const [busy, setBusy] = React.useState(false);
  const [notifyExceptions, setNotifyExceptions] = React.useState(true);
  const [notifyApprovals, setNotifyApprovals] = React.useState(true);
  const [notifyDigest, setNotifyDigest] = React.useState(false);

  async function reset() {
    setBusy(true);
    try {
      const result = await resetDemoAction();
      if (result.ok) {
        toast.success(result.message);
        setConfirmOpen(false);
        router.push("/work");
      } else {
        toast.error(result.message);
      }
    } finally {
      setBusy(false);
    }
  }

  return (
    <Tabs defaultValue="organization">
      <TabsList className="mb-4">
        <TabsTrigger value="organization">Organization</TabsTrigger>
        <TabsTrigger value="integrations">Integrations</TabsTrigger>
        <TabsTrigger value="access">Access & permissions</TabsTrigger>
        <TabsTrigger value="governance">Governance</TabsTrigger>
        <TabsTrigger value="preferences">Preferences</TabsTrigger>
      </TabsList>

      <TabsContent value="organization" className="flex max-w-2xl flex-col gap-4">
        <SummaryBlock
          title="Organization profile"
          rows={[
            { label: "Name", value: "Acme Logistics" },
            { label: "Workspace ID", value: <span className="font-mono">ws_acme_logistics</span> },
            { label: "Primary function", value: "Finance operations" },
            { label: "Region", value: "EMEA" },
          ]}
        />
        <SummaryBlock
          title="Workspace preferences"
          rows={[
            { label: "Default landing section", value: "Work" },
            { label: "Theme", value: "Light" },
            { label: "Currency display", value: "USD" },
            { label: "Date format", value: "Nov 15, 2026" },
          ]}
        />
      </TabsContent>

      <TabsContent value="integrations" className="flex max-w-2xl flex-col gap-2.5">
        {CONNECTIONS.map((c) => (
          <div key={c.name} className="rounded-lg border border-border bg-surface px-5 py-4 shadow-xs">
            <div className="flex items-center justify-between gap-4">
              <div className="min-w-0">
                <p className="text-[13px] font-medium text-foreground">{c.name}</p>
                <p className="truncate font-mono text-[11.5px] text-muted-foreground">{c.detail}</p>
              </div>
              <StatusBadge tone="success">{c.status}</StatusBadge>
            </div>
            <div className="mt-2.5 flex items-center gap-2 border-t pt-2.5 text-[12px] text-muted-foreground">
              <span className="h-1.5 w-1.5 rounded-full bg-success" aria-hidden="true" />
              Last synchronized {c.synced}
            </div>
          </div>
        ))}
      </TabsContent>

      <TabsContent value="access" className="flex max-w-2xl flex-col gap-2.5">
        {dbUsers.map((u) => (
          <div key={u.email} className="flex items-center justify-between gap-4 rounded-lg border border-border bg-surface px-5 py-3.5 shadow-xs">
            <div className="min-w-0">
              <p className="text-[13px] font-medium text-foreground">{u.name}</p>
              <p className="truncate text-[12px] text-muted-foreground">{u.email}</p>
            </div>
            <StatusBadge tone={u.role === "CONTROLLER" ? "brand" : u.role === "AGENT" ? "neutral" : "info"}>
              {roleLabel(u.role)}
            </StatusBadge>
          </div>
        ))}
        <div className="rounded-lg border border-border bg-surface px-5 py-4 shadow-xs">
          <h3 className="mb-2 text-[12px] font-semibold text-foreground">Role permissions</h3>
          <ul className="flex flex-col gap-1.5 text-[12.5px] text-muted-foreground">
            <li>
              <span className="font-medium text-foreground">Finance Ops</span> — review cases, request documents, hold
              payments, escalate; propose payments for approval.
            </li>
            <li>
              <span className="font-medium text-foreground">Controller</span> — all Finance Ops permissions plus final
              payment authorization.
            </li>
            <li>
              <span className="font-medium text-foreground">Atlas Agent</span> — read evidence, propose actions; cannot
              execute payments (policy-enforced).
            </li>
          </ul>
        </div>
        <p className="text-[12px] text-muted-foreground">
          Use the role switcher at the bottom of the sidebar to preview the app as Amina or Jean.
        </p>
      </TabsContent>

      <TabsContent value="governance" className="flex max-w-2xl flex-col gap-4">
        <SummaryBlock
          title={`Active policy — ${policyVersion}`}
          rows={[
            { label: "Price tolerance", value: `${priceTolerance}%` },
            { label: "Quantity rule", value: "Invoice quantity cannot exceed received quantity" },
            { label: "Approval threshold", value: `$${approvalThreshold.toLocaleString()}` },
            { label: "Required evidence", value: requiredEvidence.join(", ") },
          ]}
          footer="Policies are versioned. Every authorization decision records the policy version that applied at decision time."
        />
        <CardSection title="Enforcement rules">
          <ul className="flex flex-col gap-1.5 px-4 py-3.5">
            {rules.map((r) => (
              <li key={r} className="flex gap-2 text-[12.5px] leading-relaxed text-muted-foreground">
                <span className="mt-[7px] h-1 w-1 shrink-0 rounded-full bg-[#b9c0ca]" aria-hidden="true" />
                {r}
              </li>
            ))}
          </ul>
        </CardSection>
        <CardSection title="Agent permissions">
          <ul className="flex flex-col gap-2 px-4 py-3.5 text-[12.5px]">
            <PermissionRow allowed label="Read evidence and operational records" />
            <PermissionRow allowed label="Request documents from suppliers" />
            <PermissionRow allowed label="Escalate cases" />
            <PermissionRow allowed label="Propose payment approval" />
            <PermissionRow allowed={false} label="Execute payment approval" />
          </ul>
        </CardSection>
      </TabsContent>

      <TabsContent value="preferences" className="flex max-w-2xl flex-col gap-2.5">
        <NotificationRow
          title="Exception notifications"
          description="Notify when a new invoice exception needs a decision."
          checked={notifyExceptions}
          onChange={setNotifyExceptions}
        />
        <NotificationRow
          title="Approval notifications"
          description="Notify when a payment approval is waiting for your decision."
          checked={notifyApprovals}
          onChange={setNotifyApprovals}
        />
        <NotificationRow
          title="Daily digest"
          description="A single summary email of open exceptions each morning."
          checked={notifyDigest}
          onChange={setNotifyDigest}
        />
      </TabsContent>

      {/* Demo reset */}
      <div className="mt-8 flex max-w-2xl items-center justify-between rounded-lg border border-dashed border-border px-5 py-4">
        <div>
          <p className="text-[13px] font-medium text-foreground">Reset demo</p>
          <p className="text-[12.5px] text-muted-foreground">
            Restore the original seed data — all cases, approvals, and audit events are regenerated.
          </p>
        </div>
        <Button variant="outline" size="sm" onClick={() => setConfirmOpen(true)} disabled={busy}>
          <RefreshCw className="h-3.5 w-3.5" aria-hidden="true" />
          Reset demo
        </Button>
      </div>

      <Dialog open={confirmOpen} onOpenChange={(o) => !o && !busy && setConfirmOpen(false)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Reset demo data?</DialogTitle>
            <DialogDescription>
              This will delete all current cases, approvals, and audit events and restore the original seed data. This
              cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter className="gap-2">
            <Button variant="outline" onClick={() => setConfirmOpen(false)} disabled={busy}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={reset} disabled={busy}>
              {busy ? "Resetting…" : "Reset demo"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Tabs>
  );
}

function PermissionRow({ label, allowed }: { label: string; allowed: boolean }) {
  return (
    <li className="flex items-center justify-between gap-3">
      <span className="text-muted-foreground">{label}</span>
      <StatusBadge tone={allowed ? "success" : "danger"}>{allowed ? "Allowed" : "Blocked"}</StatusBadge>
    </li>
  );
}

function NotificationRow({
  title,
  description,
  checked,
  onChange,
}: {
  title: string;
  description: string;
  checked: boolean;
  onChange: (v: boolean) => void;
}) {
  return (
    <div className="flex items-center justify-between gap-4 rounded-lg border border-border bg-surface px-5 py-3.5 shadow-xs">
      <div className="min-w-0">
        <p className="text-[13px] font-medium text-foreground">{title}</p>
        <p className="text-[12px] text-muted-foreground">{description}</p>
      </div>
      <Switch checked={checked} onCheckedChange={onChange} aria-label={title} />
    </div>
  );
}

function roleLabel(role: string): string {
  switch (role) {
    case "FINANCE_OPS":
      return "Finance Ops";
    case "CONTROLLER":
      return "Controller";
    case "AGENT":
      return "Automation";
    default:
      return role;
  }
}
