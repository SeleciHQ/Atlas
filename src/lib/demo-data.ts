import { db } from "../lib/db";
import { analyzeCase, type AgentCaseInput } from "../lib/agent";
import { DEFAULT_POLICY_CONFIG } from "../types/policy";
import type { ExceptionType, CaseStatus, InvoiceStatus } from "../types/ontology";

const DAY = 86_400_000;
const HOUR = 3_600_000;

function daysAgo(days: number, hour = 10, minute = 0): Date {
  const d = new Date(Date.now() - days * DAY);
  d.setHours(hour, minute, 0, 0);
  if (d.getTime() > Date.now()) d.setDate(d.getDate() - 1);
  return d;
}

function hoursAgo(hours: number): Date {
  return new Date(Date.now() - hours * HOUR);
}

interface SeedDeliveryDef {
  reference: string;
  type: string;
  daysAgo: number;
  hasPhoto?: boolean;
  photoNote?: string;
  lines: { lineNo: number; item: string; quantity: number }[];
}

interface SeedShipmentDef {
  reference: string;
  poNumber: string;
  status: string;
  carrier: string;
  shippedDaysAgo: number;
  deliveries: SeedDeliveryDef[];
}

interface SeedPoDef {
  number: string;
  supplierCode: string;
  status: string;
  issuedDaysAgo: number;
  expectedDeliveryDaysAgo: number;
  lines: { lineNo: number; item: string; sku: string; quantity: number; unitPrice: number }[];
}

interface SeedInvoiceDef {
  number: string;
  supplierCode: string;
  poNumber: string | null;
  daysAgo: number;
  status: InvoiceStatus;
  ocrStatus?: string;
  erpStatus?: string | null;
  duplicateOf?: string | null;
  taxDocMissing?: boolean;
  lines: { lineNo: number; item: string; invoicedQuantity: number; unitPrice: number }[];
}

interface SeedDiscrepancyDef {
  reference: string;
  type: ExceptionType;
  severity: string;
  description: string;
  status: string;
}

interface SeedResolutionDef {
  type: string;
  note: string;
  actorName?: string;
  hoursAgo: number;
}

interface SeedApprovalDef {
  reference: string;
  status: string;
  rationale: string;
  decidedByName?: string;
  decidedHoursAgo?: number;
  comment?: string;
  createdHoursAgo: number;
}

interface SeedCaseDef {
  invoiceNumber: string;
  status: CaseStatus;
  exceptionType: ExceptionType;
  daysAgo: number;
  escalated?: boolean;
  discrepancies: SeedDiscrepancyDef[];
  resolutions?: SeedResolutionDef[];
  approval?: SeedApprovalDef;
  resolvedHoursAgo?: number;
}

const SUPPLIERS = [
  {
    code: "SUP-001",
    name: "Kigali Metals",
    country: "Rwanda",
    status: "ACTIVE",
    contactEmail: "procurement@kigalimetals.rw",
    notes: "Industrial steel and cement supplier. Two open payment disputes.",
  },
  {
    code: "SUP-002",
    name: "Douala Packaging",
    country: "Cameroon",
    status: "ACTIVE",
    contactEmail: "ap@doualapack.cm",
    notes: "Packaging materials. Price escalations on box contracts in Q3.",
  },
  {
    code: "SUP-003",
    name: "BuildCo",
    country: "Kenya",
    status: "PROBATION",
    contactEmail: "billing@buildco.co.ke",
    notes: "Construction materials. Under supplier probation due to repeated invoicing errors.",
  },
  {
    code: "SUP-004",
    name: "Sahel Logistics",
    country: "Mali",
    status: "ON_HOLD",
    contactEmail: "invoices@sahellogistics.ml",
    notes: "Freight and logistics. Payments on hold pending contract renegotiation.",
  },
  {
    code: "SUP-005",
    name: "Atlantic Freight",
    country: "Ghana",
    status: "ACTIVE",
    contactEmail: "finance@atlanticfreight.gh",
    notes: "Container freight. Reliable on-time record.",
  },
];

