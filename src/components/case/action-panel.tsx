"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  approvePaymentAction,
  escalateCaseAction,
  holdPaymentAction,
  recordCorrectedDocumentAction,
  requestDocumentAction,
} from "@/lib/actions";
import { cn } from "@/lib/utils";

type PendingAction = "request_document" | "hold_payment" | "escalate_case" | "approve_payment" | "record_corrected" | null;

const ACTION_CONFIRM: Record<
  Exclude<PendingAction, null>,
  { title: string; description: (supplierName: string, amountText: string) => string; confirmLabel: string }
> = {
  request_document: {
    title: "Send document request",
    description: (s) => `Send a document request to ${s}? The supplier will be asked to reissue a corrected invoice.`,
    confirmLabel: "Send request",
  },
  hold_payment: {
    title: "Hold payment",
    description: (s, a) => `Hold the payment of ${a} to ${s}? Payment will be blocked in the ERP until this case is resolved.`,
    confirmLabel: "Hold payment",
  },
  escalate_case: {
    title: "Escalate to procurement",
    description: () => "Escalate this case to the procurement team? They will verify the purchase order and supplier terms.",
    confirmLabel: "Escalate",
  },
  approve_payment: {
    title: "Approve payment",
    description: (s, a) => `Approve payment of ${a} to ${s}? This will release payment in the mock ERP and create an immutable approval record.`,
    confirmLabel: "Approve payment",
  },
  record_corrected: {
    title: "Record corrected invoice",
    description: (s) => `Record that ${s} sent a corrected invoice? Discrepancies will be marked resolved and policy will be re-evaluated.`,
    confirmLabel: "Record",
  },
};

export function CaseActionPanel({
  caseId,
  status,
  supplierName,
  amountText,
  approveBlockedReason,
  requiresApproval,
  isController,
  onActionSuccess,
}: {
  caseId: string;
  status: string;
  supplierName: string;
  amountText: string;
  approveBlockedReason: string | null;
  requiresApproval: boolean;
  isController: boolean;
  onActionSuccess?: () => void;
}) {
  const router = useRouter();
  const [pendingAction, setPendingAction] = React.useState<PendingAction>(null);
  const [busy, setBusy] = React.useState(false);
  const resolved = status === "RESOLVED";
  const waitingSupplier = status === "WAITING_SUPPLIER";

  async function execute(action: Exclude<PendingAction, null>) {
    setBusy(true);
    try {
      const result =
        action === "request_document"
          ? await requestDocumentAction(caseId)
          : action === "hold_payment"
            ? await holdPaymentAction(caseId)
            : action === "escalate_case"
              ? await escalateCaseAction(caseId)
              : action === "approve_payment"
                ? await approvePaymentAction(caseId)
                : await recordCorrectedDocumentAction(caseId);
      if (result.ok) {
        toast.success(result.message);
        setPendingAction(null);
        onActionSuccess?.();
        if (result.redirect) router.push(result.redirect);
        else router.refresh();
      } else {
        toast.error(result.message, { description: "No changes were made." });
      }
    } finally {
      setBusy(false);
    }
  }

  const blocked = pendingAction !== null;
  const confirm = pendingAction ? ACTION_CONFIRM[pendingAction] : null;

  return (
    <div className="flex flex-col gap-2.5">
      {!resolved && waitingSupplier && (
        <Button
          variant="secondary"
          disabled={blocked || busy}
          onClick={() => setPendingAction("record_corrected")}
          className="justify-start"
        >
          Mark corrected invoice as received
        </Button>
      )}
      <Button
        variant="outline"
        disabled={blocked || busy || resolved}
        onClick={() => setPendingAction("request_document")}
        className="justify-start"
      >
        Request corrected invoice
      </Button>
      <Button
        variant="outline"
        disabled={blocked || busy || resolved}
        onClick={() => setPendingAction("hold_payment")}
        className="justify-start"
      >
        Hold payment
      </Button>
      <Button
        variant="outline"
        disabled={blocked || busy || resolved}
        onClick={() => setPendingAction("escalate_case")}
        className="justify-start"
      >
        Escalate to procurement
      </Button>

      <div className="mt-1 flex flex-col gap-1.5">
        <Button
          disabled={Boolean(approveBlockedReason) || blocked || busy || resolved}
          onClick={() => setPendingAction("approve_payment")}
          className={cn("justify-start", approveBlockedReason && "opacity-60")}
          title={approveBlockedReason ?? undefined}
          aria-describedby={approveBlockedReason ? "approve-blocked-reason" : undefined}
        >
          Approve payment
          {requiresApproval && !approveBlockedReason && (
            <span className="ml-1 text-xs font-normal opacity-80">· routes to controller</span>
          )}
        </Button>
        {approveBlockedReason && (
          <p id="approve-blocked-reason" className="text-xs text-destructive/90">
            Blocked by policy: {approveBlockedReason}
          </p>
        )}
        {!approveBlockedReason && !isController && requiresApproval && (
          <p className="text-xs text-muted-foreground">
            Policy requires controller approval for payments above the $5,000 threshold.
          </p>
        )}
      </div>

      {resolved && (
        <p className="text-xs text-muted-foreground">This case is resolved. No further actions are available.</p>
      )}

      <Dialog open={pendingAction !== null} onOpenChange={(o) => !o && !busy && setPendingAction(null)}>
        <DialogContent className="sm:max-w-md">
          {confirm && (
            <>
              <DialogHeader>
                <DialogTitle>{confirm.title}</DialogTitle>
                <DialogDescription>
                  {confirm.description(supplierName, amountText)}
                </DialogDescription>
              </DialogHeader>
              <DialogFooter className="gap-2">
                <Button variant="outline" onClick={() => setPendingAction(null)} disabled={busy}>
                  Cancel
                </Button>
                <Button
                  onClick={() => pendingAction && execute(pendingAction)}
                  disabled={busy}
                  variant={pendingAction === "escalate_case" || pendingAction === "hold_payment" ? "destructive" : "default"}
                >
                  {busy ? "Working…" : confirm.confirmLabel}
                </Button>
              </DialogFooter>
            </>
          )}
        </DialogContent>
      </Dialog>
    </div>
  );
}
