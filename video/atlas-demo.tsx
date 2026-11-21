import {
  Easing,
  interpolate,
  Sequence,
  staticFile,
  useCurrentFrame,
  useVideoConfig,
} from "remotion";
import { Audio } from "@remotion/media";
import {
  ArrowDownRight,
  ArrowRight,
  Building2,
  Check,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  ClipboardCheck,
  Clock3,
  FileCheck2,
  FileText,
  Inbox,
  ListOrdered,
  Search,
  Settings,
  ShieldCheck,
  Sparkles,
  Truck,
  X,
} from "lucide-react";
import type { CSSProperties, ReactNode } from "react";

const C = {
  background: "#f8f9fb",
  surface: "#ffffff",
  surfaceSubtle: "#f3f5f7",
  foreground: "#20242b",
  secondary: "#737b88",
  muted: "#9299a5",
  brand: "#3559d8",
  brandSoft: "#eef2ff",
  border: "#e8ebef",
  success: "#217e3b",
  successBg: "#ecf8f0",
  warning: "#a5630a",
  warningBg: "#fef6e5",
  danger: "#c43c3c",
  dangerBg: "#fdf0f0",
  info: "#2a63b8",
};

const APP_LEFT = 248;
const APP_WIDTH = 1280;
const frameAt = (seconds: number, fps: number) => Math.round(seconds * fps);

type Cue = { start: number; end: number; lines: string[] };
const CAPTIONS: Cue[] = [
  { start: 0, end: 3, lines: ["This is Atlas."] },
  { start: 3, end: 8, lines: ["The work queue brings invoice exceptions", "that need review into one place."] },
  { start: 8, end: 14, lines: ["INV-4821 from Kigali Metals is matched", "to PO-2291 and its delivery record."] },
  { start: 14, end: 20, lines: ["100 steel rods are invoiced.", "GRN-882 records 80 received."] },
  { start: 20, end: 27, lines: ["A 20-unit gap and 4% price variance", "exceed the 2% tolerance."] },
  { start: 27, end: 34, lines: ["The demo agent proposes a payment hold", "and a corrected invoice."] },
  { start: 34, end: 39, lines: ["Policy blocks payment until the quantity", "mismatch is resolved."] },
  { start: 39, end: 47, lines: ["A document request is ready for review.", "Outbound actions are simulated."] },
  { start: 47, end: 55, lines: ["The timeline links the recommendation,", "policy result, and action to the invoice."] },
  { start: 55, end: 60, lines: ["Kubernetes for Enterprise Reality"] },
];

const SCENES = [
  { id: "queue", start: 0, end: 7.6 },
  { id: "match", start: 7, end: 21 },
  { id: "evidence", start: 20.4, end: 28 },
  { id: "policy", start: 27.4, end: 39.5 },
  { id: "action", start: 38.9, end: 47.5 },
  { id: "activity", start: 46.9, end: 60 },
];

const CLICK_EVENTS = [
  { at: 2.65, x: 624, y: 246 },
  { at: 13.2, x: 655, y: 578 },
  { at: 22.1, x: 1024, y: 272 },
  { at: 25.35, x: 1373, y: 354 },
  { at: 38.35, x: 1460, y: 786 },
  { at: 40.4, x: 1263, y: 613 },
  { at: 49.2, x: 660, y: 240 },
];

export function AtlasDemo() {
  const frame = useCurrentFrame();
  const { fps, durationInFrames } = useVideoConfig();
  const activeCaption = CAPTIONS.find((cue) => frame >= frameAt(cue.start, fps) && frame < frameAt(cue.end, fps));
  const audioVolume = (audioFrame: number) =>
    interpolate(audioFrame, [0, fps * 2, durationInFrames - fps * 3, durationInFrames], [0, 0.8, 0.8, 0], {
      extrapolateLeft: "clamp",
      extrapolateRight: "clamp",
    });

  return (
    <div style={styles.root}>
      <Audio src={staticFile("atlas-demo-ambient.wav")} volume={audioVolume} />
      <ClickSounds fps={fps} />
      <AppShell frame={frame} fps={fps}>
        {SCENES.map((scene) => {
          const opacity = sceneOpacity(frame, frameAt(scene.start, fps), frameAt(scene.end, fps));
          if (opacity <= 0) return null;
          return (
            <div key={scene.id} style={sceneStyle(opacity, frame, frameAt(scene.start, fps))}>
              {scene.id === "queue" && <QueueScene frame={frame} />}
              {scene.id === "match" && <MatchScene frame={frame} />}
              {scene.id === "evidence" && <EvidenceScene frame={frame} />}
              {scene.id === "policy" && <PolicyScene frame={frame} />}
              {scene.id === "action" && <ActionScene frame={frame} />}
              {scene.id === "activity" && <ActivityScene frame={frame} />}
            </div>
          );
        })}
      </AppShell>
      {frame < frameAt(56, fps) && <div style={styles.previewTag}><span style={styles.previewDot} />PRODUCT WORKFLOW PREVIEW · SIMULATED AGENT &amp; ACTIONS</div>}
      {frame >= frameAt(56, fps) && <FinalSlate frame={frame} fps={fps} />}
      {activeCaption && <Caption cue={activeCaption} frame={frame} fps={fps} />}
      {frame < frameAt(55, fps) && <MousePointer frame={frame} fps={fps} />}
      <ClickFeedback frame={frame} fps={fps} />
      <ProgressBar frame={frame} durationInFrames={durationInFrames} />
    </div>
  );
}

function sceneOpacity(frame: number, start: number, end: number) {
  const fade = 12;
  const enter = interpolate(frame, [start, start + fade], [0, 1], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });
  const exit = interpolate(frame, [end - fade, end], [1, 0], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });
  return Math.min(enter, exit);
}

function sceneStyle(opacity: number, frame: number, start: number): CSSProperties {
  const entrance = interpolate(frame, [start, start + 12], [18, 0], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
    easing: Easing.out(Easing.quad),
  });
  return { ...styles.scene, opacity, transform: `translateY(${entrance}px)` };
}

function AppShell({ children, frame, fps }: { children: ReactNode; frame: number; fps: number }) {
  const avatarPulse = interpolate(frame, [frameAt(0.5, fps), frameAt(1.1, fps), frameAt(1.8, fps)], [0, 1, 0], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });
  return (
    <div style={styles.app}>
      <aside style={styles.sidebar}>
        <div style={styles.wordmarkRow}>
          <span style={styles.wordmark}>Atlas</span>
        </div>
        <div style={styles.orgSwitch}>
          <div>
            <div style={styles.orgName}>Acme Logistics</div>
            <div style={styles.orgSub}>Finance Operations</div>
          </div>
          <ChevronDown size={15} color={C.muted} />
        </div>
        <nav style={styles.nav}>
          <NavItem icon={<Inbox size={18} />} label="Work" active={frame < frameAt(46.9, fps)} />
          <NavItem icon={<FileText size={18} />} label="Invoices" />
          <NavItem icon={<Building2 size={18} />} label="Suppliers" />
          <NavItem icon={<ClipboardCheck size={18} />} label="Approvals" />
          <NavItem icon={<ListOrdered size={18} />} label="Activity" active={frame >= frameAt(46.9, fps) && frame < frameAt(56, fps)} />
        </nav>
        <div style={styles.sidebarBottom}>
          <NavItem icon={<Settings size={18} />} label="Settings" />
          <div style={styles.sidebarDivider} />
          <div style={styles.searchRow}><Search size={15} /><span>Search</span><span style={styles.shortcut}>⌘ K</span></div>
          <div style={styles.userRow}>
            <div style={{ ...styles.avatar, boxShadow: `0 0 0 ${2 + avatarPulse * 3}px rgba(53,89,216,${avatarPulse * 0.16})` }}>AM</div>
            <div style={{ flex: 1 }}>
              <div style={styles.userName}>Amina M.</div>
              <div style={styles.userRole}>Finance Ops</div>
            </div>
            <ChevronDown size={14} color={C.muted} />
          </div>
        </div>
      </aside>
      <main style={styles.main}>
        <div style={styles.mainInner}>{children}</div>
      </main>
    </div>
  );
}