const PURCHASE_ORDERS: SeedPoDef[] = [
  {
    number: "PO-2291",
    supplierCode: "SUP-001",
    status: "CLOSED",
    issuedDaysAgo: 12,
    expectedDeliveryDaysAgo: 5,
    lines: [
      { lineNo: 1, item: "Steel rods", sku: "KM-SR-100", quantity: 100, unitPrice: 100.0 },
      { lineNo: 2, item: "Cement bags", sku: "KM-CB-50", quantity: 50, unitPrice: 41.5 },
    ],
  },
  {
    number: "PO-2298",
    supplierCode: "SUP-001",
    status: "CLOSED",
    issuedDaysAgo: 20,
    expectedDeliveryDaysAgo: 16,
    lines: [{ lineNo: 1, item: "Hydraulic valves", sku: "KM-HV-32", quantity: 40, unitPrice: 118.75 }],
  },
  {
    number: "PO-2301",
    supplierCode: "SUP-001",
    status: "CLOSED",
    issuedDaysAgo: 17,
    expectedDeliveryDaysAgo: 15,
    lines: [{ lineNo: 1, item: "Angle brackets", sku: "KM-AB-08", quantity: 500, unitPrice: 9.3 }],
  },
  {
    number: "PO-2279",
    supplierCode: "SUP-001",
    status: "CLOSED",
    issuedDaysAgo: 30,
    expectedDeliveryDaysAgo: 27,
    lines: [{ lineNo: 1, item: "Container freight", sku: "AF-CF-01", quantity: 1, unitPrice: 5400 }],
  },
  {
    number: "PO-2305",
    supplierCode: "SUP-002",
    status: "OPEN",
    issuedDaysAgo: 15,
    expectedDeliveryDaysAgo: 12,
    lines: [
      { lineNo: 1, item: "Corrugated boxes", sku: "DP-BX-A3", quantity: 2000, unitPrice: 1.35 },
      { lineNo: 2, item: "Pallets", sku: "DP-PL-02", quantity: 200, unitPrice: 2.0 },
    ],
  },
  {
    number: "PO-2312",
    supplierCode: "SUP-002",
    status: "OPEN",
    issuedDaysAgo: 6,
    expectedDeliveryDaysAgo: 4,
    lines: [
      { lineNo: 1, item: "Corrugated boxes", sku: "DP-BX-B2", quantity: 5000, unitPrice: 3.4 },
      { lineNo: 2, item: "Labels", sku: "DP-LB-01", quantity: 20000, unitPrice: 0.08 },
    ],
  },
  {
    number: "PO-2310",
    supplierCode: "SUP-003",
    status: "OPEN",
    issuedDaysAgo: 10,
    expectedDeliveryDaysAgo: 7,
    lines: [
      { lineNo: 1, item: "Rebar", sku: "BC-RB-12", quantity: 500, unitPrice: 4.6 },
      { lineNo: 2, item: "Safety helmets", sku: "BC-SH-02", quantity: 200, unitPrice: 4.8 },
    ],
  },
  {
    number: "PO-2304",
    supplierCode: "SUP-003",
    status: "CLOSED",
    issuedDaysAgo: 16,
    expectedDeliveryDaysAgo: 13,
    lines: [
      { lineNo: 1, item: "Rebar", sku: "BC-RB-12", quantity: 1000, unitPrice: 11.4 },
      { lineNo: 2, item: "Welded mesh", sku: "BC-WM-02", quantity: 200, unitPrice: 14.5 },
    ],
  },
  {
    number: "PO-2309",
    supplierCode: "SUP-003",
    status: "CLOSED",
    issuedDaysAgo: 8,
    expectedDeliveryDaysAgo: 3,
    lines: [
      { lineNo: 1, item: "Safety helmets", sku: "BC-SH-02", quantity: 1500, unitPrice: 4.8 },
      { lineNo: 2, item: "Nitrile gloves", sku: "BC-NG-01", quantity: 50, unitPrice: 1.0 },
    ],
  },
  {
    number: "PO-2293",
    supplierCode: "SUP-003",
    status: "OPEN",
    issuedDaysAgo: 5,
    expectedDeliveryDaysAgo: 3,
    lines: [{ lineNo: 1, item: "Welded mesh", sku: "BC-WM-02", quantity: 250, unitPrice: 9.2 }],
  },
  {
    number: "PO-2285",
    supplierCode: "SUP-003",
    status: "CLOSED",
    issuedDaysAgo: 20,
    expectedDeliveryDaysAgo: 17,
    lines: [{ lineNo: 1, item: "Safety helmets", sku: "BC-SH-02", quantity: 1000, unitPrice: 4.8 }],
  },
  {
    number: "PO-2288",
    supplierCode: "SUP-004",
    status: "CLOSED",
    issuedDaysAgo: 25,
    expectedDeliveryDaysAgo: 20,
    lines: [{ lineNo: 1, item: "Freight - Lagos to Bamako", sku: "SL-FR-01", quantity: 1, unitPrice: 6100 }],
  },
  {
    number: "PO-2281",
    supplierCode: "SUP-004",
    status: "CLOSED",
    issuedDaysAgo: 28,
    expectedDeliveryDaysAgo: 25,
    lines: [{ lineNo: 1, item: "Freight - Tema to Kano", sku: "SL-FR-02", quantity: 1, unitPrice: 1150 }],
  },
  {
    number: "PO-2313",
    supplierCode: "SUP-004",
    status: "OPEN",
    issuedDaysAgo: 7,
    expectedDeliveryDaysAgo: 5,
    lines: [{ lineNo: 1, item: "Freight - Casablanca to Ouagadougou", sku: "SL-FR-03", quantity: 1, unitPrice: 21000 }],
  },
  {
    number: "PO-2306",
    supplierCode: "SUP-005",
    status: "OPEN",
    issuedDaysAgo: 9,
    expectedDeliveryDaysAgo: 6,
    lines: [{ lineNo: 1, item: "Container freight", sku: "AF-CF-01", quantity: 2, unitPrice: 4450 }],
  },
  {
    number: "PO-2315",
    supplierCode: "SUP-005",
    status: "OPEN",
    issuedDaysAgo: 1,
    expectedDeliveryDaysAgo: 0,
    lines: [{ lineNo: 1, item: "Container freight", sku: "AF-CF-01", quantity: 1, unitPrice: 6700 }],
  },
];

const SHIPMENTS: SeedShipmentDef[] = [
  {
    reference: "SHP-4417",
    poNumber: "PO-2291",
    status: "DELIVERED",
    carrier: "Rwanda Freight Co",
    shippedDaysAgo: 9,
    deliveries: [
      {
        reference: "GRN-882",
        type: "PARTIAL_DELIVERY",
        daysAgo: 5,
        hasPhoto: true,
        photoNote: "Steel rods partial delivery, 80 bundles stacked at receiving bay 2.",
        lines: [{ lineNo: 1, item: "Steel rods", quantity: 80 }],
      },
      {
        reference: "GRN-875",
        type: "DELIVERY",
        daysAgo: 6,
        lines: [{ lineNo: 1, item: "Cement bags", quantity: 50 }],
      },
    ],
  },
  {
    reference: "SHP-4420",
    poNumber: "PO-2298",
    status: "DELIVERED",
    carrier: "Rwanda Freight Co",
    shippedDaysAgo: 18,
    deliveries: [
      {
        reference: "GRN-861",
        type: "DELIVERY",
        daysAgo: 16,
        lines: [{ lineNo: 1, item: "Hydraulic valves", quantity: 40 }],
      },
    ],
  },
  {
    reference: "SHP-4415",
    poNumber: "PO-2301",
    status: "DELIVERED",
    carrier: "Rwanda Freight Co",
    shippedDaysAgo: 13,
    deliveries: [
      {
        reference: "GRN-864",
        type: "DELIVERY",
        daysAgo: 11,
        lines: [{ lineNo: 1, item: "Angle brackets", quantity: 500 }],
      },
    ],
  },
  {
    reference: "SHP-4402",
    poNumber: "PO-2279",
    status: "DELIVERED",
    carrier: "Atlantic Freight",
    shippedDaysAgo: 28,
    deliveries: [
      {
        reference: "GRN-850",
        type: "DELIVERY",
        daysAgo: 27,
        hasPhoto: true,
        photoNote: "Container offloaded at yard; count confirmed with driver.",
        lines: [{ lineNo: 1, item: "Container freight", quantity: 1 }],
      },
    ],
  },
  {
    reference: "SHP-4424",
    poNumber: "PO-2305",
    status: "DELIVERED",
    carrier: "Douala Road Transport",
    shippedDaysAgo: 13,
    deliveries: [
      {
        reference: "GRN-866",
        type: "DELIVERY",
        daysAgo: 12,
        hasPhoto: true,
        photoNote: "Full load received; boxes stacked on 8 pallets.",
        lines: [
          { lineNo: 1, item: "Corrugated boxes", quantity: 2000 },
          { lineNo: 2, item: "Pallets", quantity: 200 },
        ],
      },
    ],
  },
  {
    reference: "SHP-4426",
    poNumber: "PO-2312",
    status: "DELIVERED",
    carrier: "Douala Road Transport",
    shippedDaysAgo: 4,
    deliveries: [
      {
        reference: "GRN-873",
        type: "DELIVERY",
        daysAgo: 4,
        hasPhoto: true,
        photoNote: "Corrected count confirmed after recount with warehouse lead.",
        lines: [
          { lineNo: 1, item: "Corrugated boxes", quantity: 5000 },
          { lineNo: 2, item: "Labels", quantity: 20000 },
        ],
      },
    ],
  },
  {
    reference: "SHP-4428",
    poNumber: "PO-2310",
    status: "DELIVERED",
    carrier: "Mombasa Inland Rail",
    shippedDaysAgo: 8,
    deliveries: [
      {
        reference: "GRN-870",
        type: "DELIVERY",
        daysAgo: 7,
        lines: [
          { lineNo: 1, item: "Rebar", quantity: 500 },
          { lineNo: 2, item: "Safety helmets", quantity: 200 },
        ],
      },
    ],
  },
  {
    reference: "SHP-4422",
    poNumber: "PO-2304",
    status: "DELIVERED",
    carrier: "Mombasa Inland Rail",
    shippedDaysAgo: 14,
    deliveries: [
      {
        reference: "GRN-869",
        type: "DELIVERY",
        daysAgo: 13,
        lines: [
          { lineNo: 1, item: "Rebar", quantity: 1000 },
          { lineNo: 2, item: "Welded mesh", quantity: 150 },
        ],
      },
    ],
  },
  {
    reference: "SHP-4425",
    poNumber: "PO-2309",
    status: "DELIVERED",
    carrier: "Mombasa Inland Rail",
    shippedDaysAgo: 8,
    deliveries: [
      {
        reference: "GRN-877",
        type: "DELIVERY",
        daysAgo: 3,
        lines: [
          { lineNo: 1, item: "Safety helmets", quantity: 1500 },
          { lineNo: 2, item: "Nitrile gloves", quantity: 50 },
        ],
      },
    ],
  },
  {
    reference: "SHP-4430",
    poNumber: "PO-2293",
    status: "DELIVERED",
    carrier: "Mombasa Inland Rail",
    shippedDaysAgo: 3,
    deliveries: [
      {
        reference: "GRN-878",
        type: "DELIVERY",
        daysAgo: 3,
        hasPhoto: true,
        photoNote: "Mesh bundles on pallets 1-4 at loading dock B.",
        lines: [{ lineNo: 1, item: "Welded mesh", quantity: 200 }],
      },
    ],
  },
  {
    reference: "SHP-4412",
    poNumber: "PO-2285",
    status: "DELIVERED",
    carrier: "Mombasa Inland Rail",
    shippedDaysAgo: 18,
    deliveries: [
      {
        reference: "GRN-860",
        type: "DELIVERY",
        daysAgo: 17,
        lines: [{ lineNo: 1, item: "Safety helmets", quantity: 1000 }],
      },
    ],
  },
  {
    reference: "SHP-4410",
    poNumber: "PO-2288",
    status: "DELIVERED",
    carrier: "Sahel Logistics",
    shippedDaysAgo: 27,
    deliveries: [
      {
        reference: "GRN-858",
        type: "DELIVERY",
        daysAgo: 4,
        lines: [{ lineNo: 1, item: "Freight - Lagos to Bamako", quantity: 1 }],
      },
    ],
  },
  {
    reference: "SHP-4405",
    poNumber: "PO-2281",
    status: "DELIVERED",
    carrier: "Sahel Logistics",
    shippedDaysAgo: 26,
    deliveries: [
      {
        reference: "GRN-855",
        type: "DELIVERY",
        daysAgo: 25,
        lines: [{ lineNo: 1, item: "Freight - Tema to Kano", quantity: 1 }],
      },
    ],
  },
  {
    reference: "SHP-4427",
    poNumber: "PO-2313",
    status: "DELIVERED",
    carrier: "Sahel Logistics",
    shippedDaysAgo: 5,
    deliveries: [
      {
        reference: "GRN-874",
        type: "DELIVERY",
        daysAgo: 5,
        lines: [{ lineNo: 1, item: "Freight - Casablanca to Ouagadougou", quantity: 1 }],
      },
    ],
  },
  {
    reference: "SHP-4421",
    poNumber: "PO-2306",
    status: "PARTIAL",
    carrier: "Atlantic Freight",
    shippedDaysAgo: 8,
    deliveries: [
      {
        reference: "GRN-868",
        type: "PARTIAL_DELIVERY",
        daysAgo: 6,
        lines: [{ lineNo: 1, item: "Container freight", quantity: 1 }],
      },
    ],
  },
  {
    reference: "SHP-4431",
    poNumber: "PO-2315",
    status: "IN_TRANSIT",
    carrier: "Atlantic Freight",
    shippedDaysAgo: 0,
    deliveries: [],
  },
];

