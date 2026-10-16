import jsPDF from "jspdf";
import autoTable from "jspdf-autotable";

// ── Types ─────────────────────────────────────────────────────────────────────

export type ReportType = "portfolio" | "transactions" | "tax";

export interface BrandingOptions {
  companyName?: string;
  primaryColor?: [number, number, number]; // RGB
  logoDataUrl?: string; // base64 PNG/JPEG
}

export interface PortfolioRow {
  asset: string;
  symbol: string;
  quantity: number;
  avgCost: number;
  currentPrice: number;
  totalValue: number;
  gainLoss: number;
  gainLossPct: number;
}

export interface TransactionRow {
  date: string;
  type: string;
  asset: string;
  quantity: number;
  price: number;
  total: number;
  status: string;
}

export interface TaxRow {
  asset: string;
  acquired: string;
  disposed: string;
  proceeds: number;
  costBasis: number;
  gain: number;
  term: "Short" | "Long";
}

export interface GeneratePDFOptions {
  type: ReportType;
  title?: string;
  subtitle?: string;
  branding?: BrandingOptions;
  /** base64 PNG of a chart to embed */
  chartImageDataUrl?: string;
  /** Portfolio report rows */
  portfolioRows?: PortfolioRow[];
  /** Transaction report rows */
  transactionRows?: TransactionRow[];
  /** Tax document rows */
  taxRows?: TaxRow[];
  /** Called periodically with 0-100 progress */
  onProgress?: (pct: number) => void;
}

// ── Helpers ───────────────────────────────────────────────────────────────────

const fmt = (n: number, currency = true) =>
  currency
    ? n.toLocaleString("en-US", { style: "currency", currency: "USD" })
    : n.toLocaleString("en-US");

const pctFmt = (n: number) =>
  `${n >= 0 ? "+" : ""}${n.toFixed(2)}%`;

const DEFAULT_PRIMARY: [number, number, number] = [99, 102, 241]; // indigo-500

// ── Core generator ────────────────────────────────────────────────────────────