function NavItem({ icon, label, active = false, count }: { icon: ReactNode; label: string; active?: boolean; count?: string }) {
  return (
    <div style={{ ...styles.navItem, ...(active ? styles.navItemActive : {}) }}>
      <span style={{ ...styles.navIcon, ...(active ? { color: C.brand } : {}) }}>{icon}</span>
      <span style={{ fontWeight: active ? 600 : 500 }}>{label}</span>
      {count && <span style={{ ...styles.navCount, ...(active ? styles.navCountActive : {}) }}>{count}</span>}
    </div>
  );
}

function QueueScene({ frame }: { frame: number }) {
  const { fps } = useVideoConfig();
  const highlight = interpolate(frame, [frameAt(2.1, fps), frameAt(2.5, fps), frameAt(4.2, fps), frameAt(4.6, fps)], [0, 1, 1, 0], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });
  return (
    <>
      <PageTitle title="Good day, Amina." description="Your invoice exceptions are ready for review." />
      <div style={styles.filters}>
        <Chip active>All</Chip><Chip>Needs my decision</Chip><Chip>Waiting on supplier</Chip><Chip>Ready for approval</Chip>
      </div>
      <div style={styles.groupHeader}><ChevronDown size={15} /><span>Needs decision</span></div>
      <div style={styles.queueCard}>
        <QueueRow emphasis={highlight} priority age="2d ago" number="INV-4821" supplier="Kigali Metals" issue="Quantity mismatch · Price variance" amount="$12,480.00" recommendation="Hold payment and request corrected invoice" />
        <QueueRow age="1d ago" number="INV-4688" supplier="Douala Packaging" issue="Price variance" amount="$3,240.00" recommendation="Request price clarification from supplier" />
        <QueueRow age="1d ago" number="INV-4790" supplier="BuildCo" issue="Missing tax document" amount="$980.00" recommendation="Request the missing tax document" />
        <QueueRow age="1d ago" number="INV-4827" supplier="Douala Packaging" issue="Purchase order not matched" amount="$1,500.00" recommendation="Escalate to procurement to confirm the PO" />
      </div>
    </>
  );
}

function QueueRow({
  number,
  supplier,
  issue,
  amount,
  recommendation,
  emphasis = 0,
  priority = false,
  age = "—",
}: {
  number: string;
  supplier: string;
  issue: string;
  amount: string;
  recommendation?: string;
  emphasis?: number;
  priority?: boolean;
  age?: string;
}) {
  const background = emphasis > 0.05 ? `rgba(238,242,255,${0.6 * emphasis})` : C.surface;
  return (
    <div style={{ ...styles.queueRow, background, minHeight: 84, boxShadow: `inset 3px 0 0 rgba(53,89,216,${emphasis * 0.85})` }}>
      <div style={{ flex: 1, minWidth: 0 }}>
        <div style={styles.queueMainLine}>
          {priority && <span style={styles.priority}>HIGHEST PRIORITY</span>}
          <span style={styles.monoStrong}>{number}</span><span style={styles.separator}>·</span><strong>{supplier}</strong><span style={styles.separator}>·</span><span style={styles.rowIssue}>{issue}</span>
        </div>
        {recommendation && <div style={styles.queueRecommendation}><span style={{ color: C.brand, fontWeight: 600 }}>Agent:</span> {recommendation}</div>}
      </div>
      <div style={styles.rowRight}>
        <strong style={styles.rowAmount}>{amount}</strong>
        <Badge tone="warning">Needs decision</Badge>
        <span style={styles.rowTime}>{age}</span>
        <ChevronRight size={16} color="#c1c7d0" />
      </div>
    </div>
  );
}

function MatchScene({ frame }: { frame: number }) {
  const { fps } = useVideoConfig();
  const lineFocus = interpolate(frame, [frameAt(12.8, fps), frameAt(13.2, fps), frameAt(15.1, fps), frameAt(15.6, fps)], [0, 1, 1, 0], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });
  return (
    <>
      <Breadcrumbs current="INV-4821" />
      <PageTitle
        title={<><span style={styles.titleMono}>INV-4821</span><span style={styles.titleDot}>·</span><span>Kigali Metals</span><Badge tone="warning">Open</Badge></>}
        description={<><b>$12,480.00</b><span>·</span>Quantity mismatch<span>·</span>Created 2 days ago</>}
      />
      <div style={styles.detailGrid}>
        <div style={styles.leftColumn}>
          <SectionTitle>What happened</SectionTitle>
          <SummaryCard title="Invoice" rows={[
            ["Invoice number", "INV-4821", true], ["Supplier", "Kigali Metals"], ["Amount", "$12,480.00"], ["Purchase order", "PO-2291", true], ["Invoice date", "Nov 19, 2026"],
          ]} />
          <div style={{ marginTop: 18 }}>
            <SectionCard title="Line-level comparison" icon={<ArrowDownRight size={15} color={C.muted} />}>
              <ReconTable focus={lineFocus} />
            </SectionCard>
          </div>
          <div style={{ ...styles.callout, marginTop: 16 }}><CircleIcon tone="danger" /><span>Invoice quantity exceeds received quantity by <b>20 steel rods</b>.</span></div>
        </div>
        <div style={styles.rightColumn}>
          <RecommendationCard compact />
          <PolicyCard compact />
        </div>
      </div>
    </>
  );
}

function EvidenceScene({ frame }: { frame: number }) {
  const { fps } = useVideoConfig();
  const reveal = interpolate(frame, [frameAt(21, fps), frameAt(21.5, fps), frameAt(27.5, fps)], [0, 1, 1], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });
  const previewOpacity = interpolate(frame, [frameAt(22.3, fps), frameAt(22.55, fps), frameAt(25.05, fps), frameAt(25.35, fps)], [0, 1, 1, 0], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });
  return (
    <>
      <Breadcrumbs current="INV-4821" />
      <PageTitle title={<><span style={styles.titleMono}>INV-4821</span><span style={styles.titleDot}>·</span>Kigali Metals</>} description="Case evidence and reconciliation results" />
      <div style={styles.evidenceLayout}>
        <div>
          <SectionTitle>Evidence</SectionTitle>
          <div style={styles.evidenceGrid}>
            <EvidenceTile icon={<FileText size={20} />} label="Invoice PDF" file="INV-4821.pdf" tint="blue" />
            <EvidenceTile icon={<ClipboardCheck size={20} />} label="Purchase order" file="PO-2291.pdf" tint="slate" />
            <EvidenceTile icon={<Truck size={20} />} label="Delivery note" file="GRN-882.pdf" tint="green" active={frame >= frameAt(21.8, fps) && frame < frameAt(25.4, fps)} />
            <EvidenceTile icon={<FileCheck2 size={20} />} label="Delivery photo" file="GRN-882.jpg" tint="sand" />
          </div>
          <div style={{ marginTop: 22 }}>
            <SectionCard title="Line-level comparison">
              <ReconTable />
            </SectionCard>
          </div>
        </div>
        <div style={{ ...styles.discrepancyPanel, opacity: reveal }}>
          <SectionCard title="Discrepancies" icon={<Clock3 size={14} color={C.muted} />}>
            <Discrepancy reference="DIS-311" name="Quantity mismatch" severity="High" detail="100 units invoiced; 80 units confirmed in GRN-882." />
            <div style={styles.innerDivider} />
            <Discrepancy reference="DIS-313" name="Price variance" severity="Medium" detail="$104.00 per unit vs. $100.00 on PO-2291." />
          </SectionCard>
          <div style={{ ...styles.noteCard, marginTop: 16 }}><ShieldCheck size={17} color={C.brand} /><span>Evidence remains linked to the source record.</span></div>
        </div>
      </div>
      {previewOpacity > 0 && <EvidencePreview opacity={previewOpacity} />}
    </>
  );
}