const INVOICES: SeedInvoiceDef[] = [
  // --- Cases ---
  {
    number: "INV-4821",
    supplierCode: "SUP-001",
    poNumber: "PO-2291",
    daysAgo: 2,
    status: "IN_REVIEW",
    lines: [
      { lineNo: 1, item: "Steel rods", invoicedQuantity: 100, unitPrice: 104.0 },
      { lineNo: 2, item: "Cement bags", invoicedQuantity: 50, unitPrice: 41.6 },
    ],
  },
  {
    number: "INV-4688",
    supplierCode: "SUP-002",
    poNumber: "PO-2305",
    daysAgo: 1,
    status: "IN_REVIEW",
    lines: [
      { lineNo: 1, item: "Corrugated boxes", invoicedQuantity: 2000, unitPrice: 1.42 },
      { lineNo: 2, item: "Pallets", invoicedQuantity: 200, unitPrice: 2.0 },
    ],
  },
  {
    number: "INV-4790",
    supplierCode: "SUP-003",
    poNumber: "PO-2310",
    daysAgo: 1,
    status: "IN_REVIEW",
    taxDocMissing: true,
    lines: [{ lineNo: 1, item: "Safety helmets", invoicedQuantity: 200, unitPrice: 4.9 }],
  },
  {
    number: "INV-4827",
    supplierCode: "SUP-002",
    poNumber: null,
    daysAgo: 1,
    status: "IN_REVIEW",
    lines: [{ lineNo: 1, item: "Pallets", invoicedQuantity: 1200, unitPrice: 1.25 }],
  },
  {
    number: "INV-4556",
    supplierCode: "SUP-004",
    poNumber: "PO-2288",
    daysAgo: 6,
    status: "WAITING_SUPPLIER",
    lines: [{ lineNo: 1, item: "Freight - Lagos to Bamako", invoicedQuantity: 1, unitPrice: 6100 }],
  },
  {
    number: "INV-4702",
    supplierCode: "SUP-001",
    poNumber: "PO-2301",
    daysAgo: 3,
    status: "WAITING_SUPPLIER",
    lines: [{ lineNo: 1, item: "Angle brackets", invoicedQuantity: 500, unitPrice: 9.5 }],
  },
  {
    number: "INV-4733",
    supplierCode: "SUP-005",
    poNumber: "PO-2306",
    daysAgo: 2,
    status: "WAITING_SUPPLIER",
    lines: [{ lineNo: 1, item: "Container freight", invoicedQuantity: 2, unitPrice: 4450 }],
  },
  {
    number: "INV-4601",
    supplierCode: "SUP-003",
    poNumber: "PO-2304",
    daysAgo: 5,
    status: "WAITING_SUPPLIER",
    lines: [
      { lineNo: 1, item: "Rebar", invoicedQuantity: 1000, unitPrice: 11.4 },
      { lineNo: 2, item: "Welded mesh", invoicedQuantity: 200, unitPrice: 14.5 },
    ],
  },
  {
    number: "INV-4788",
    supplierCode: "SUP-002",
    poNumber: "PO-2312",
    daysAgo: 3,
    status: "HELD",
    lines: [
      { lineNo: 1, item: "Corrugated boxes", invoicedQuantity: 5000, unitPrice: 3.4 },
      { lineNo: 2, item: "Labels", invoicedQuantity: 20000, unitPrice: 0.08 },
    ],
  },
  {
    number: "INV-4802",
    supplierCode: "SUP-004",
    poNumber: "PO-2313",
    daysAgo: 2,
    status: "HELD",
    lines: [{ lineNo: 1, item: "Freight - Casablanca to Ouagadougou", invoicedQuantity: 1, unitPrice: 21600 }],
  },
  {
    number: "INV-4715",
    supplierCode: "SUP-003",
    poNumber: "PO-2309",
    daysAgo: 4,
    status: "HELD",
    lines: [
      { lineNo: 1, item: "Safety helmets", invoicedQuantity: 1500, unitPrice: 4.8 },
      { lineNo: 2, item: "Nitrile gloves", invoicedQuantity: 50, unitPrice: 1.0 },
    ],
  },
  {
    number: "INV-4611",
    supplierCode: "SUP-001",
    poNumber: "PO-2279",
    daysAgo: 4,
    status: "PAID",
    erpStatus: "PAYMENT_RELEASED",
    lines: [{ lineNo: 1, item: "Container freight", invoicedQuantity: 1, unitPrice: 5400 }],
  },
  {
    number: "INV-4660",
    supplierCode: "SUP-004",
    poNumber: "PO-2281",
    daysAgo: 3,
    status: "REJECTED",
    duplicateOf: "INV-4659",
    erpStatus: "NOT_POSTED",
    lines: [{ lineNo: 1, item: "Freight - Tema to Kano", invoicedQuantity: 1, unitPrice: 1150 }],
  },
  {
    number: "INV-4741",
    supplierCode: "SUP-003",
    poNumber: "PO-2293",
    daysAgo: 3,
    status: "HELD",
    lines: [{ lineNo: 1, item: "Welded mesh", invoicedQuantity: 250, unitPrice: 9.2 }],
  },
  // --- Invoices without cases ---
  {
    number: "INV-4659",
    supplierCode: "SUP-004",
    poNumber: "PO-2281",
    daysAgo: 27,
    status: "PAID",
    erpStatus: "PAYMENT_RELEASED",
    lines: [{ lineNo: 1, item: "Freight - Tema to Kano", invoicedQuantity: 1, unitPrice: 1150 }],
  },
  {
    number: "INV-4760",
    supplierCode: "SUP-003",
    poNumber: "PO-2285",
    daysAgo: 16,
    status: "PAID",
    erpStatus: "PAYMENT_RELEASED",
    lines: [{ lineNo: 1, item: "Safety helmets", invoicedQuantity: 1000, unitPrice: 4.8 }],
  },
  {
    number: "INV-4830",
    supplierCode: "SUP-005",
    poNumber: "PO-2315",
    daysAgo: 0,
    status: "RECEIVED",
    lines: [{ lineNo: 1, item: "Container freight", invoicedQuantity: 1, unitPrice: 6700 }],
  },
];

