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
import { Textarea } from "@/components/ui/textarea";
import { decideApprovalAction } from "@/lib/actions";

type Decision = "APPROVE" | "REJECT" | "REQUEST_INFO";

export function ApprovalDecisionPanel({
  approvalId,
  amountText,
  supplierName,
  canDecide,
  decided,
}: {
  approvalId: string;
  amountText: string;
  supplierName: string;
  canDecide: boolean;
  decided: boolean;
}) {
  const router = useRouter();
  const [pending, setPending] = React.useState<Decision | null>(null);
  const [busy, setBusy] = React.useState(false);
  const [comment, setComment] = React.useState("");

  async function execute(decision: Decision) {
    setBusy(true);
    try {
      const result = await decideApprovalAction(approvalId, decision, comment || undefined);
      if (result.ok) {
        toast.success(result.message);
        setPending(null);
        router.push(result.redirect ?? "/approvals");
      } else {
        toast.error(result.message, { description: "No changes were made." });
      }
    } finally {
      setBusy(false);
    }
  }

  const confirmCopy: Record<Decision, { title: string; description: string; label: string }> = {
    APPROVE: {
      title: "Approve payment",
      description: `Approve payment of ${amountText} to ${supplierName}? This will release payment in the mock ERP and create an immutable approval record.`,
      label: "Approve",
    },
    REJECT: {
      title: "Reject payment",
      description: `Reject the proposed payment of ${amountText} to ${supplierName}? The invoice will be marked rejected and the case resolved.`,
      label: "Reject",
    },
    REQUEST_INFO: {
      title: "Request more information",
      description: `Request more information from ${supplierName}? The case will move back to “Waiting on supplier”.`,
      label: "Request info",
    },
  };

  return (
    <div className="flex flex-col gap-2.5">
      {!canDecide && !decided && (
        <p className="rounded-md border border-warning/20 bg-warning-bg px-3 py-2 text-[12.5px] text-warning">
          Policy requires controller approval. Switch to Jean (Controller) to decide this approval.
        </p>
      )}
      {decided ? (
        <p className="text-[13px] text-muted-foreground">This approval has already been decided.</p>
      ) : (
        <>
          <Button disabled={!canDecide || busy || pending !== null} onClick={() => setPending("APPROVE")}>
            Approve
          </Button>
          <div className="grid grid-cols-2 gap-2.5">
            <Button variant="outline" disabled={!canDecide || busy || pending !== null} onClick={() => setPending("REJECT")}>
              Reject
            </Button>
            <Button variant="outline" disabled={!canDecide || busy || pending !== null} onClick={() => setPending("REQUEST_INFO")}>
              Request more info
            </Button>
          </div>
        </>
      )}

      <Dialog open={pending !== null} onOpenChange={(o) => !o && !busy && setPending(null)}>
        <DialogContent className="sm:max-w-md">
          {pending && (
            <>
              <DialogHeader>
                <DialogTitle>{confirmCopy[pending].title}</DialogTitle>
                <DialogDescription>{confirmCopy[pending].description}</DialogDescription>
              </DialogHeader>
              {pending !== "APPROVE" && (
                <Textarea
                  value={comment}
                  onChange={(e) => setComment(e.target.value)}
                  placeholder="Optional note for the audit record…"
                  aria-label="Decision note"
                  rows={3}
                />
              )}
              <DialogFooter className="gap-2">
                <Button variant="outline" onClick={() => setPending(null)} disabled={busy}>
                  Cancel
                </Button>
                <Button
                  variant={pending === "REJECT" ? "destructive" : "default"}
                  onClick={() => execute(pending)}
                  disabled={busy}
                >
                  {busy ? "Working…" : confirmCopy[pending].label}
                </Button>
              </DialogFooter>
            </>
          )}
        </DialogContent>
      </Dialog>
    </div>
  );
}