function PolicyScene({ frame }: { frame: number }) {
  const { fps } = useVideoConfig();
  const pulse = interpolate(frame, [frameAt(31.5, fps), frameAt(32.1, fps), frameAt(33.9, fps), frameAt(34.4, fps)], [0, 1, 1, 0], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });
  const actionFocus = interpolate(frame, [frameAt(37.9, fps), frameAt(38.2, fps), frameAt(38.7, fps), frameAt(39, fps)], [0, 1, 1, 0], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });
  return (
    <>
      <Breadcrumbs current="INV-4821" />
      <PageTitle title={<><span style={styles.titleMono}>INV-4821</span><span style={styles.titleDot}>·</span>Kigali Metals</>} description={<><b>$12,480.00</b><span>·</span>Quantity mismatch<span>·</span>Open case</>} />
      <div style={styles.detailGrid}>
        <div style={styles.leftColumn}>
          <SectionTitle>What happened</SectionTitle>
          <SummaryCard title="Invoice" rows={[["Invoice number", "INV-4821", true], ["Supplier", "Kigali Metals"], ["Amount", "$12,480.00"], ["Purchase order", "PO-2291", true], ["Delivery", "SHP-4417 · GRN-882", true]]} />
          <SectionCard title="Line-level comparison" style={{ marginTop: 18 }}>
            <ReconTable />
          </SectionCard>
          <div style={{ ...styles.evidenceStrip, marginTop: 17 }}>
            <EvidenceMini icon={<FileText size={16} />} text="Invoice" />
            <EvidenceMini icon={<ClipboardCheck size={16} />} text="PO-2291" />
            <EvidenceMini icon={<Truck size={16} />} text="GRN-882" />
          </div>
        </div>
        <div style={styles.rightColumn}>
          <RecommendationCard frame={frame} />
          <PolicyCard frame={frame} pulse={pulse} />
          <ActionButtons focus={actionFocus} />
        </div>
      </div>
    </>
  );
}

function ActionScene({ frame }: { frame: number }) {
  const { fps } = useVideoConfig();
  const modalProgress = interpolate(frame, [frameAt(39, fps), frameAt(39.55, fps)], [0, 1], { extrapolateLeft: "clamp", extrapolateRight: "clamp", easing: Easing.out(Easing.cubic) });
  const sent = frame >= frameAt(40.55, fps);
  return (
    <>
      <Breadcrumbs current="INV-4821" />
      <PageTitle title={<><span style={styles.titleMono}>INV-4821</span><span style={styles.titleDot}>·</span>Kigali Metals</>} description="Request a corrected invoice" />
      <div style={styles.detailGrid}>
        <div style={styles.leftColumn}>
          <SectionTitle>Actions</SectionTitle>
          <SummaryCard title="Available actions" rows={[["Request document", "Allowed", false], ["Hold payment", "Allowed", false], ["Approve payment", "Blocked by policy", false]]} />
          <div style={{ ...styles.callout, marginTop: 18 }}><CircleIcon tone="danger" /><span>Payment remains blocked until the quantity difference is resolved.</span></div>
        </div>
        <div style={styles.rightColumn}>
          <RecommendationCard compact />
          <PolicyCard compact />
        </div>
      </div>
      <div style={{ ...styles.modalBackdrop, opacity: modalProgress }}>
        <div style={{ ...styles.dialog, transform: `translateY(${(1 - modalProgress) * 18}px) scale(${0.99 + modalProgress * 0.01})` }}>
          <button style={styles.dialogClose}><X size={17} color={C.muted} /></button>
          <div style={styles.dialogIcon}><FileText size={19} color={C.brand} /></div>
          <div style={styles.dialogTitle}>{sent ? "Request recorded" : "Send document request"}</div>
          <div style={styles.dialogDescription}>{sent ? "The demo recorded a request for a corrected invoice." : "Ask Kigali Metals to reissue a corrected invoice for INV-4821."}</div>
          <div style={styles.dialogDivider} />
          <div style={styles.dialogFooter}>
            <span style={styles.dialogFootnote}>SIMULATED OUTBOUND ACTION</span>
            <div style={styles.dialogButtons}><span style={styles.cancelButton}>Cancel</span><span style={styles.primaryButton}>{sent ? <><Check size={15} /> Recorded</> : <>Send request <ArrowRight size={15} /></>}</span></div>
          </div>
        </div>
      </div>
      {sent && <div style={styles.toast}><CheckCircle2 size={17} color={C.success} /><span>Document request recorded</span></div>}
    </>
  );
}

function ActivityScene({ frame }: { frame: number }) {
  const { fps } = useVideoConfig();
  const inspectorOpacity = interpolate(frame, [frameAt(49.2, fps), frameAt(49.65, fps), frameAt(55.5, fps)], [0, 1, 1], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });
  return (
    <>
      <PageTitle title="Activity" description="A traceable record of actions across your workspace." />
      <div style={styles.filters}>
        <Chip active>All events</Chip><Chip>Human</Chip><Chip>Agent</Chip><Chip>System</Chip><Chip><ShieldCheck size={13} />Policy &amp; security</Chip>
        <div style={styles.activitySearch}><Search size={14} /><span>Search actors, actions, objects…</span></div>
      </div>
      <div style={styles.activityGrid}>
        <div>
          <div style={styles.dateLabel}>NOVEMBER 21, 2026</div>
          <div style={styles.activityList}>
            <ActivityRow time="10:42" actorType="AGENT" action="Proposed hold payment" object="case · INV-4821" selected />
            <ActivityRow time="10:42" actorType="SYSTEM" action="Policy evaluation blocked" object="approve_payment · INV-4821" />
            <ActivityRow time="10:43" actorType="AGENT" action="Requested corrected invoice" object="supplier · Kigali Metals" />
            <ActivityRow time="10:43" actorType="SYSTEM" action="Evidence linked to case" object="3 source records · INV-4821" />
          </div>
        </div>
        <div style={{ ...styles.inspector, opacity: inspectorOpacity }}>
          <div style={styles.inspectorHeader}><span style={styles.inspectorEyebrow}>EVENT INSPECTION</span><Badge tone="neutral">Proposed</Badge></div>
          <h3 style={styles.inspectorTitle}>Proposed hold payment</h3>
          <div style={styles.detailRows}>
            <DetailRow label="Actor"><Badge tone="brand">Agent · demo</Badge></DetailRow>
            <DetailRow label="Object"><span style={styles.mono}>case · INV-4821</span></DetailRow>
            <DetailRow label="Policy version"><span style={styles.mono}>payment-policy-v3</span></DetailRow>
            <DetailRow label="Timestamp">Nov 21, 2026 · 10:42 AM</DetailRow>
          </div>
          <div style={styles.jsonSectionLabel}>INPUTS</div>
          <div style={styles.jsonBox}><span style={{ color: "#58698d" }}>{`{\n  "invoice": "INV-4821",\n  "purchaseOrder": "PO-2291",\n  "delivery": "GRN-882",\n  "evidence": 4\n}`}</span></div>
          <div style={styles.jsonSectionLabel}>POLICY RESULT</div>
          <div style={styles.policyResult}><span style={styles.policyResultDot} />Approval blocked · quantity mismatch unresolved</div>
          <div style={styles.openCase}>Open case workspace <ArrowRight size={14} /></div>
        </div>
      </div>
    </>
  );
}

function FinalSlate({ frame, fps }: { frame: number; fps: number }) {
  const opacity = interpolate(frame, [frameAt(56, fps), frameAt(57, fps), frameAt(59.5, fps), frameAt(60, fps)], [0, 1, 1, 0], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });
  return (
    <div style={{ ...styles.finalSlate, opacity }}>
      <div style={styles.closingLogo}>Atlas</div>
      <div style={styles.closingRule} />
      <div style={styles.closingCaption}>Kubernetes for Enterprise Reality</div>
      <div style={styles.closingMeta}>Product walkthrough · local demo data</div>
    </div>
  );
}

