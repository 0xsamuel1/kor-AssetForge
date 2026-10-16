"use client";

import * as React from "react";
import { Info, Loader2, Wallet } from "lucide-react";

import { OrderConfirmModal } from "@/components/OrderConfirmModal";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { toast } from "@/lib/toast";
import {
  SLIPPAGE_PRESETS,
  calculateQuote,
  getBestPrice,
  tradingApi,
  type OrderBookSnapshot,
  type OrderReceipt,
  type OrderSide,
  type OrderType,
} from "@/lib/trading";
import type { StellarWallet } from "@/lib/stellar";
import { cn, formatAmount, formatCurrency } from "@/lib/utils";

export interface OrderFormHandle {
  /** Fills the form from an order book click. */
  applyPrice: (price: number, side: OrderSide) => void;
}

interface OrderFormProps {
  assetId: string;
  assetCode: string;
  assetName?: string;
  book: OrderBookSnapshot | null;
  /** Units of the asset the account can sell, when known. */
  availableBalance?: number;
  wallet?: StellarWallet;
  onSubmitted?: (receipt: OrderReceipt) => void;
  ref?: React.Ref<OrderFormHandle>;
  className?: string;
}

/** Segmented control used for both the side and the order type. */
function Segmented<T extends string>({
  options,
  value,
  onChange,
  ariaLabel,
}: {
  options: { value: T; label: string; activeClassName?: string }[];
  value: T;
  onChange: (value: T) => void;
  ariaLabel: string;
}) {
  return (
    <div
      role="group"
      aria-label={ariaLabel}
      className="grid grid-cols-2 gap-1 rounded-lg bg-muted p-1"
    >
      {options.map((option) => (
        <button
          key={option.value}
          type="button"
          onClick={() => onChange(option.value)}
          aria-pressed={value === option.value}
          className={cn(
            "rounded-md px-3 py-1.5 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50",
            value === option.value
              ? (option.activeClassName ?? "bg-background text-foreground shadow-sm")
              : "text-muted-foreground hover:text-foreground",
          )}
        >
          {option.label}
        </button>
      ))}
    </div>
  );
}

function SummaryRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-baseline justify-between gap-4">
      <span className="text-xs text-muted-foreground">{label}</span>
      <span className="font-mono text-xs">{value}</span>
    </div>
  );
}

/**
 * Buy/sell ticket for a single asset.
 *
 * Handles market and limit orders, live cost calculation (fees, slippage,
 * price impact) and routes every submission through a confirmation step.
 */