const CASES: SeedCaseDef[] = [
  {
    invoiceNumber: "INV-4821",
    status: "OPEN",
    exceptionType: "QUANTITY_MISMATCH",
    daysAgo: 2,
    discrepancies: [
      {
        reference: "DIS-311",
        type: "QUANTITY_MISMATCH",
        severity: "HIGH",
        description: "Invoice bills 100 steel rods; delivery GRN-882 confirms 80 units received.",
        status: "OPEN",
      },
      {
        reference: "DIS-313",
        type: "PRICE_VARIANCE",
        severity: "MEDIUM",
        description: "Steel rod unit price $104.00 vs PO price $100.00 (4.0% variance, above 2% tolerance).",
        status: "OPEN",
      },
    ],
  },
  {
    invoiceNumber: "INV-4688",
    status: "OPEN",
    exceptionType: "PRICE_VARIANCE",
    daysAgo: 1,
    discrepancies: [
      {
        reference: "DIS-318",
        type: "PRICE_VARIANCE",
        severity: "HIGH",
        description: "Corrugated box price $1.42 vs PO price $1.35 (5.2% variance, above 2% tolerance).",
        status: "OPEN",
      },
    ],
  },
  {
    invoiceNumber: "INV-4790",
    status: "OPEN",
    exceptionType: "MISSING_TAX_DOC",
    daysAgo: 1,
    discrepancies: [
      {
        reference: "DIS-321",
        type: "MISSING_TAX_DOC",
        severity: "MEDIUM",
        description: "VAT certificate not attached. Supplier is tax-registered in Kenya.",
        status: "OPEN",
      },
    ],
  },
  {
    invoiceNumber: "INV-4827",
    status: "ESCALATED",
    exceptionType: "UNAPPROVED",
    daysAgo: 1,
    escalated: true,
    discrepancies: [
      {
        reference: "DIS-322",
        type: "UNAPPROVED",
        severity: "HIGH",
        description: "No purchase order on file for this invoice. Procurement has no matching requisition.",
        status: "OPEN",
      },
    ],
    resolutions: [
      {
        type: "ESCALATED",
        note: "Escalated to procurement for PO verification.",
        actorName: "Amina Diallo",
        hoursAgo: 3,
      },
    ],
  },
  {
    invoiceNumber: "INV-4556",
    status: "WAITING_SUPPLIER",
    exceptionType: "LATE_DELIVERY",
    daysAgo: 6,
    discrepancies: [
      {
        reference: "DIS-301",
        type: "LATE_DELIVERY",
        severity: "LOW",
        description: "Delivery arrived 16 days after the expected date on PO-2288.",
        status: "OPEN",
      },
    ],
    resolutions: [
      {
        type: "DOCUMENT_REQUESTED",
        note: "Requested proof of delivery and delay explanation from Sahel Logistics.",
        actorName: "Amina Diallo",
        hoursAgo: 96,
      },
    ],
  },
  {
    invoiceNumber: "INV-4702",
    status: "WAITING_SUPPLIER",
    exceptionType: "LATE_DELIVERY",
    daysAgo: 3,
    discrepancies: [
      {
        reference: "DIS-305",
        type: "LATE_DELIVERY",
        severity: "MEDIUM",
        description: "Angle brackets delivered 4 days after the expected date on PO-2301.",
        status: "OPEN",
      },
    ],
    resolutions: [
      {
        type: "DOCUMENT_REQUESTED",
        note: "Requested delivery explanation from Kigali Metals.",
        actorName: "Amina Diallo",
        hoursAgo: 48,
      },
    ],
  },
  {
    invoiceNumber: "INV-4733",
    status: "WAITING_SUPPLIER",
    exceptionType: "PARTIAL_DELIVERY",
    daysAgo: 2,
    discrepancies: [
      {
        reference: "DIS-308",
        type: "PARTIAL_DELIVERY",
        severity: "MEDIUM",
        description: "1 of 2 containers delivered; second container still in transit on SHP-4421.",
        status: "OPEN",
      },
    ],
    resolutions: [
      {
        type: "DOCUMENT_REQUESTED",
        note: "Requested expected delivery date for the remaining container.",
        actorName: "Amina Diallo",
        hoursAgo: 24,
      },
    ],
  },
  {
    invoiceNumber: "INV-4601",
    status: "WAITING_SUPPLIER",
    exceptionType: "QUANTITY_MISMATCH",
    daysAgo: 5,
    discrepancies: [
      {
        reference: "DIS-303",
        type: "QUANTITY_MISMATCH",
        severity: "HIGH",
        description: "Invoiced 200 welded mesh; GRN-869 confirms 150 received.",
        status: "OPEN",
      },
    ],
    resolutions: [
      {
        type: "DOCUMENT_REQUESTED",
        note: "Requested corrected invoice from BuildCo.",
        actorName: "Amina Diallo",
        hoursAgo: 96,
      },
    ],
  },
  {
    invoiceNumber: "INV-4788",
    status: "READY_FOR_APPROVAL",
    exceptionType: "QUANTITY_MISMATCH",
    daysAgo: 3,
    discrepancies: [
      {
        reference: "DIS-309",
        type: "QUANTITY_MISMATCH",
        severity: "HIGH",
        description: "Invoiced 5,000 corrugated boxes; initial delivery note showed 4,600 received.",
        status: "RESOLVED",
      },
    ],
    resolutions: [
      {
        type: "CORRECTED_INVOICE_RECEIVED",
        note: "Corrected invoice received; warehouse recount confirmed 5,000 boxes delivered.",
        actorName: "Amina Diallo",
        hoursAgo: 20,
      },
    ],
    approval: {
      reference: "APR-112",
      status: "PENDING",
      rationale: "Quantity variance was resolved with corrected invoice. Amount exceeds the $5,000 approval threshold.",
      createdHoursAgo: 18,
    },
  },
  {
    invoiceNumber: "INV-4802",
    status: "READY_FOR_APPROVAL",
    exceptionType: "PRICE_VARIANCE",
    daysAgo: 2,
    discrepancies: [
      {
        reference: "DIS-310",
        type: "PRICE_VARIANCE",
        severity: "MEDIUM",
        description: "Freight surcharge $21,600 vs contracted $21,000 (2.9% variance, above 2% tolerance).",
        status: "RESOLVED",
      },
    ],
    resolutions: [
      {
        type: "CORRECTED_INVOICE_RECEIVED",
        note: "Supplier issued a credit note acknowledging the surcharge; invoice stands as billed.",
        actorName: "Amina Diallo",
        hoursAgo: 21,
      },
    ],
    approval: {
      reference: "APR-113",
      status: "PENDING",
      rationale: "Price variance above tolerance was reviewed and accepted with supplier credit note. Amount exceeds the $5,000 approval threshold.",
      createdHoursAgo: 19,
    },
  },
  {
    invoiceNumber: "INV-4715",
    status: "READY_FOR_APPROVAL",
    exceptionType: "MISSING_DELIVERY",
    daysAgo: 4,
    discrepancies: [
      {
        reference: "DIS-314",
        type: "MISSING_DELIVERY",
        severity: "HIGH",
        description: "No delivery event on file for SHP-4425 at time of review.",
        status: "RESOLVED",
      },
    ],
    resolutions: [
      {
        type: "CORRECTED_INVOICE_RECEIVED",
        note: "Delivery record GRN-877 located in warehouse system; quantities confirmed.",
        actorName: "Amina Diallo",
        hoursAgo: 22,
      },
    ],
    approval: {
      reference: "APR-114",
      status: "PENDING",
      rationale: "Missing delivery record was located and quantities confirmed. Amount exceeds the $5,000 approval threshold.",
      createdHoursAgo: 17,
    },
  },
  {
    invoiceNumber: "INV-4611",
    status: "RESOLVED",
    exceptionType: "MISSING_DELIVERY",
    daysAgo: 4,
    resolvedHoursAgo: 2,
    discrepancies: [
      {
        reference: "DIS-307",
        type: "MISSING_DELIVERY",
        severity: "HIGH",
        description: "No goods received note on file for SHP-4402 at time of review.",
        status: "RESOLVED",
      },
    ],
    resolutions: [
      {
        type: "PAYMENT_RELEASED",
        note: "Delivery record located; payment approved by controller and released in ERP.",
        actorName: "Jean Baptiste",
        hoursAgo: 2,
      },
    ],
    approval: {
      reference: "APR-111",
      status: "APPROVED",
      rationale: "Missing delivery record was located and quantities confirmed. Amount exceeds the $5,000 approval threshold.",
      decidedByName: "Jean Baptiste",
      decidedHoursAgo: 2,
      comment: "Delivery confirmed by warehouse. Proceed with payment.",
      createdHoursAgo: 26,
    },
  },
  {
    invoiceNumber: "INV-4660",
    status: "RESOLVED",
    exceptionType: "DUPLICATE_INVOICE",
    daysAgo: 3,
    resolvedHoursAgo: 1,
    discrepancies: [
      {
        reference: "DIS-316",
        type: "DUPLICATE_INVOICE",
        severity: "HIGH",
        description: "Exact duplicate of INV-4659, which was paid on the same PO-2281.",
        status: "RESOLVED",
      },
    ],
    resolutions: [
      {
        type: "INVOICE_REJECTED",
        note: "Invoice rejected as duplicate. Supplier notified.",
        actorName: "Amina Diallo",
        hoursAgo: 1,
      },
    ],
  },
  {
    invoiceNumber: "INV-4741",
    status: "ESCALATED",
    exceptionType: "QUANTITY_MISMATCH",
    daysAgo: 3,
    escalated: true,
    discrepancies: [
      {
        reference: "DIS-315",
        type: "QUANTITY_MISMATCH",
        severity: "HIGH",
        description: "Invoiced 250 welded mesh; GRN-878 confirms 200 received.",
        status: "OPEN",
      },
    ],
    resolutions: [
      {
        type: "ESCALATED",
        note: "Escalated to procurement after supplier disputed the delivery count.",
        actorName: "Amina Diallo",
        hoursAgo: 26,
      },
    ],
  },
];