function RecommendationCard({ compact = false, frame = 0 }: { compact?: boolean; frame?: number }) {
  const { fps } = useVideoConfig();
  const phase = interpolate(frame, [frameAt(28.3, fps), frameAt(29.3, fps), frameAt(33.3, fps), frameAt(34, fps)], [0.985, 1, 1, 0.985], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });
  return (
    <div style={{ ...styles.recommendationCard, transform: `scale(${phase})` }}>
      <div style={styles.recommendationHeader}><span style={styles.recommendationLabel}><Sparkles size={14} color={C.brand} />Agent recommendation</span><Badge tone="success">high confidence</Badge></div>
      <div style={styles.recommendationBody}>
        <div style={styles.recommendationText}>Hold payment and request corrected invoice.</div>
        <div style={styles.recommendationMeta}><span>Evidence: <b>4 sources</b></span><span>Policy: <b>Payment policy v3</b></span></div>
        {!compact && <>
          <div style={styles.whyDivider} />
          <div style={styles.labelCaps}>WHY</div>
          <div style={styles.whyList}>
            <WhyLine n="1" text="Matched invoice INV-4821 to PO-2291." />
            <WhyLine n="2" text="GRN-882 confirms 80 steel rods received." />
            <WhyLine n="3" text="Invoice bills 100 rods; 20-unit gap found." />
            <WhyLine n="4" text="Price variance 4.0% exceeds 2% tolerance." />
          </div>
          <div style={styles.ruleViolation}><b>Quantity rule</b><span>Invoice quantity cannot exceed received quantity.</span></div>
        </>}
      </div>
    </div>
  );
}

function WhyLine({ n, text }: { n: string; text: string }) {
  return <div style={styles.whyLine}><span style={styles.whyNumber}>{n}.</span><span>{text}</span></div>;
}

function PolicyCard({ compact = false, pulse = 0 }: { compact?: boolean; frame?: number; pulse?: number }) {
  return (
    <div style={styles.policyCard}>
      <div style={styles.policyHeader}><span style={styles.policyTitle}><ShieldCheck size={15} color={C.brand} />Policy</span><span style={styles.policyVersion}>payment-policy-v3</span></div>
      <div style={styles.policyBody}>
        <PolicyRule text="Human approval required above $5,000." />
        <PolicyRule text="Agent cannot approve payment directly." />
        {!compact && <PolicyRule text="Agent may request documents and escalate cases." />}
        <PolicyRule text="Invoice quantity cannot exceed received quantity." />
        <PolicyRule text="Price variance above 2% requires review." />
        <div style={{ ...styles.blockedBadge, boxShadow: pulse ? `0 0 0 ${pulse * 5}px rgba(196,60,60,${pulse * 0.08})` : undefined }}>
          <X size={14} />Approve $12,480.00: blocked by policy
        </div>
        {!compact && <div style={styles.blockedReason}>Unresolved quantity mismatch. Payment approval is unavailable until the received quantity is confirmed.</div>}
      </div>
    </div>
  );
}

function PolicyRule({ text }: { text: string }) {
  return <div style={styles.policyRule}><span style={styles.policyBullet} />{text}</div>;
}

function ActionButtons({ focus = 0 }: { focus?: number }) {
  return (
    <div style={styles.actionButtons}>
      <div style={{ ...styles.actionButton, ...(focus > 0 ? styles.actionButtonFocused : {}), boxShadow: focus > 0.1 ? `0 0 0 ${focus * 4}px rgba(53,89,216,${focus * 0.12})` : undefined }}><FileText size={15} color={focus > 0 ? C.brand : C.secondary} />Request corrected invoice<ChevronRight size={14} color={C.muted} /></div>
      <div style={styles.actionButton}><ShieldCheck size={15} color={C.secondary} />Hold payment<ChevronRight size={14} color={C.muted} /></div>
      <div style={styles.disabledButton}><CheckCircle2 size={15} color="#c7ccd3" />Approve payment <span style={styles.actionBlocked}>Blocked by policy</span></div>
    </div>
  );
}

function SummaryCard({ title, rows }: { title: string; rows: [string, string, boolean?][] }) {
  return (
    <div style={styles.summaryCard}>
      <div style={styles.summaryHeader}>{title}</div>
      <div style={styles.summaryRows}>{rows.map(([label, value, mono]) => <div style={styles.summaryRow} key={label}><span>{label}</span><strong style={mono ? styles.mono : undefined}>{value}</strong></div>)}</div>
      <div style={styles.summaryFooter}>Matched to a purchase order and delivery record. Review the source evidence before resolving this case.</div>
    </div>
  );
}

function SectionCard({ title, icon, children, style }: { title: string; icon?: ReactNode; children: ReactNode; style?: CSSProperties }) {
  return <div style={{ ...styles.sectionCard, ...style }}><div style={styles.sectionCardHeader}><span>{title}</span>{icon}</div>{children}</div>;
}

