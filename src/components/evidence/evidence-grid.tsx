"use client";

import * as React from "react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Badge } from "@/components/ui/badge";
import {
  FileText,
  ClipboardList,
  Truck,
  Camera,
  Landmark,
  FileCheck2,
} from "lucide-react";
import { cn } from "@/lib/utils";

export interface EvidenceDoc {
  id: string;
  kind: string;
  name: string;
  createdAt: string;
  extractedFields?: Record<string, unknown> | null;
  deliveryRef?: string | null;
  deliveryType?: string | null;
  deliveredAt?: string | null;
  deliveryLines?: { item: string; quantity: number }[] | null;
  photoNote?: string | null;
  poNumber?: string | null;
}

const KIND_ICON: Record<string, React.ElementType> = {
  INVOICE_PDF: FileText,
  CORRECTED_INVOICE: FileCheck2,
  PURCHASE_ORDER: ClipboardList,
  DELIVERY_NOTE: Truck,
  DELIVERY_PHOTO: Camera,
  TAX_DOC: Landmark,
};

const KIND_LABEL: Record<string, string> = {
  INVOICE_PDF: "Invoice PDF",
  CORRECTED_INVOICE: "Corrected invoice",
  PURCHASE_ORDER: "Purchase order",
  DELIVERY_NOTE: "Delivery note",
  DELIVERY_PHOTO: "Delivery photo",
  TAX_DOC: "Tax document",
};

export function EvidenceGrid({ documents }: { documents: EvidenceDoc[] }) {
  const [openDoc, setOpenDoc] = React.useState<EvidenceDoc | null>(null);
  return (
    <>
      <ul className="grid grid-cols-2 gap-2.5 sm:grid-cols-3 xl:grid-cols-5">
        {documents.map((doc) => {
          const Icon = KIND_ICON[doc.kind] ?? FileText;
          return (
            <li key={doc.id}>
              <button
                type="button"
                onClick={() => setOpenDoc(doc)}
                aria-label={`Open ${KIND_LABEL[doc.kind] ?? doc.kind}: ${doc.name}`}
                className="flex w-full flex-col items-start gap-2 rounded-lg border border-border bg-surface px-3.5 py-3 text-left shadow-xs transition-colors hover:border-brand/40 hover:bg-brand-soft/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/30"
              >
                <Icon className="h-4 w-4 text-[#8f97a3]" aria-hidden="true" />
                <span className="text-[12.5px] font-medium leading-tight text-foreground">
                  {KIND_LABEL[doc.kind] ?? doc.kind}
                </span>
                <span className="truncate font-mono text-[10.5px] text-muted-foreground">{doc.name}</span>
              </button>
            </li>
          );
        })}
        {documents.length === 0 && (
          <li className="col-span-full rounded-lg border border-dashed px-4 py-6 text-center text-sm text-muted-foreground">
            No evidence documents attached yet.
          </li>
        )}
      </ul>
      <EvidenceDialog doc={openDoc} onClose={() => setOpenDoc(null)} />
    </>
  );
}

function FieldRow({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex items-baseline justify-between gap-4 border-b border-dashed py-1.5 last:border-0">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="text-sm font-medium tabular-nums">{value}</dd>
    </div>
  );
}

function EvidenceDialog({ doc, onClose }: { doc: EvidenceDoc | null; onClose: () => void }) {
  if (!doc) return null;
  const isPhoto = doc.kind === "DELIVERY_PHOTO";
  const fields = (doc.extractedFields ?? {}) as Record<string, unknown>;
  return (
    <Dialog open={Boolean(doc)} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2 text-base">
            {KIND_LABEL[doc.kind] ?? doc.kind}
            <Badge variant="secondary" className="font-mono text-[11px]">
              {doc.name}
            </Badge>
          </DialogTitle>
          <DialogDescription>
            {isPhoto ? "Photo captured at goods receipt." : "Mock document preview with extracted fields."}
          </DialogDescription>
        </DialogHeader>
        <div className="max-h-[60vh] overflow-y-auto">
          {isPhoto ? (
            <figure className="flex flex-col gap-3">
              <div className="flex aspect-[4/3] items-center justify-center rounded-lg border bg-gradient-to-br from-muted to-muted/40 text-muted-foreground">
                <div className="flex flex-col items-center gap-2">
                  <Camera className="h-8 w-8" aria-hidden="true" />
                  <span className="text-xs font-medium">delivery_photo.jpg · 1.2 MB</span>
                </div>
              </div>
              {doc.photoNote && (
                <figcaption className="text-sm text-muted-foreground">
                  <span className="font-medium text-foreground">Warehouse note: </span>
                  {doc.photoNote}
                </figcaption>
              )}
            </figure>
          ) : (
            <div className={cn("mx-auto max-w-md rounded-md border bg-white px-6 py-5 text-zinc-900 shadow-sm dark:bg-zinc-50")}>
              <div className="mb-3 flex items-center justify-between border-b border-dashed border-zinc-300 pb-2">
                <span className="text-[11px] font-semibold uppercase tracking-widest text-zinc-500">
                  {doc.kind === "PURCHASE_ORDER"
                    ? "Purchase order"
                    : doc.kind === "DELIVERY_NOTE"
                      ? "Goods received note"
                      : "Invoice"}
                </span>
                <span className="font-mono text-xs text-zinc-500">{doc.name}</span>
              </div>
              <dl>
                {Object.entries(fields).length > 0
                  ? Object.entries(fields).map(([k, v]) => (
                      <FieldRow key={k} label={titleize(k)} value={formatValue(v)} />
                    ))
                  : null}
                {doc.poNumber && <FieldRow label="PO number" value={<span className="font-mono">{doc.poNumber}</span>} />}
                {doc.deliveryRef && <FieldRow label="Reference" value={<span className="font-mono">{doc.deliveryRef}</span>} />}
                {doc.deliveryType && <FieldRow label="Event type" value={doc.deliveryType.replace(/_/g, " ").toLowerCase()} />}
                {doc.deliveredAt && <FieldRow label="Delivered at" value={formatDateSafe(doc.deliveredAt)} />}
                {doc.deliveryLines?.map((l, i) => (
                  <FieldRow key={i} label={`Line ${i + 1} — ${l.item}`} value={`${l.quantity.toLocaleString()} units`} />
                ))}
              </dl>
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}

function titleize(key: string): string {
  return key
    .replace(/([A-Z])/g, " $1")
    .replace(/_/g, " ")
    .replace(/^./, (c) => c.toUpperCase())
    .trim();
}

function formatValue(v: unknown): React.ReactNode {
  if (v === null || v === undefined) return "—";
  if (typeof v === "number") {
    const isMoneyish = v % 1 !== 0 || v > 1000;
    return isMoneyish ? `$${v.toLocaleString("en-US", { maximumFractionDigits: 2 })}` : v.toLocaleString();
  }
  if (typeof v === "string" && /^\d{4}-\d{2}-\d{2}/.test(v)) return formatDateSafe(v);
  return String(v);
}

function formatDateSafe(v: string): string {
  try {
    return new Intl.DateTimeFormat("en-US", { month: "short", day: "numeric", year: "numeric" }).format(new Date(v));
  } catch {
    return v;
  }
}
