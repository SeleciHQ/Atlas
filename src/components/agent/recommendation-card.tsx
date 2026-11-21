import { StatusBadge } from "@/components/shared/status-badge";
import { ReasoningPanel } from "./reasoning-panel";
import { Sparkles } from "lucide-react";

const CONFIDENCE_TONE = {
  HIGH: "success",
  MEDIUM: "warning",
  LOW: "danger",
} as const;

export function RecommendationCard({
  recommendation,
  confidence,
  evidenceCount,
  policyName,
  whySteps,
  violations,
}: {
  recommendation: string;
  confidence: string;
  evidenceCount: number;
  policyName: string;
  whySteps: string[];
  violations: { rule: string; detail: string }[];
}) {
  const tone = CONFIDENCE_TONE[confidence as keyof typeof CONFIDENCE_TONE] ?? "neutral";
  return (
    <div className="rounded-lg border border-brand/25 bg-brand-soft/40 shadow-xs">
      <div className="flex h-9 items-center justify-between border-b border-brand/15 px-4">
        <span className="flex items-center gap-1.5 text-[12px] font-semibold text-brand">
          <Sparkles className="h-3.5 w-3.5" aria-hidden="true" />
          Agent recommendation
        </span>
        <StatusBadge tone={tone}>{confidence.toLowerCase()} confidence</StatusBadge>
      </div>
      <div className="px-4 py-3.5">
        <p className="text-[14px] font-medium leading-snug text-foreground">{recommendation}</p>
        <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-[12px] text-muted-foreground">
          <span>
            Evidence: <span className="font-medium text-foreground">{evidenceCount} sources</span>
          </span>
          <span>
            Policy: <span className="font-medium text-foreground">{policyName}</span>
          </span>
        </div>

        {whySteps.length > 0 && (
          <div className="mt-3 border-t border-brand/15 pt-3">
            <p className="label-caps mb-2 text-muted-foreground">Why</p>
            <ol className="flex flex-col gap-1.5">
              {whySteps.map((step, i) => (
                <li key={i} className="flex gap-2.5 text-[12.5px] leading-relaxed text-foreground">
                  <span className="num w-4 shrink-0 text-muted-foreground">{i + 1}.</span>
                  <span>{step}</span>
                </li>
              ))}
            </ol>
          </div>
        )}

        {violations.length > 0 && (
          <div className="mt-3 flex flex-col gap-2">
            <p className="label-caps text-muted-foreground">Rule violations</p>
            {violations.map((v) => (
              <div key={v.rule} className="rounded-md border border-danger/20 bg-danger-bg px-3 py-2">
                <p className="text-[12.5px] font-medium text-danger">{titleize(v.rule)}</p>
                <p className="mt-0.5 text-[12.5px] text-muted-foreground">{v.detail}</p>
              </div>
            ))}
          </div>
        )}

        <ReasoningPanel steps={whySteps} />
      </div>
    </div>
  );
}

function titleize(rule: string): string {
  return rule
    .split("_")
    .map((w) => w.charAt(0).toUpperCase() + w.slice(1))
    .join(" ");
}