function ReconTable({ focus = 0 }: { focus?: number }) {
  const rows = [
    { item: "Steel rods", ordered: "100", invoiced: "100", received: "80", difference: "+20", failed: true },
    { item: "Cement bags", ordered: "50", invoiced: "50", received: "50", difference: "0", failed: false },
  ];
  const headings = ["Item", "Ordered", "Invoiced", "Received", "Difference", "Status"];
  return (
    <table style={styles.table}>
      <thead><tr>{headings.map((heading, i) => <th key={heading} style={{ ...styles.tableHeading, textAlign: i === 0 ? "left" : "right" }}>{heading}</th>)}</tr></thead>
      <tbody>
        {rows.map((row, rowIndex) => (
          <tr key={row.item} style={{ background: rowIndex === 0 && focus ? `rgba(253,240,240,${0.8 * focus})` : C.surface }}>
            <td style={{ ...styles.tableCell, ...styles.tableItem }}>{row.item}</td>
            <td style={styles.tableCell}>{row.ordered}</td>
            <td style={styles.tableCell}>{row.invoiced}</td>
            <td style={styles.tableCell}>{row.received}</td>
            <td style={{ ...styles.tableCell, ...(row.failed ? styles.badNumber : {}) }}>{row.difference}</td>
            <td style={{ ...styles.tableCell, textAlign: "right" }}><Badge tone={row.failed ? "danger" : "success"}>{row.failed ? "Failed" : "Passed"}</Badge></td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function SectionTitle({ children }: { children: ReactNode }) {
  return <h2 style={styles.sectionTitle}>{children}</h2>;
}

function PageTitle({ title, description }: { title: ReactNode; description?: ReactNode }) {
  return <header style={styles.pageHeader}><h1 style={styles.pageTitle}>{title}</h1>{description && <div style={styles.pageDescription}>{description}</div>}</header>;
}

function Breadcrumbs({ current }: { current: string }) {
  return <div style={styles.breadcrumbs}><span>Work</span><ChevronRight size={12} /><span style={styles.mono}>{current}</span></div>;
}

function Chip({ children, active = false }: { children: ReactNode; active?: boolean }) {
  return <div style={{ ...styles.chip, ...(active ? styles.chipActive : {}) }}>{children}</div>;
}

function Badge({ children, tone = "neutral" }: { children: ReactNode; tone?: "neutral" | "warning" | "danger" | "success" | "brand" }) {
  const toneStyle = {
    neutral: { color: C.secondary, background: C.surfaceSubtle, border: C.border },
    warning: { color: C.warning, background: C.warningBg, border: "#f1dfbd" },
    danger: { color: C.danger, background: C.dangerBg, border: "#f2d8d8" },
    success: { color: C.success, background: C.successBg, border: "#d6ebdc" },
    brand: { color: C.brand, background: C.brandSoft, border: "#d8e0ff" },
  }[tone];
  return <span style={{ ...styles.badge, color: toneStyle.color, background: toneStyle.background, borderColor: toneStyle.border }}>{children}</span>;
}

function EvidenceTile({ icon, label, file, tint, active = false }: { icon: ReactNode; label: string; file: string; tint: "blue" | "slate" | "green" | "sand"; active?: boolean }) {
  const colors = { blue: "#e8efff", slate: "#eff1f4", green: "#e9f5ed", sand: "#faf1e4" };
  return <div style={{ ...styles.evidenceTile, ...(active ? styles.evidenceTileActive : {}) }}><div style={{ ...styles.evidenceIcon, background: colors[tint] }}>{icon}</div><div style={styles.evidenceLabel}>{label}</div><div style={styles.evidenceFile}>{file}</div><div style={styles.viewEvidence}>Open preview <ArrowRight size={12} /></div></div>;
}

function EvidencePreview({ opacity }: { opacity: number }) {
  return (
    <div style={{ ...styles.modalBackdrop, opacity }}>
      <div style={styles.evidenceDialog}>
        <div style={styles.evidencePreviewTop}>
          <span>Goods received note</span>
          <Badge tone="brand">Source record</Badge>
          <span style={styles.evidencePreviewClose}><X size={16} color={C.muted} /></span>
        </div>
        <div style={styles.evidencePaper}>
          <div style={styles.evidencePaperBrand}>GOODS RECEIVED NOTE <span>GRN-882</span></div>
          <div style={styles.evidencePaperTitle}>Kigali Metals</div>
          <div style={styles.evidencePaperMeta}>Purchase order <b>PO-2291</b><span>·</span> Received Nov 16, 2026</div>
          <div style={styles.evidencePaperLine}><span>Steel rods</span><b>80 bundles received</b><Badge tone="success">Confirmed</Badge></div>
          <div style={styles.evidencePaperNote}>Warehouse note: Partial delivery, 80 bundles at receiving bay 2.</div>
        </div>
      </div>
    </div>
  );
}

function Discrepancy({ reference, name, severity, detail }: { reference: string; name: string; severity: string; detail: string }) {
  return <div style={styles.discrepancy}><div style={styles.discrepancyHead}><span style={styles.mono}>{reference}</span><Badge tone={severity === "High" ? "danger" : "warning"}>{severity}</Badge></div><div style={styles.discrepancyTitle}>{name}</div><div style={styles.discrepancyDetail}>{detail}</div></div>;
}

function EvidenceMini({ icon, text }: { icon: ReactNode; text: string }) {
  return <div style={styles.evidenceMini}>{icon}<span>{text}</span><CheckCircle2 size={13} color={C.success} /></div>;
}

function ActivityRow({ time, actorType, action, object, selected = false }: { time: string; actorType: "AGENT" | "SYSTEM" | "HUMAN"; action: string; object: string; selected?: boolean }) {
  const color = actorType === "AGENT" ? C.brand : actorType === "HUMAN" ? C.info : "#b9c0ca";
  return <div style={{ ...styles.activityRow, background: selected ? "#f3f5ff" : C.surface }}><span style={styles.eventTime}>{time}</span><span style={{ ...styles.eventDot, background: color }} /><div style={styles.eventCopy}><div style={styles.eventAction}>{action}</div><div style={styles.eventObject}>{object}</div></div><Badge tone={actorType === "AGENT" ? "brand" : "neutral"}>{actorType === "AGENT" ? "Agent" : actorType === "HUMAN" ? "Human" : "System"}</Badge><ChevronRight size={15} color="#c1c7d0" /></div>;
}

function DetailRow({ label, children }: { label: string; children: ReactNode }) {
  return <div style={styles.detailRow}><span>{label}</span><strong>{children}</strong></div>;
}

function Caption({ cue, frame, fps }: { cue: Cue; frame: number; fps: number }) {
  const start = frameAt(cue.start, fps);
  const end = frameAt(cue.end, fps);
  const fadeDuration = frameAt(0.35, fps);
  const fade = Math.min(
    interpolate(frame, [start, start + fadeDuration], [0, 1], { extrapolateLeft: "clamp", extrapolateRight: "clamp" }),
    interpolate(frame, [end - fadeDuration, end], [1, 0], { extrapolateLeft: "clamp", extrapolateRight: "clamp" }),
  );
  return (
    <div style={{ ...styles.captionWrap, opacity: fade, transform: `translate(-50%, ${interpolate(fade, [0, 1], [8, 0])}px)` }}>
      <div style={styles.captionText}>{cue.lines.map((line) => <div key={line}>{line}</div>)}</div>
    </div>
  );
}

function MousePointer({ frame, fps }: { frame: number; fps: number }) {
  const path: [number, number, number][] = [
    [0, 1580, 820], [2.3, 624, 246], [3.2, 624, 246], [7.3, 700, 320],
    [12.8, 655, 578], [14.2, 810, 442], [21.4, 1024, 272], [22.2, 1024, 272],
    [24.9, 1364, 367], [25.35, 1364, 367], [27.5, 1240, 485],
    [37.8, 1460, 786], [38.35, 1460, 786], [39.4, 1412, 572],
    [40.15, 1263, 613], [40.55, 1263, 613], [47.2, 1300, 450],
    [49, 660, 240], [49.2, 660, 240], [50.2, 950, 420], [54.8, 1560, 810],
  ];
  const input = path.map(([seconds]) => frameAt(seconds, fps));
  const x = interpolate(frame, input, path.map(([, px]) => px), { extrapolateLeft: "clamp", extrapolateRight: "clamp", easing: Easing.inOut(Easing.quad) });
  const y = interpolate(frame, input, path.map(([, , py]) => py), { extrapolateLeft: "clamp", extrapolateRight: "clamp", easing: Easing.inOut(Easing.quad) });
  const clicking = CLICK_EVENTS.some((event) => Math.abs(frame - frameAt(event.at, fps)) < 2);
  return <div style={{ ...styles.cursor, left: x, top: y, transform: `scale(${clicking ? 0.84 : 1})` }}><svg width="27" height="34" viewBox="0 0 27 34" fill="none"><path d="M2 1.8L2.1 25.4L8.2 19.4L12.7 30.8L17.2 28.9L12.5 17.8L21 17.2L2 1.8Z" fill="#fff" stroke="#20242b" strokeWidth="1.6" strokeLinejoin="round" /></svg></div>;
}

function ClickSounds({ fps }: { fps: number }) {
  const duration = frameAt(0.16, fps);
  return <>{CLICK_EVENTS.map((event, index) => <Sequence key={index} from={Math.max(0, frameAt(event.at, fps) - 1)} durationInFrames={duration} layout="none"><Audio src={staticFile("atlas-ui-click.wav")} volume={0.78} /></Sequence>)}</>;
}

function ClickFeedback({ frame, fps }: { frame: number; fps: number }) {
  const rippleFrames = frameAt(0.42, fps);
  return <>{CLICK_EVENTS.map((event) => {
    const elapsed = frame - frameAt(event.at, fps);
    if (elapsed < 0 || elapsed > rippleFrames) return null;
    const progress = elapsed / rippleFrames;
    const diameter = 18 + progress * 54;
    return <div key={event.at} style={{ ...styles.clickFeedback, left: event.x - diameter / 2, top: event.y - diameter / 2, width: diameter, height: diameter, opacity: (1 - progress) * 0.82 }} />;
  })}</>;
}

function ProgressBar({ frame, durationInFrames }: { frame: number; durationInFrames: number }) {
  const pct = frame / durationInFrames;
  return <div style={styles.progressTrack}><div style={{ ...styles.progressFill, width: `${pct * 100}%` }} /></div>;
}

function CircleIcon({ tone }: { tone: "danger" }) {
  return <span style={{ ...styles.circleIcon, color: tone === "danger" ? C.danger : C.brand }}>!</span>;
}

const styles: Record<string, CSSProperties> = {
  root: { width: 1920, height: 1080, overflow: "hidden", position: "relative", background: C.background, color: C.foreground, fontFamily: 'Inter, "Segoe UI", Arial, sans-serif', fontSize: 15, letterSpacing: "-0.01em" },
  app: { position: "absolute", inset: 0, display: "flex", background: C.background },
  sidebar: { position: "absolute", left: 0, top: 0, bottom: 0, width: APP_LEFT, background: C.surface, borderRight: `1px solid ${C.border}`, display: "flex", flexDirection: "column", boxSizing: "border-box", zIndex: 3 },
  wordmarkRow: { height: 66, display: "flex", alignItems: "center", gap: 11, padding: "0 22px" },
  wordmark: { fontWeight: 650, fontSize: 19, letterSpacing: "-0.04em" },
  orgSwitch: { margin: "4px 13px 12px", padding: "12px 11px", height: 57, border: `1px solid ${C.border}`, borderRadius: 7, display: "flex", alignItems: "center", justifyContent: "space-between", boxSizing: "border-box", boxShadow: "0 1px 2px rgba(25,30,40,.03)" },
  orgName: { fontSize: 14, fontWeight: 550, lineHeight: 1.3 },
  orgSub: { fontSize: 12, color: C.secondary, marginTop: 3 },
  nav: { display: "flex", flexDirection: "column", gap: 3, padding: "7px 12px" },
  navItem: { height: 43, padding: "0 12px", borderRadius: 6, color: C.secondary, display: "flex", alignItems: "center", gap: 12, fontSize: 14, boxSizing: "border-box" },
  navItemActive: { background: C.brandSoft, color: C.brand },
  navIcon: { width: 19, height: 19, display: "flex", alignItems: "center", justifyContent: "center", color: "#8f97a3" },
  navCount: { marginLeft: "auto", background: C.surfaceSubtle, color: C.secondary, padding: "2px 7px", borderRadius: 4, fontSize: 12, fontWeight: 600 },
  navCountActive: { background: "white", color: C.brand },
  sidebarBottom: { marginTop: "auto", padding: "0 12px 15px" },
  sidebarDivider: { height: 1, background: C.border, margin: "12px 0" },
  searchRow: { height: 38, display: "flex", alignItems: "center", gap: 10, color: C.secondary, padding: "0 12px", fontSize: 13 },
  shortcut: { marginLeft: "auto", border: `1px solid ${C.border}`, borderRadius: 4, padding: "1px 5px", color: C.muted, fontSize: 10 },
  userRow: { height: 55, display: "flex", alignItems: "center", gap: 10, padding: "0 6px" },
  avatar: { width: 33, height: 33, borderRadius: "50%", background: "#e9edfa", color: C.brand, fontWeight: 650, fontSize: 12, display: "flex", alignItems: "center", justifyContent: "center" },
  userName: { fontSize: 13, fontWeight: 600 },
  userRole: { fontSize: 11, color: C.secondary, marginTop: 2 },
  main: { position: "absolute", inset: "0 0 0 248px", overflow: "hidden" },
  mainInner: { position: "absolute", width: APP_WIDTH, left: `calc(50% - ${APP_WIDTH / 2}px)`, top: 0, bottom: 0, padding: "37px 0 98px", boxSizing: "border-box" },
  scene: { position: "absolute", left: 0, right: 0, top: 0, bottom: 0, paddingTop: 37, boxSizing: "border-box" },
  pageHeader: { marginBottom: 25 },
  pageTitle: { margin: 0, display: "flex", alignItems: "center", gap: 10, minHeight: 33, fontSize: 26, lineHeight: 1.25, fontWeight: 620, letterSpacing: "-0.035em", color: C.foreground },
  pageDescription: { display: "flex", alignItems: "center", gap: 9, color: C.secondary, fontSize: 14, marginTop: 8 },
  titleMono: { fontFamily: "'Geist Mono', Consolas, monospace", fontWeight: 600, letterSpacing: "-0.035em" },
  titleDot: { color: "#c5cad2", fontSize: 23, fontWeight: 400 },
  breadcrumbs: { display: "flex", alignItems: "center", gap: 7, color: C.secondary, fontSize: 13, marginBottom: 12 },
  filters: { display: "flex", alignItems: "center", gap: 8, marginBottom: 23 },
  chip: { display: "inline-flex", alignItems: "center", gap: 7, height: 32, padding: "0 12px", background: C.surface, border: `1px solid ${C.border}`, borderRadius: 6, fontSize: 13, fontWeight: 500, color: C.secondary, whiteSpace: "nowrap" },
  chipActive: { background: C.brandSoft, borderColor: "#d2dcff", color: C.brand },
  groupHeader: { display: "flex", alignItems: "center", gap: 8, margin: "8px 0 10px", color: C.foreground, fontSize: 15, fontWeight: 620 },
  queueCard: { background: C.surface, border: `1px solid ${C.border}`, borderRadius: 9, overflow: "hidden", boxShadow: "0 1px 2px rgba(18,24,40,.025)" },
  queueRow: { display: "flex", alignItems: "center", justifyContent: "space-between", gap: 20, padding: "15px 17px", borderBottom: `1px solid ${C.border}`, boxSizing: "border-box" },
  queueMainLine: { display: "flex", alignItems: "center", gap: 9, flexWrap: "wrap", fontSize: 14 },
  priority: { padding: "3px 6px", background: C.brand, color: "white", borderRadius: 4, fontSize: 9, letterSpacing: ".09em", fontWeight: 650 },
  monoStrong: { fontFamily: "'Geist Mono', Consolas, monospace", fontWeight: 650, color: C.foreground, fontSize: 13 },
  separator: { color: "#bcc2cc" },
  rowIssue: { color: C.secondary, fontSize: 13 },
  queueRecommendation: { color: C.secondary, marginTop: 8, fontSize: 13 },
  rowRight: { display: "flex", alignItems: "center", gap: 16, flexShrink: 0 },
  rowAmount: { fontVariantNumeric: "tabular-nums", fontSize: 14, fontWeight: 620 },
  rowTime: { width: 57, textAlign: "right", fontSize: 12, color: C.muted },
  detailGrid: { display: "grid", gridTemplateColumns: "minmax(0, 1fr) 390px", gap: 24, alignItems: "start" },
  leftColumn: { minWidth: 0 },
  rightColumn: { display: "flex", flexDirection: "column", gap: 15 },
  sectionTitle: { margin: "0 0 13px", fontSize: 17, fontWeight: 620, letterSpacing: "-0.015em" },
  summaryCard: { border: `1px solid ${C.border}`, borderRadius: 8, background: C.surface, boxShadow: "0 1px 2px rgba(18,24,40,.025)", overflow: "hidden" },
  summaryHeader: { height: 42, boxSizing: "border-box", display: "flex", alignItems: "center", padding: "0 16px", borderBottom: `1px solid ${C.border}`, fontSize: 13, fontWeight: 620 },
  summaryRows: { padding: "0 17px" },
  summaryRow: { minHeight: 38, display: "flex", alignItems: "center", justifyContent: "space-between", borderBottom: `1px solid ${C.border}`, fontSize: 13, color: C.secondary },
  summaryFooter: { background: C.surfaceSubtle, padding: "11px 17px", fontSize: 12, lineHeight: 1.45, color: C.secondary },
  sectionCard: { border: `1px solid ${C.border}`, borderRadius: 8, background: C.surface, boxShadow: "0 1px 2px rgba(18,24,40,.025)", overflow: "hidden" },
  sectionCardHeader: { height: 41, padding: "0 16px", display: "flex", alignItems: "center", justifyContent: "space-between", borderBottom: `1px solid ${C.border}`, fontSize: 13, fontWeight: 620 },
  table: { width: "100%", borderCollapse: "collapse", fontSize: 12.5, fontVariantNumeric: "tabular-nums" },
  tableHeading: { color: C.secondary, fontSize: 10, textTransform: "uppercase", letterSpacing: ".07em", fontWeight: 550, padding: "11px 11px", borderBottom: `1px solid ${C.border}`, whiteSpace: "nowrap" },
  tableCell: { padding: "13px 11px", textAlign: "right", borderBottom: `1px solid ${C.border}`, color: C.secondary, whiteSpace: "nowrap" },
  tableItem: { textAlign: "left", color: C.foreground, fontWeight: 600 },
  badNumber: { color: C.danger, fontWeight: 650 },
  callout: { display: "flex", alignItems: "center", gap: 10, background: C.dangerBg, border: "1px solid #f2d8d8", borderRadius: 7, padding: "11px 14px", color: "#843434", fontSize: 13 },
  circleIcon: { width: 18, height: 18, display: "inline-flex", justifyContent: "center", alignItems: "center", borderRadius: "50%", border: "1.5px solid currentColor", fontSize: 12, fontWeight: 700, flexShrink: 0 },
  recommendationCard: { border: "1px solid #d5defb", background: "rgba(238,242,255,.52)", borderRadius: 9, boxShadow: "0 1px 3px rgba(24,40,100,.035)", transformOrigin: "center" },
  recommendationHeader: { height: 43, padding: "0 15px", display: "flex", alignItems: "center", justifyContent: "space-between", borderBottom: "1px solid rgba(53,89,216,.13)" },
  recommendationLabel: { display: "flex", alignItems: "center", gap: 7, color: C.brand, fontSize: 13, fontWeight: 650 },
  recommendationBody: { padding: "15px 16px" },
  recommendationText: { fontSize: 15, lineHeight: 1.35, fontWeight: 600 },
  recommendationMeta: { display: "flex", justifyContent: "space-between", gap: 10, marginTop: 11, color: C.secondary, fontSize: 11.5 },
  whyDivider: { height: 1, background: "rgba(53,89,216,.16)", margin: "15px 0 12px" },
  labelCaps: { color: C.muted, fontWeight: 650, fontSize: 10, letterSpacing: ".11em", marginBottom: 8 },
  whyList: { display: "flex", flexDirection: "column", gap: 7 },
  whyLine: { display: "flex", gap: 8, color: C.foreground, fontSize: 11.5, lineHeight: 1.35 },
  whyNumber: { width: 15, flexShrink: 0, color: C.muted, fontFamily: "Consolas, monospace" },
  ruleViolation: { display: "flex", flexDirection: "column", gap: 4, marginTop: 13, padding: "10px 11px", border: "1px solid #f2d8d8", borderRadius: 6, background: C.dangerBg, fontSize: 11, color: C.danger },
  policyCard: { border: `1px solid ${C.border}`, background: C.surface, borderRadius: 9, boxShadow: "0 1px 2px rgba(18,24,40,.025)", overflow: "hidden" },
  policyHeader: { height: 42, padding: "0 15px", display: "flex", alignItems: "center", justifyContent: "space-between", borderBottom: `1px solid ${C.border}` },
  policyTitle: { display: "flex", alignItems: "center", gap: 7, fontSize: 13, fontWeight: 650 },
  policyVersion: { color: C.muted, fontSize: 10, fontFamily: "Consolas, monospace" },
  policyBody: { padding: "13px 15px" },
  policyRule: { display: "flex", alignItems: "flex-start", gap: 8, marginBottom: 8, color: C.secondary, fontSize: 11.5, lineHeight: 1.4 },
  policyBullet: { width: 4, height: 4, background: "#b9c0ca", borderRadius: "50%", marginTop: 5, flexShrink: 0 },
  blockedBadge: { display: "flex", alignItems: "center", gap: 6, border: "1px solid #f1d4d4", background: C.dangerBg, color: C.danger, borderRadius: 5, padding: "7px 8px", fontSize: 10.5, fontWeight: 600, marginTop: 10 },
  blockedReason: { color: C.secondary, fontSize: 11, lineHeight: 1.45, marginTop: 8 },
  actionButtons: { display: "flex", flexDirection: "column", gap: 7 },
  actionButton: { height: 38, display: "flex", alignItems: "center", gap: 8, border: `1px solid ${C.border}`, background: C.surface, borderRadius: 6, padding: "0 11px", color: C.foreground, fontSize: 12, fontWeight: 550 },
  actionButtonFocused: { borderColor: "#b9c8ff", background: "#f5f7ff", color: C.brand },
  disabledButton: { height: 38, display: "flex", alignItems: "center", gap: 8, border: `1px solid ${C.border}`, background: "#f7f8fa", borderRadius: 6, padding: "0 11px", color: "#aeb4be", fontSize: 12, fontWeight: 550 },
  actionBlocked: { marginLeft: "auto", color: C.danger, fontSize: 10 },
  evidenceLayout: { display: "grid", gridTemplateColumns: "minmax(0, 1fr) 325px", gap: 23, alignItems: "start" },
  evidenceGrid: { display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: 12 },
  evidenceTile: { minHeight: 174, padding: 15, background: C.surface, border: `1px solid ${C.border}`, borderRadius: 8, boxShadow: "0 1px 2px rgba(18,24,40,.025)" },
  evidenceTileActive: { border: `2px solid ${C.brand}`, padding: 14, background: "#f8f9ff", boxShadow: "0 0 0 4px rgba(53,89,216,.09)" },
  evidenceIcon: { width: 35, height: 35, borderRadius: 7, display: "flex", alignItems: "center", justifyContent: "center", color: C.secondary, marginBottom: 13 },
  evidenceLabel: { fontSize: 13, fontWeight: 600 },
  evidenceFile: { fontFamily: "Consolas, monospace", color: C.muted, fontSize: 10.5, marginTop: 5 },
  viewEvidence: { display: "flex", alignItems: "center", gap: 5, marginTop: 17, color: C.brand, fontSize: 11, fontWeight: 550 },
  discrepancyPanel: { transition: "none" },
  discrepancy: { padding: "13px 15px" },
  discrepancyHead: { display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 9, fontSize: 11 },
  discrepancyTitle: { fontSize: 13, fontWeight: 600 },
  discrepancyDetail: { fontSize: 11.5, lineHeight: 1.45, color: C.secondary, marginTop: 5 },
  innerDivider: { height: 1, background: C.border, margin: "0 14px" },
  noteCard: { display: "flex", alignItems: "center", gap: 9, padding: "12px 13px", border: "1px solid #dbe3fb", background: "#f5f7ff", borderRadius: 7, color: C.secondary, fontSize: 11.5 },
  evidenceStrip: { display: "flex", gap: 8 },
  evidenceMini: { flex: 1, height: 43, display: "flex", alignItems: "center", gap: 8, border: `1px solid ${C.border}`, borderRadius: 6, padding: "0 10px", background: C.surface, color: C.secondary, fontSize: 11 },
  modalBackdrop: { position: "absolute", inset: 0, zIndex: 8, background: "rgba(31,38,53,.20)", display: "flex", alignItems: "center", justifyContent: "center", backdropFilter: "blur(1px)" },
  evidenceDialog: { width: 590, padding: 20, border: `1px solid ${C.border}`, borderRadius: 11, background: C.surface, boxShadow: "0 22px 70px rgba(25,34,56,.18)" },
  evidencePreviewTop: { display: "flex", alignItems: "center", gap: 9, height: 35, paddingBottom: 12, borderBottom: `1px solid ${C.border}`, fontSize: 14, fontWeight: 620 },
  evidencePreviewClose: { marginLeft: "auto", padding: 4, display: "flex" },
  evidencePaper: { marginTop: 17, border: "1px solid #e6e8ec", borderRadius: 7, padding: "18px 20px", background: "#fdfdfd", boxShadow: "0 2px 7px rgba(18,24,40,.035)" },
  evidencePaperBrand: { display: "flex", alignItems: "center", justifyContent: "space-between", color: C.muted, fontSize: 10, letterSpacing: ".1em", fontWeight: 650, borderBottom: "1px dashed #d9dde4", paddingBottom: 10 },
  evidencePaperTitle: { fontSize: 21, fontWeight: 620, marginTop: 18 },
  evidencePaperMeta: { display: "flex", alignItems: "center", gap: 9, color: C.secondary, fontSize: 12, marginTop: 6 },
  evidencePaperLine: { display: "flex", alignItems: "center", gap: 12, marginTop: 24, padding: "15px 12px", border: `1px solid ${C.border}`, borderRadius: 6, fontSize: 13 },
  evidencePaperNote: { marginTop: 14, color: C.secondary, background: C.surfaceSubtle, borderRadius: 5, padding: "10px 12px", fontSize: 11.5 },
  dialog: { width: 520, background: C.surface, border: `1px solid ${C.border}`, borderRadius: 12, boxShadow: "0 22px 70px rgba(25,34,56,.18)", padding: 25, position: "relative", boxSizing: "border-box" },
  dialogClose: { border: 0, background: "transparent", position: "absolute", top: 15, right: 15, padding: 4 },
  dialogIcon: { width: 40, height: 40, background: C.brandSoft, borderRadius: 8, display: "flex", alignItems: "center", justifyContent: "center", marginBottom: 16 },
  dialogTitle: { fontSize: 18, fontWeight: 650, letterSpacing: "-0.02em" },
  dialogDescription: { marginTop: 8, color: C.secondary, fontSize: 13, lineHeight: 1.5, maxWidth: 430 },
  dialogDivider: { height: 1, background: C.border, margin: "22px -25px 15px" },
  dialogFooter: { display: "flex", alignItems: "center", justifyContent: "space-between" },
  dialogFootnote: { color: C.muted, fontSize: 9, fontWeight: 650, letterSpacing: ".11em" },
  dialogButtons: { display: "flex", alignItems: "center", gap: 9 },
  cancelButton: { border: `1px solid ${C.border}`, borderRadius: 6, padding: "8px 12px", color: C.secondary, fontSize: 12, fontWeight: 550 },
  primaryButton: { display: "inline-flex", alignItems: "center", gap: 7, borderRadius: 6, padding: "8px 13px", background: C.brand, color: "white", fontSize: 12, fontWeight: 600 },
  toast: { position: "absolute", left: "50%", top: 34, transform: "translateX(-50%)", zIndex: 10, display: "flex", alignItems: "center", gap: 9, border: `1px solid ${C.border}`, borderRadius: 8, background: C.surface, boxShadow: "0 8px 28px rgba(28,36,54,.12)", padding: "12px 16px", fontSize: 13, fontWeight: 550 },
  activitySearch: { marginLeft: "auto", display: "flex", alignItems: "center", gap: 8, height: 32, width: 270, border: `1px solid ${C.border}`, borderRadius: 6, background: C.surface, color: C.muted, padding: "0 10px", fontSize: 11.5 },
  activityGrid: { display: "grid", gridTemplateColumns: "minmax(0, 1fr) 420px", gap: 22 },
  dateLabel: { color: C.muted, fontSize: 10, fontWeight: 650, letterSpacing: ".1em", margin: "5px 0 10px" },
  activityList: { border: `1px solid ${C.border}`, borderRadius: 8, background: C.surface, overflow: "hidden" },
  activityRow: { minHeight: 67, display: "flex", alignItems: "center", gap: 11, padding: "0 13px", borderBottom: `1px solid ${C.border}` },
  eventTime: { width: 44, color: C.muted, fontSize: 11, fontFamily: "Consolas, monospace" },
  eventDot: { width: 7, height: 7, borderRadius: "50%", flexShrink: 0 },
  eventCopy: { flex: 1, minWidth: 0 },
  eventAction: { color: C.foreground, fontSize: 12.5, fontWeight: 550 },
  eventObject: { color: C.secondary, fontSize: 11, marginTop: 3 },
  inspector: { border: `1px solid ${C.border}`, borderRadius: 8, background: C.surface, boxShadow: "0 1px 2px rgba(18,24,40,.025)", padding: 17 },
  inspectorHeader: { display: "flex", alignItems: "center", justifyContent: "space-between" },
  inspectorEyebrow: { color: C.muted, fontSize: 9, fontWeight: 650, letterSpacing: ".12em" },
  inspectorTitle: { fontSize: 17, margin: "13px 0 12px", letterSpacing: "-.02em" },
  detailRows: { borderTop: `1px solid ${C.border}`, borderBottom: `1px solid ${C.border}` },
  detailRow: { display: "flex", alignItems: "center", justifyContent: "space-between", minHeight: 34, borderBottom: `1px solid ${C.border}`, color: C.secondary, fontSize: 11 },
  mono: { fontFamily: "'Geist Mono', Consolas, monospace", color: C.foreground, fontWeight: 500, fontSize: 11 },
  jsonSectionLabel: { color: C.muted, fontSize: 9, fontWeight: 650, letterSpacing: ".12em", margin: "14px 0 6px" },
  jsonBox: { background: "#f7f8fa", border: `1px solid ${C.border}`, borderRadius: 6, padding: "9px 11px", fontFamily: "Consolas, monospace", fontSize: 10, lineHeight: 1.5 },
  policyResult: { display: "flex", alignItems: "center", gap: 7, color: C.danger, fontSize: 10.5, background: C.dangerBg, border: "1px solid #f2d8d8", borderRadius: 5, padding: "8px 9px" },
  policyResultDot: { width: 6, height: 6, background: C.danger, borderRadius: "50%" },
  openCase: { display: "flex", alignItems: "center", gap: 6, color: C.brand, fontSize: 11, fontWeight: 550, marginTop: 14 },
  finalSlate: { position: "absolute", inset: 0, zIndex: 16, background: "#fbfcfd", display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center" },
  closingLogo: { fontSize: 27, fontWeight: 650, letterSpacing: "-.045em" },
  closingRule: { height: 1, width: 95, background: "#dfe3e9", margin: "22px 0 20px" },
  closingCaption: { color: C.foreground, fontSize: 31, fontWeight: 600, letterSpacing: "-.03em" },
  closingMeta: { marginTop: 15, color: C.muted, fontSize: 12 },
  previewTag: { position: "absolute", top: 18, right: 22, zIndex: 15, display: "flex", alignItems: "center", gap: 8, padding: "7px 10px", border: `1px solid rgba(232,235,239,.95)`, borderRadius: 6, background: "rgba(255,255,255,.92)", color: C.secondary, fontSize: 9, fontWeight: 650, letterSpacing: ".07em", boxShadow: "0 2px 8px rgba(20,28,45,.035)" },
  previewDot: { width: 6, height: 6, borderRadius: "50%", background: C.warning },
  captionWrap: { position: "absolute", left: "50%", bottom: 51, zIndex: 20, transform: "translateX(-50%)", minWidth: 490, maxWidth: 1250, background: "rgba(24,29,39,.92)", border: "1px solid rgba(255,255,255,.12)", borderRadius: 9, padding: "14px 24px", boxShadow: "0 10px 30px rgba(0,0,0,.16)", boxSizing: "border-box" },
  captionText: { color: "white", textAlign: "center", fontSize: 22, lineHeight: 1.4, fontWeight: 500, letterSpacing: "-.01em", textShadow: "0 1px 1px rgba(0,0,0,.15)" },
  cursor: { position: "absolute", zIndex: 30, width: 30, height: 38, pointerEvents: "none", filter: "drop-shadow(0 2px 2px rgba(18,24,40,.2))" },
  clickFeedback: { position: "absolute", zIndex: 32, pointerEvents: "none", borderRadius: "50%", border: "2px solid rgba(53,89,216,.78)", background: "rgba(53,89,216,.12)", boxShadow: "0 0 0 3px rgba(255,255,255,.82)" },
  progressTrack: { position: "absolute", bottom: 0, left: 0, right: 0, height: 4, zIndex: 40, background: "rgba(53,89,216,.11)" },
  progressFill: { height: "100%", background: C.brand },
};