function evidenceCountFor(invoiceNumber: string, deliveries: { hasPhoto?: boolean }[]): number {
  const invoice = INVOICES.find((i) => i.number === invoiceNumber);
  let count = invoice?.poNumber ? 2 : 1; // invoice + PO
  count += deliveries.length > 0 ? 1 : 0; // delivery note
  count += deliveries.some((d) => d.hasPhoto) ? 1 : 0; // photo
  if (invoice?.taxDocMissing) count -= 0;
  return count;
}

function agentInputFor(
  invoice: SeedInvoiceDef,
  caseDef: SeedCaseDef,
  po: SeedPoDef | undefined,
  shipment: SeedShipmentDef | undefined
): AgentCaseInput {
  const deliveries = (shipment?.deliveries ?? []).map((d) => ({
    reference: d.reference,
    type: d.type,
    lines: d.lines.map((l) => ({ item: l.item, quantity: l.quantity })),
  }));
  return {
    invoiceNumber: invoice.number,
    exceptionType: caseDef.exceptionType,
    caseStatus: caseDef.status,
    amount: invoice.lines.reduce((sum, l) => sum + l.invoicedQuantity * l.unitPrice, 0),
    currency: "USD",
    poNumber: po?.number ?? null,
    poLines: (po?.lines ?? []).map((l) => ({ item: l.item, quantity: l.quantity, unitPrice: l.unitPrice })),
    invoiceLines: invoice.lines.map((l) => ({
      item: l.item,
      invoicedQuantity: l.invoicedQuantity,
      unitPrice: l.unitPrice,
    })),
    deliveries,
    discrepancyTypes: caseDef.discrepancies.map((d) => d.type),
    supplierPriorIssues: priorIssuesFor(invoice.supplierCode, invoice.number),
    taxDocMissing: Boolean(invoice.taxDocMissing),
    duplicateOf: invoice.duplicateOf ?? null,
    evidenceCount: evidenceCountFor(invoice.number, shipment?.deliveries ?? []),
  };
}

