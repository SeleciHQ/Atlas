import { StatusBadge } from "@/components/shared/status-badge";
import { ShieldCheck } from "lucide-react";

export function PolicyCard({
  policyVersion,
  rules,
  approveEvaluation,
  amountText,
}: {
  policyVersion: string;
  rules: string[];
  approveEvaluation: { allowed: boolean; requiresApproval: boolean; blockedReason: string | null };
  amountText: string;
}) {
  return (
    <div className="rounded-lg border border-border bg-surface shadow-xs">
      <div className="flex h-9 items-center justify-between border-b px-4">
        <span className="flex items-center gap-1.5 text-[12px] font-semibold text-foreground">
          <ShieldCheck className="h-3.5 w-3.5 text-brand" aria-hidden="true" />
          Policy
        </span>
        <span className="font-mono text-[11px] text-muted-foreground">{policyVersion}</span>
      </div>
      <div className="px-4 py-3.5">
        <ul className="flex flex-col gap-1.5">
          {rules.map((rule, i) => (
            <li key={i} className="flex gap-2 text-[12.5px] leading-relaxed text-muted-foreground">
              <span className="mt-[7px] h-1 w-1 shrink-0 rounded-full bg-[#b9c0ca]" aria-hidden="true" />
              <span>{rule}</span>
            </li>
          ))}
        </ul>
        <div className="mt-3">
          {approveEvaluation.allowed ? (
            <StatusBadge tone={approveEvaluation.requiresApproval ? "brand" : "success"}>
              {approveEvaluation.requiresApproval
                ? `Approve ${amountText}: allowed — requires controller approval`
                : `Approve ${amountText}: allowed — can be executed directly`}
            </StatusBadge>
          ) : (
            <StatusBadge tone="danger">Approve {amountText}: blocked by policy</StatusBadge>
          )}
          {approveEvaluation.blockedReason && (
            <p className="mt-2 text-[12.5px] leading-relaxed text-muted-foreground">{approveEvaluation.blockedReason}</p>
          )}
        </div>
      </div>
    </div>
  );
}