export function OrderForm({
  assetId,
  assetCode,
  assetName,
  book,
  availableBalance,
  wallet,
  onSubmitted,
  ref,
  className,
}: OrderFormProps) {
  const [side, setSide] = React.useState<OrderSide>("buy");
  const [type, setType] = React.useState<OrderType>("market");
  const [amount, setAmount] = React.useState("");
  const [limitPrice, setLimitPrice] = React.useState("");
  const [slippage, setSlippage] = React.useState<number>(0.005);
  const [isConfirmOpen, setIsConfirmOpen] = React.useState(false);
  const [isSubmitting, setIsSubmitting] = React.useState(false);

  React.useImperativeHandle(
    ref,
    () => ({
      applyPrice: (price, nextSide) => {
        setSide(nextSide);
        setType("limit");
        setLimitPrice(String(price));
      },
    }),
    [],
  );

  const parsedAmount = Number.parseFloat(amount);
  const parsedLimitPrice = Number.parseFloat(limitPrice);
  const hasAmount = Number.isFinite(parsedAmount) && parsedAmount > 0;
  const hasLimitPrice =
    Number.isFinite(parsedLimitPrice) && parsedLimitPrice > 0;

  const bestPrice = getBestPrice(book, side);

  const quote = React.useMemo(
    () =>
      calculateQuote({
        side,
        type,
        amount: hasAmount ? parsedAmount : 0,
        limitPrice: hasLimitPrice ? parsedLimitPrice : bestPrice,
        slippageTolerance: slippage,
        book,
      }),
    [
      side,
      type,
      hasAmount,
      parsedAmount,
      hasLimitPrice,
      parsedLimitPrice,
      bestPrice,
      slippage,
      book,
    ],
  );

  // First blocking problem with the ticket, or null when it's ready to submit.
  const validationError = (() => {
    if (!amount) return null;
    if (!hasAmount) return "Enter an amount greater than zero.";
    if (type === "limit" && !hasLimitPrice) return "Enter a limit price.";
    if (
      side === "sell" &&
      availableBalance !== undefined &&
      parsedAmount > availableBalance
    ) {
      return `You only hold ${formatAmount(availableBalance, 4)} ${assetCode}.`;
    }
    return null;
  })();

  const canSubmit =
    Boolean(wallet?.connected) &&
    hasAmount &&
    (type === "market" || hasLimitPrice) &&
    !validationError &&
    !isSubmitting;

  const handleSubmit = async () => {
    if (!wallet?.connected) return;

    setIsSubmitting(true);
    try {
      const receipt = await tradingApi.submitOrder({
        assetId,
        side,
        type,
        amount: parsedAmount,
        price: quote.averagePrice,
        slippageTolerance: slippage,
        account: wallet.publicKey,
      });

      setIsConfirmOpen(false);
      setAmount("");
      onSubmitted?.(receipt);

      toast.success(
        `${side === "buy" ? "Buy" : "Sell"} order submitted`,
        {
          description: `${formatAmount(parsedAmount, 4)} ${assetCode} · ${receipt.status.replace("_", " ")}`,
        },
      );
    } catch (error) {
      toast.error("Order failed", {
        description:
          error instanceof Error
            ? error.message
            : "The marketplace did not accept this order.",
      });
    } finally {
      setIsSubmitting(false);
    }
  };

  const isBuy = side === "buy";

  return (
    <Card className={className}>
      <CardHeader className="border-b">
        <CardTitle>Trade {assetCode}</CardTitle>
        <CardDescription>
          {bestPrice > 0
            ? `Best ${isBuy ? "ask" : "bid"} ${formatCurrency(bestPrice)}`
            : "No live pricing available"}
        </CardDescription>
      </CardHeader>

      <CardContent className="space-y-4">
        <Segmented
          ariaLabel="Order side"
          value={side}
          onChange={setSide}
          options={[
            {
              value: "buy",
              label: "Buy",
              activeClassName:
                "bg-emerald-500/15 text-emerald-700 dark:text-emerald-400",
            },
            {
              value: "sell",
              label: "Sell",
              activeClassName: "bg-destructive/15 text-destructive",
            },
          ]}
        />

        <Segmented
          ariaLabel="Order type"
          value={type}
          onChange={setType}
          options={[
            { value: "market", label: "Market" },
            { value: "limit", label: "Limit" },
          ]}
        />

        <div className="space-y-1.5">
          <div className="flex items-baseline justify-between">
            <Label htmlFor="order-amount">Amount ({assetCode})</Label>
            {availableBalance !== undefined && (
              <button
                type="button"
                onClick={() => setAmount(String(availableBalance))}
                className="text-xs font-medium text-primary hover:underline"
              >
                Max {formatAmount(availableBalance, 4)}
              </button>
            )}
          </div>
          <Input
            id="order-amount"
            inputMode="decimal"
            placeholder="0.00"
            value={amount}
            onChange={(event) => setAmount(event.target.value)}
            aria-invalid={Boolean(validationError)}
          />
        </div>

        {type === "limit" && (
          <div className="space-y-1.5">
            <Label htmlFor="order-price">Limit price</Label>
            <Input
              id="order-price"
              inputMode="decimal"
              placeholder={bestPrice ? String(bestPrice) : "0.00"}
              value={limitPrice}
              onChange={(event) => setLimitPrice(event.target.value)}
            />
          </div>
        )}

        <div className="space-y-1.5">
          <Label>Slippage tolerance</Label>
          <div className="flex flex-wrap gap-1.5">
            {SLIPPAGE_PRESETS.map((preset) => (
              <button
                key={preset}
                type="button"
                onClick={() => setSlippage(preset)}
                aria-pressed={slippage === preset}
                className={cn(
                  "rounded-md px-2.5 py-1 text-xs font-medium ring-1 transition-colors focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/50",
                  slippage === preset
                    ? "bg-primary text-primary-foreground ring-transparent"
                    : "ring-foreground/10 hover:bg-muted",
                )}
              >
                {(preset * 100).toFixed(preset < 0.01 ? 1 : 0)}%
              </button>
            ))}
          </div>
        </div>

        {validationError && (
          <p role="alert" className="text-xs text-destructive">
            {validationError}
          </p>
        )}

        {/* Live calculator — updates on every keystroke and book update. */}
        <div className="space-y-1.5 rounded-lg bg-muted/40 p-3">
          <SummaryRow
            label={type === "market" ? "Average price" : "Limit price"}
            value={quote.averagePrice ? formatCurrency(quote.averagePrice) : "—"}
          />
          <SummaryRow
            label="Platform fee"
            value={formatCurrency(quote.platformFee)}
          />
          <SummaryRow
            label="Network fee"
            value={`${formatAmount(quote.networkFee, 5)} XLM`}
          />
          <SummaryRow
            label="Price impact"
            value={`${(quote.priceImpact * 100).toFixed(2)}%`}
          />
          <SummaryRow
            label={isBuy ? "Total to pay" : "Total to receive"}
            value={formatCurrency(quote.total)}
          />
        </div>

        {hasAmount && !quote.hasEnoughLiquidity && (
          <p className="flex items-start gap-1.5 text-xs text-amber-600 dark:text-amber-400">
            <Info className="mt-0.5 size-3.5 shrink-0" aria-hidden="true" />
            Only {formatAmount(quote.fillableAmount, 2)} {assetCode} is available
            at the moment — the rest may rest on the book.
          </p>
        )}

        {wallet?.connected ? (
          <Button
            className="w-full"
            onClick={() => setIsConfirmOpen(true)}
            disabled={!canSubmit}
          >
            {isSubmitting && (
              <Loader2 className="size-4 animate-spin" aria-hidden="true" />
            )}
            {isBuy ? "Buy" : "Sell"} {assetCode}
          </Button>
        ) : (
          <p className="flex items-center justify-center gap-2 rounded-lg bg-muted p-3 text-xs text-muted-foreground">
            <Wallet className="size-3.5" aria-hidden="true" />
            Connect a wallet to place orders.
          </p>
        )}
      </CardContent>

      <OrderConfirmModal
        open={isConfirmOpen}
        onOpenChange={setIsConfirmOpen}
        quote={hasAmount ? quote : null}
        assetCode={assetCode}
        assetName={assetName}
        isSubmitting={isSubmitting}
        onConfirm={handleSubmit}
      />
    </Card>
  );
}