function priorIssuesFor(supplierCode: string, excludeInvoice: string): number {
  return CASES.filter(
    (c) => {
      const inv = INVOICES.find((i) => i.number === c.invoiceNumber);
      return inv && inv.supplierCode === supplierCode && c.invoiceNumber !== excludeInvoice && c.invoiceNumber !== "INV-4741";
    }
  ).length;
}

interface AuditEventSeed {
  actor: string;
  actorType: "HUMAN" | "AGENT" | "SYSTEM";
  action: string;
  objectType: string;
  objectId: string;
  inputs?: Record<string, unknown>;
  outputs?: Record<string, unknown>;
  createdAt: Date;
}

function timelineFor(
  caseDef: SeedCaseDef,
  invoice: SeedInvoiceDef,
  po: SeedPoDef | undefined,
  shipment: SeedShipmentDef | undefined,
  agent: ReturnType<typeof analyzeCase>
): AuditEventSeed[] {
  const base = daysAgo(caseDef.daysAgo, 9, caseDef.daysAgo % 7 + 2);
  const events: AuditEventSeed[] = [];
  const push = (
    minutes: number,
    actorType: AuditEventSeed["actorType"],
    actor: string,
    action: string,
    objectType: string,
    objectId: string,
    inputs?: Record<string, unknown>,
    outputs?: Record<string, unknown>
  ) => events.push({ actor, actorType, action, objectType, objectId, inputs, outputs, createdAt: new Date(base.getTime() + minutes * 60_000) });

  push(0, "SYSTEM", "Atlas Ingest", "invoice.received", "invoice", invoice.number, {
    source: invoice.supplierCode === "SUP-003" ? "sftp://invoices/buildco" : "email:ap@supplier.inbox",
    file: `${invoice.number}.pdf`,
  });
  push(1, "AGENT", "Atlas Agent", "ocr.extraction_completed", "invoice", invoice.number, {
    fields: 7,
    confidence: 0.98,
  });
  if (po) {
    push(2, "AGENT", "Atlas Agent", "invoice.matched_to_po", "purchase_order", po.number, {
      matchScore: 0.94,
      supplier: invoice.supplierCode,
    });
  } else {
    push(2, "AGENT", "Atlas Agent", "invoice.po_match_failed", "invoice", invoice.number, {
      candidates: 0,
    });
  }
  const deliveryRefs = shipment?.deliveries.map((d) => d.reference) ?? [];
  if (deliveryRefs.length > 0) {
    push(3, "AGENT", "Atlas Agent", "delivery.linked", "delivery_event", deliveryRefs[0], {
      linkedRefs: deliveryRefs,
    });
  }
  push(4, "AGENT", "Atlas Agent", "discrepancy.detected", "case", invoice.number, {
    types: caseDef.discrepancies.map((d) => d.type),
  });
  for (const d of caseDef.discrepancies) {
    push(5, "SYSTEM", "Atlas Engine", "discrepancy.created", "discrepancy", d.reference, {
      type: d.type,
      severity: d.severity,
    });
  }
  push(6, "AGENT", "Atlas Agent", "agent.context_requested", "case", invoice.number, {
    requested: ["supplier_history", "policy", "delivery_events"],
  });
  push(7, "SYSTEM", "Atlas Engine", "policy.read_allowed", "case", invoice.number, {
    scope: ["invoice", "purchase_order", "delivery_event", "supplier_history"],
  });
  push(8, "AGENT", "Atlas Agent", "agent.action_proposed", "case", invoice.number, {
    recommendedAction: agent.recommendedAction,
    recommendation: agent.recommendation,
    confidence: agent.confidence,
  });
  push(9, "SYSTEM", "Atlas Engine", "policy.evaluated", "case", invoice.number, {
    action: "approve_payment",
    result: agent.recommendedAction === "approve_payment" ? "allowed_with_approval" : "blocked",
    policyVersion: DEFAULT_POLICY_CONFIG.version,
  });

  const st = caseDef.status;
  if (st === "WAITING_SUPPLIER" && caseDef.resolutions) {
    const res = caseDef.resolutions[0];
    push(15, "HUMAN", res.actorName ?? "Amina Diallo", "action.approved", "case", invoice.number, {
      action: "request_document",
    });
    push(16, "SYSTEM", "Atlas Outbound", "supplier.document_request_sent", "supplier", invoice.supplierCode, {
      method: "email",
      to: SUPPLIERS.find((s) => s.code === invoice.supplierCode)?.contactEmail,
    });
  }
  if (st === "READY_FOR_APPROVAL") {
    push(20, "SYSTEM", "Atlas Ingest", "corrected_invoice.received", "invoice", invoice.number, {
      note: "Corrected document attached to case.",
    });
    push(21, "SYSTEM", "Atlas Engine", "discrepancy.resolved", "discrepancy", caseDef.discrepancies[0].reference, {
      resolvedBy: "corrected_document",
    });
    if (caseDef.approval) {
      push(25, "HUMAN", "Amina Diallo", "payment.proposed_for_approval", "approval", caseDef.approval.reference, {
        action: "approve_payment",
      });
      push(26, "SYSTEM", "Atlas Engine", "policy.controller_approval_required", "approval", caseDef.approval.reference, {
        reason: "amount_above_threshold",
      });
      push(27, "SYSTEM", "Atlas Engine", "approval.created", "approval", caseDef.approval.reference, {
        amount: invoice.lines.reduce((s, l) => s + l.invoicedQuantity * l.unitPrice, 0),
      });
    }
  }
  if (st === "ESCALATED" && caseDef.resolutions) {
    const res = caseDef.resolutions[0];
    push(15, "HUMAN", res.actorName ?? "Amina Diallo", "case.escalated", "case", invoice.number, {
      team: "procurement",
    });
    push(16, "SYSTEM", "Atlas Outbound", "procurement.notified", "case", invoice.number, {
      channel: "email",
    });
  }

  // Tail events that happened recently (today), e.g. resolutions and approvals decided today.
  const tailBase = caseDef.resolvedHoursAgo ? hoursAgo(caseDef.resolvedHoursAgo) : null;
  if (tailBase && st === "RESOLVED") {
    const t = (minutes: number, actorType: AuditEventSeed["actorType"], actor: string, action: string, objectType: string, objectId: string, inputs?: Record<string, unknown>, outputs?: Record<string, unknown>) =>
      events.push({ actor, actorType, action, objectType, objectId, inputs, outputs, createdAt: new Date(tailBase.getTime() - 30 * 60_000 + minutes * 60_000) });
    if (caseDef.approval?.status === "APPROVED") {
      t(0, "HUMAN", caseDef.approval.decidedByName ?? "Jean Baptiste", "approval.decided", "approval", caseDef.approval.reference, {
        decision: "APPROVED",
        comment: caseDef.approval.comment,
      });
      t(2, "SYSTEM", "Atlas ERP Adapter", "erp.payment_released", "invoice", invoice.number, {
        erpDocument: `ERP-PAY-${invoice.number.slice(-4)}`,
      });
      t(4, "SYSTEM", "Atlas Engine", "invoice.marked_paid", "invoice", invoice.number, {
        status: "PAID",
      });
    } else if (caseDef.resolutions?.[0].type === "INVOICE_REJECTED") {
      const res = caseDef.resolutions[0];
      t(0, "HUMAN", res.actorName ?? "Amina Diallo", "action.executed", "case", invoice.number, {
        action: "reject_invoice",
      });
      t(2, "SYSTEM", "Atlas Outbound", "supplier.notified", "supplier", invoice.supplierCode, {
        method: "email",
        reason: "duplicate_invoice_rejected",
      });
    }
  }

  return events.sort((a, b) => a.createdAt.getTime() - b.createdAt.getTime());
}