export async function generatePDF(options: GeneratePDFOptions): Promise<void> {
  const {
    type,
    title,
    subtitle,
    branding = {},
    chartImageDataUrl,
    portfolioRows = [],
    transactionRows = [],
    taxRows = [],
    onProgress,
  } = options;

  const primary = branding.primaryColor ?? DEFAULT_PRIMARY;
  const companyName = branding.companyName ?? "AssetForge";

  const doc = new jsPDF({ orientation: "portrait", unit: "mm", format: "a4" });
  const W = doc.internal.pageSize.getWidth();
  const H = doc.internal.pageSize.getHeight();

  onProgress?.(5);

  // ── Header bar ──────────────────────────────────────────────────────────────
  doc.setFillColor(...primary);
  doc.rect(0, 0, W, 22, "F");

  // Logo (optional)
  if (branding.logoDataUrl) {
    try {
      doc.addImage(branding.logoDataUrl, "PNG", 8, 4, 14, 14);
    } catch {
      // skip invalid logo
    }
  }

  doc.setTextColor(255, 255, 255);
  doc.setFont("helvetica", "bold");
  doc.setFontSize(13);
  doc.text(companyName, branding.logoDataUrl ? 26 : 8, 13);

  // Report date (right-aligned)
  doc.setFontSize(8);
  doc.setFont("helvetica", "normal");
  const now = new Date().toLocaleDateString("en-US", {
    year: "numeric",
    month: "long",
    day: "numeric",
  });
  doc.text(`Generated ${now}`, W - 8, 13, { align: "right" });

  onProgress?.(15);

  // ── Title area ──────────────────────────────────────────────────────────────
  let y = 32;
  doc.setTextColor(30, 30, 30);
  doc.setFontSize(18);
  doc.setFont("helvetica", "bold");
  const reportTitle =
    title ??
    { portfolio: "Portfolio Report", transactions: "Transaction History", tax: "Tax Document" }[type];
  doc.text(reportTitle, 14, y);

  y += 6;
  if (subtitle) {
    doc.setFontSize(10);
    doc.setFont("helvetica", "normal");
    doc.setTextColor(100, 100, 100);
    doc.text(subtitle, 14, y);
    y += 5;
  }

  // Divider
  y += 2;
  doc.setDrawColor(...primary);
  doc.setLineWidth(0.5);
  doc.line(14, y, W - 14, y);
  y += 6;

  onProgress?.(30);

  // ── Chart (if provided) ─────────────────────────────────────────────────────
  if (chartImageDataUrl) {
    const chartW = W - 28;
    const chartH = 60;
    try {
      doc.addImage(chartImageDataUrl, "PNG", 14, y, chartW, chartH);
      y += chartH + 8;
    } catch {
      // skip bad image
    }
  }

  onProgress?.(50);

  // ── Table ───────────────────────────────────────────────────────────────────
  const headStyle = {
    fillColor: primary,
    textColor: [255, 255, 255] as [number, number, number],
    fontSize: 9,
    fontStyle: "bold" as const,
  };

  if (type === "portfolio" && portfolioRows.length > 0) {
    // Summary stats
    const totalValue = portfolioRows.reduce((s, r) => s + r.totalValue, 0);
    const totalGL = portfolioRows.reduce((s, r) => s + r.gainLoss, 0);

    doc.setFontSize(10);
    doc.setFont("helvetica", "bold");
    doc.setTextColor(30, 30, 30);
    doc.text(`Total Portfolio Value: ${fmt(totalValue)}`, 14, y);
    y += 5;
    doc.setTextColor(totalGL >= 0 ? 16 : 220, totalGL >= 0 ? 185 : 38, totalGL >= 0 ? 129 : 38);
    doc.text(`Total Gain / Loss: ${fmt(totalGL)} (${pctFmt((totalGL / (totalValue - totalGL)) * 100)})`, 14, y);
    y += 8;
    doc.setTextColor(30, 30, 30);

    autoTable(doc, {
      startY: y,
      head: [["Asset", "Symbol", "Qty", "Avg Cost", "Price", "Value", "G/L", "G/L %"]],
      body: portfolioRows.map((r) => [
        r.asset,
        r.symbol,
        fmt(r.quantity, false),
        fmt(r.avgCost),
        fmt(r.currentPrice),
        fmt(r.totalValue),
        fmt(r.gainLoss),
        pctFmt(r.gainLossPct),
      ]),
      headStyles: headStyle,
      alternateRowStyles: { fillColor: [248, 248, 252] },
      styles: { fontSize: 8, cellPadding: 2 },
      // Colour the G/L column red for losses and green for gains.
      didParseCell: (data) => {
        if (data.section === "body" && data.column.index === 6) {
          data.cell.styles.textColor = String(data.cell.raw).startsWith("-")
            ? [220, 38, 38]
            : [16, 185, 129];
        }
      },
    });
  }

  if (type === "transactions" && transactionRows.length > 0) {
    autoTable(doc, {
      startY: y,
      head: [["Date", "Type", "Asset", "Qty", "Price", "Total", "Status"]],
      body: transactionRows.map((r) => [
        r.date,
        r.type,
        r.asset,
        fmt(r.quantity, false),
        fmt(r.price),
        fmt(r.total),
        r.status,
      ]),
      headStyles: headStyle,
      alternateRowStyles: { fillColor: [248, 248, 252] },
      styles: { fontSize: 8, cellPadding: 2 },
    });
  }

  if (type === "tax" && taxRows.length > 0) {
    const totalGain = taxRows.reduce((s, r) => s + r.gain, 0);

    doc.setFontSize(10);
    doc.setFont("helvetica", "bold");
    doc.setTextColor(30, 30, 30);
    doc.text(`Net Capital Gain / Loss: ${fmt(totalGain)}`, 14, y);
    y += 8;

    autoTable(doc, {
      startY: y,
      head: [["Asset", "Acquired", "Disposed", "Proceeds", "Cost Basis", "Gain/Loss", "Term"]],
      body: taxRows.map((r) => [
        r.asset,
        r.acquired,
        r.disposed,
        fmt(r.proceeds),
        fmt(r.costBasis),
        fmt(r.gain),
        r.term,
      ]),
      headStyles: headStyle,
      alternateRowStyles: { fillColor: [248, 248, 252] },
      styles: { fontSize: 8, cellPadding: 2 },
    });
  }

  onProgress?.(85);

  // ── Footer on every page ────────────────────────────────────────────────────
  const pageCount = doc.getNumberOfPages();
  for (let i = 1; i <= pageCount; i++) {
    doc.setPage(i);
    doc.setFillColor(245, 245, 250);
    doc.rect(0, H - 10, W, 10, "F");
    doc.setFontSize(7);
    doc.setTextColor(140, 140, 140);
    doc.text(
      `${companyName} · Confidential · Page ${i} of ${pageCount}`,
      W / 2,
      H - 4,
      { align: "center" }
    );
  }

  onProgress?.(95);

  // ── Save ────────────────────────────────────────────────────────────────────
  const filename = `${companyName.toLowerCase()}-${type}-report-${Date.now()}.pdf`;
  doc.save(filename);

  onProgress?.(100);
}
