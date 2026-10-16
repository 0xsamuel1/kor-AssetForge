"use client";

import { Loader2, TriangleAlert } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { HIGH_IMPACT_THRESHOLD, type TradeQuote } from "@/lib/trading";
import { cn, formatAmount, formatCurrency } from "@/lib/utils";

interface OrderConfirmModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  quote: TradeQuote | null;
  assetCode: string;
  assetName?: string;
  isSubmitting: boolean;
  onConfirm: () => void;
}

function SummaryRow({
  label,
  value,
  emphasis = false,
}: {
  label: string;
  value: string;
  emphasis?: boolean;
}) {
  return (
    <div className="flex items-baseline justify-between gap-4 py-1.5">
      <span className="text-xs text-muted-foreground">{label}</span>
      <span
        className={cn(
          "text-right font-mono text-sm",
          emphasis && "font-semibold",
        )}
      >
        {value}
      </span>
    </div>
  );
}

/** Formats seconds as a short human estimate ("~10s", "~1 min"). */
function formatEta(seconds: number): string {
  if (seconds < 60) return `~${seconds}s`;
  return `~${Math.round(seconds / 60)} min`;
}

/**
 * Final review step before an order is submitted: exact amounts, fees, the
 * slippage limit that will be enforced, and warnings for thin liquidity or
 * heavy price impact.
 */
export function OrderConfirmModal({
  open,
  onOpenChange,
  quote,
  assetCode,
  assetName,
  isSubmitting,
  onConfirm,
}: OrderConfirmModalProps) {
  if (!quote) return null;

  const isBuy = quote.side === "buy";
  const isHighImpact = quote.priceImpact >= HIGH_IMPACT_THRESHOLD;

  return (
    <Dialog open={open} onOpenChange={isSubmitting ? () => {} : onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>
            Confirm {isBuy ? "buy" : "sell"} order
          </DialogTitle>
          <DialogDescription>
            {quote.type === "market" ? "Market" : "Limit"} order for{" "}
            {assetName ?? assetCode}. Review the numbers before signing.
          </DialogDescription>
        </DialogHeader>

        {(isHighImpact || !quote.hasEnoughLiquidity) && (
          <div
            role="alert"
            className="flex items-start gap-2 rounded-lg bg-amber-500/10 p-3 text-sm text-amber-700 dark:text-amber-400"
          >
            <TriangleAlert className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
            <p>
              {!quote.hasEnoughLiquidity
                ? `The visible book only covers ${formatAmount(quote.fillableAmount, 2)} ${assetCode}. The remainder may stay unfilled.`
                : `This order moves the price by ${(quote.priceImpact * 100).toFixed(2)}%. Consider a smaller size or a limit order.`}
            </p>
          </div>
        )}

        <div className="divide-y rounded-lg bg-muted/40 px-3 py-1">
          <SummaryRow
            label="Amount"
            value={`${formatAmount(quote.amount, 4)} ${assetCode}`}
          />
          <SummaryRow
            label={quote.type === "market" ? "Average price" : "Limit price"}
            value={formatCurrency(quote.averagePrice)}
          />
          <SummaryRow label="Subtotal" value={formatCurrency(quote.subtotal)} />
          <SummaryRow
            label={`Platform fee (${(quote.platformFee / (quote.subtotal || 1) * 100).toFixed(2)}%)`}
            value={formatCurrency(quote.platformFee)}
          />
          <SummaryRow
            label="Network fee"
            value={`${formatAmount(quote.networkFee, 5)} XLM`}
          />
          <SummaryRow
            label={isBuy ? "Total to pay" : "Total to receive"}
            value={formatCurrency(quote.total)}
            emphasis
          />
        </div>

        <div className="divide-y rounded-lg px-3 py-1 ring-1 ring-foreground/10">
          <SummaryRow
            label="Price impact"
            value={`${(quote.priceImpact * 100).toFixed(2)}%`}
          />
          <SummaryRow
            label={isBuy ? "Maximum price" : "Minimum price"}
            value={formatCurrency(quote.worstPrice)}
          />
          <SummaryRow
            label={isBuy ? "Maximum spend" : "Minimum proceeds"}
            value={formatCurrency(quote.slippageLimit)}
          />
          <SummaryRow
            label="Estimated completion"
            value={formatEta(quote.estimatedSeconds)}
          />
        </div>

        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={isSubmitting}
          >
            Cancel
          </Button>
          <Button onClick={onConfirm} disabled={isSubmitting} aria-busy={isSubmitting}>
            {isSubmitting && (
              <Loader2 className="size-4 animate-spin" aria-hidden="true" />
            )}
            {isSubmitting
              ? "Submitting…"
              : `Confirm ${isBuy ? "purchase" : "sale"}`}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