export async function resetAndSeed(): Promise<void> {
  await db.auditEvent.deleteMany();
  await db.resolution.deleteMany();
  await db.approval.deleteMany();
  await db.discrepancy.deleteMany();
  await db.document.deleteMany();
  await db.case.deleteMany();
  await db.invoiceLine.deleteMany();
  await db.invoice.deleteMany();
  await db.deliveryEventLine.deleteMany();
  await db.deliveryEvent.deleteMany();
  await db.shipment.deleteMany();
  await db.purchaseOrderLine.deleteMany();
  await db.purchaseOrder.deleteMany();
  await db.supplier.deleteMany();
  await db.user.deleteMany();
  await db.policy.deleteMany();

  await db.user.createMany({
    data: [
      { id: "usr_amina", name: "Amina Diallo", email: "amina@atlas.demo", role: "FINANCE_OPS" },
      { id: "usr_jean", name: "Jean Baptiste", email: "jean@atlas.demo", role: "CONTROLLER" },
      { id: "usr_agent", name: "Atlas Agent", email: "agent@atlas.demo", role: "AGENT" },
    ],
  });

  await db.policy.create({
    data: {
      key: DEFAULT_POLICY_CONFIG.key,
      name: DEFAULT_POLICY_CONFIG.name,
      version: DEFAULT_POLICY_CONFIG.version,
      config: JSON.stringify(DEFAULT_POLICY_CONFIG),
      active: true,
    },
  });

  const supplierMap = new Map<string, string>();
  for (const s of SUPPLIERS) {
    const created = await db.supplier.create({ data: s });
    supplierMap.set(s.code, created.id);
  }

  const poMap = new Map<string, { id: string; def: SeedPoDef }>();
  for (const po of PURCHASE_ORDERS) {
    const total = po.lines.reduce((sum, l) => sum + l.quantity * l.unitPrice, 0);
    const created = await db.purchaseOrder.create({
      data: {
        number: po.number,
        supplierId: supplierMap.get(po.supplierCode)!,
        status: po.status,
        currency: "USD",
        totalAmount: total,
        issuedAt: daysAgo(po.issuedDaysAgo, 11),
        expectedDeliveryAt: daysAgo(po.expectedDeliveryDaysAgo, 17),
      },
    });
    poMap.set(po.number, { id: created.id, def: po });
    for (const line of po.lines) {
      await db.purchaseOrderLine.create({
        data: {
          poId: created.id,
          lineNo: line.lineNo,
          item: line.item,
          sku: line.sku,
          quantity: line.quantity,
          unitPrice: line.unitPrice,
        },
      });
    }
  }

  const shipmentMap = new Map<string, { id: string; def: SeedShipmentDef }>();
  for (const sh of SHIPMENTS) {
    const po = poMap.get(sh.poNumber)!;
    const created = await db.shipment.create({
      data: {
        reference: sh.reference,
        poId: po.id,
        supplierId: supplierMap.get(po.def.supplierCode)!,
        status: sh.status,
        carrier: sh.carrier,
        shippedAt: sh.shippedDaysAgo >= 0 ? daysAgo(sh.shippedDaysAgo, 8) : null,
      },
    });
    shipmentMap.set(sh.reference, { id: created.id, def: sh });
    for (const d of sh.deliveries) {
      const delivery = await db.deliveryEvent.create({
        data: {
          reference: d.reference,
          shipmentId: created.id,
          type: d.type,
          deliveredAt: daysAgo(d.daysAgo, 14),
          hasPhoto: Boolean(d.hasPhoto),
          photoNote: d.photoNote ?? null,
        },
      });
      for (const line of d.lines) {
        await db.deliveryEventLine.create({
          data: {
            deliveryEventId: delivery.id,
            lineNo: line.lineNo,
            item: line.item,
            quantity: line.quantity,
          },
        });
      }
    }
  }

  const invoiceMap = new Map<string, { id: string; def: SeedInvoiceDef }>();
  for (const inv of INVOICES) {
    const amount = inv.lines.reduce((sum, l) => sum + l.invoicedQuantity * l.unitPrice, 0);
    const created = await db.invoice.create({
      data: {
        number: inv.number,
        supplierId: supplierMap.get(inv.supplierCode)!,
        poId: inv.poNumber ? poMap.get(inv.poNumber)!.id : null,
        amount,
        currency: "USD",
        invoiceDate: daysAgo(inv.daysAgo, 9),
        dueDate: daysAgo(inv.daysAgo - 30, 9),
        status: inv.status,
        ocrStatus: inv.ocrStatus ?? "COMPLETE",
        duplicateOf: inv.duplicateOf ?? null,
        taxDocMissing: Boolean(inv.taxDocMissing),
        erpStatus: inv.erpStatus ?? "NOT_POSTED",
      },
    });
    invoiceMap.set(inv.number, { id: created.id, def: inv });
    for (const line of inv.lines) {
      await db.invoiceLine.create({
        data: {
          invoiceId: created.id,
          lineNo: line.lineNo,
          item: line.item,
          invoicedQuantity: line.invoicedQuantity,
          unitPrice: line.unitPrice,
          lineTotal: line.invoicedQuantity * line.unitPrice,
        },
      });
    }
  }

  for (const caseDef of CASES) {
    const inv = invoiceMap.get(caseDef.invoiceNumber)!;
    const amount = inv.def.lines.reduce((sum, l) => sum + l.invoicedQuantity * l.unitPrice, 0);
    const shipment = inv.def.poNumber
      ? SHIPMENTS.find((s) => s.poNumber === inv.def.poNumber)
      : undefined;
    const po = inv.def.poNumber ? poMap.get(inv.def.poNumber)!.def : undefined;
    const agent = analyzeCase(agentInputFor(inv.def, caseDef, po, shipment ?? undefined), DEFAULT_POLICY_CONFIG);

    const createdCase = await db.case.create({
      data: {
        invoiceId: inv.id,
        supplierId: supplierMap.get(inv.def.supplierCode)!,
        status: caseDef.status,
        exceptionType: caseDef.exceptionType,
        title: `${caseDef.exceptionType.replace(/_/g, " ").toLowerCase()} on ${inv.def.number}`,
        summary: summaryFor(caseDef, inv.def),
        amount,
        currency: "USD",
        highValue: amount > DEFAULT_POLICY_CONFIG.approvalThreshold,
        escalated: Boolean(caseDef.escalated),
        createdAt: daysAgo(caseDef.daysAgo, 9, (caseDef.daysAgo % 7) + 2),
        updatedAt: caseDef.resolvedHoursAgo ? hoursAgo(caseDef.resolvedHoursAgo) : hoursAgo(1),
        resolvedAt: caseDef.resolvedHoursAgo ? hoursAgo(caseDef.resolvedHoursAgo) : null,
      },
    });

    for (const d of caseDef.discrepancies) {
      await db.discrepancy.create({
        data: {
          reference: d.reference,
          caseId: createdCase.id,
          type: d.type,
          severity: d.severity,
          description: d.description,
          status: d.status,
        },
      });
    }

    // Evidence documents
    await db.document.create({
      data: {
        kind: "INVOICE_PDF",
        name: `${inv.def.number}.pdf`,
        caseId: createdCase.id,
        invoiceId: inv.id,
        supplierId: supplierMap.get(inv.def.supplierCode),
        extractedFields: JSON.stringify({
          supplier: SUPPLIERS.find((s) => s.code === inv.def.supplierCode)?.name,
          invoiceNumber: inv.def.number,
          amount,
          currency: "USD",
          quantity: inv.def.lines.reduce((s, l) => s + l.invoicedQuantity, 0),
          unitPrice: inv.def.lines[0].unitPrice,
          invoiceDate: daysAgo(inv.def.daysAgo, 9).toISOString().slice(0, 10),
          ocrConfidence: 0.98,
        }),
        createdAt: daysAgo(inv.def.daysAgo, 9),
      },
    });
    if (po) {
      await db.document.create({
        data: {
          kind: "PURCHASE_ORDER",
          name: `${po.number}.pdf`,
          caseId: createdCase.id,
          supplierId: supplierMap.get(inv.def.supplierCode),
          poId: poMap.get(po.number)!.id,
          createdAt: daysAgo(po.issuedDaysAgo, 11),
        },
      });
    }
    if (shipment) {
      const first = shipment.deliveries[0];
      if (first) {
        await db.document.create({
          data: {
            kind: "DELIVERY_NOTE",
            name: `${first.reference}.pdf`,
            caseId: createdCase.id,
            supplierId: supplierMap.get(inv.def.supplierCode),
            shipmentId: shipmentMap.get(shipment.reference)!.id,
            createdAt: daysAgo(first.daysAgo, 14),
          },
        });
        if (first.hasPhoto) {
          await db.document.create({
            data: {
              kind: "DELIVERY_PHOTO",
              name: `${first.reference}_photo.jpg`,
              caseId: createdCase.id,
              supplierId: supplierMap.get(inv.def.supplierCode),
              shipmentId: shipmentMap.get(shipment.reference)!.id,
              createdAt: daysAgo(first.daysAgo, 14, 5),
            },
          });
        }
      }
    }
    if (caseDef.status === "READY_FOR_APPROVAL") {
      await db.document.create({
        data: {
          kind: "CORRECTED_INVOICE",
          name: `${inv.def.number}_corrected.pdf`,
          caseId: createdCase.id,
          invoiceId: inv.id,
          supplierId: supplierMap.get(inv.def.supplierCode),
          createdAt: hoursAgo(20),
        },
      });
    }

    // Resolutions
    for (const r of caseDef.resolutions ?? []) {
      const actor = await db.user.findFirst({ where: { name: r.actorName } });
      await db.resolution.create({
        data: {
          caseId: createdCase.id,
          type: r.type,
          note: r.note,
          actorId: actor?.id ?? null,
          createdAt: hoursAgo(r.hoursAgo),
        },
      });
    }

    // Approvals
    if (caseDef.approval) {
      let decidedById: string | null = null;
      if (caseDef.approval.decidedByName) {
        decidedById = (await db.user.findFirst({ where: { name: caseDef.approval.decidedByName } }))?.id ?? null;
      }
      await db.approval.create({
        data: {
          reference: caseDef.approval.reference,
          caseId: createdCase.id,
          action: "approve_payment",
          amount,
          currency: "USD",
          status: caseDef.approval.status,
          rationale: caseDef.approval.rationale,
          requestedBy: "Amina Diallo",
          decidedById,
          comment: caseDef.approval.comment ?? null,
          decidedAt: caseDef.approval.decidedHoursAgo ? hoursAgo(caseDef.approval.decidedHoursAgo) : null,
          createdAt: hoursAgo(caseDef.approval.createdHoursAgo),
        },
      });
    }

    // Audit timeline
    const timeline = timelineFor(caseDef, inv.def, po, shipment, agent);
    for (const e of timeline) {
      await db.auditEvent.create({
        data: {
          actor: e.actor,
          actorType: e.actorType,
          action: e.action,
          objectType: e.objectType,
          objectId: e.objectId,
          caseId: createdCase.id,
          inputs: JSON.stringify(e.inputs ?? {}),
          outputs: JSON.stringify(e.outputs ?? {}),
          policyVersion: DEFAULT_POLICY_CONFIG.version,
          createdAt: e.createdAt,
        },
      });
    }
  }
}

function summaryFor(caseDef: SeedCaseDef, invoice: SeedInvoiceDef): string {
  switch (caseDef.exceptionType) {
    case "QUANTITY_MISMATCH": {
      const shipment = SHIPMENTS.find((s) => s.poNumber === invoice.poNumber);
      const invoiced = invoice.lines.reduce((s, l) => s + l.invoicedQuantity, 0);
      const received = (shipment?.deliveries ?? []).flatMap((d) => d.lines).reduce((s, l) => s + l.quantity, 0);
      const item = invoice.lines[0]?.item ?? "items";
      return `Invoice bills ${formatQty(invoiced)} ${item.toLowerCase()}, but delivery confirms ${formatQty(received)} units. The invoice also has a 4% price variance, above the 2% allowed tolerance.`;
    }
    case "PRICE_VARIANCE":
      return "Unit prices on this invoice exceed the contracted price on the purchase order by more than the 2% allowed tolerance.";
    case "MISSING_DELIVERY":
      return "No delivery record could be matched to this invoice at the time of review, so received quantities cannot be confirmed.";
    case "DUPLICATE_INVOICE":
      return "This invoice matches an invoice that was already recorded and paid for the same purchase order.";
    case "LATE_DELIVERY":
      return "The delivery linked to this invoice arrived well after the expected date on the purchase order.";
    case "MISSING_TAX_DOC":
      return "The supplier is tax-registered but did not attach the required VAT certificate to this invoice.";
    case "UNAPPROVED":
      return "No purchase order could be matched to this invoice, so it cannot be tied to an approved requisition.";
    case "PARTIAL_DELIVERY":
      return "The shipment linked to this invoice was delivered in part; the remaining balance is still in transit.";
  }
}

function formatQty(q: number): string {
  return q.toLocaleString("en-US");
}
