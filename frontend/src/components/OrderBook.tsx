"use client";

import { RefreshCw, TriangleAlert } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { SkeletonTable } from "@/components/ui/skeleton";
import {
  getSpread,
  withCumulativeTotals,
  type OrderBookSnapshot,
  type OrderSide,
} from "@/lib/trading";
import { cn, formatAmount } from "@/lib/utils";

interface OrderBookProps {
  book: OrderBookSnapshot | null;
  isLoading: boolean;
  error?: string | null;
  /** Retry handler shown with the error state. */
  onRetry?: () => void;
  /** Clicking a level pushes its price into the order form. */
  onSelectPrice?: (price: number, side: OrderSide) => void;
  assetCode?: string;
  /** Levels rendered per side (default: 8). */
  maxLevels?: number;
  className?: string;
}

function LevelRow({
  price,
  amount,
  total,
  depth,
  side,
  onSelect,
}: {
  price: number;
  amount: number;
  total: number;
  depth: number;
  side: OrderSide;
  onSelect?: () => void;
}) {
  const isBid = side === "buy";

  return (
    <button
      type="button"
      onClick={onSelect}
      disabled={!onSelect}
      // The depth bar is a background layer so the numbers stay readable.
      className="relative grid w-full grid-cols-3 gap-2 px-2 py-1 text-right font-mono text-xs transition-colors hover:bg-muted/60 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50 disabled:pointer-events-none"
      aria-label={`${isBid ? "Bid" : "Ask"} ${formatAmount(amount, 2)} at ${formatAmount(price, 4)}`}
    >
      <span
        aria-hidden="true"
        className={cn(
          "absolute inset-y-0 right-0 -z-0",
          isBid ? "bg-emerald-500/10" : "bg-destructive/10",
        )}
        style={{ width: `${Math.max(depth * 100, 2)}%` }}
      />
      <span
        className={cn(
          "z-10 text-left",
          isBid
            ? "text-emerald-600 dark:text-emerald-400"
            : "text-destructive",
        )}
      >
        {formatAmount(price, 4)}
      </span>
      <span className="z-10">{formatAmount(amount, 2)}</span>
      <span className="z-10 text-muted-foreground">{formatAmount(total, 2)}</span>
    </button>
  );
}

/**
 * Live order book for a single asset: asks on top, bids below, with the spread
 * in the middle and depth bars behind each level.
 */
export function OrderBook({
  book,
  isLoading,
  error,
  onRetry,
  onSelectPrice,
  assetCode = "units",
  maxLevels = 8,
  className,
}: OrderBookProps) {
  const spread = getSpread(book);

  // Asks are rendered worst-price-first so the best ask sits next to the spread.
  const asks = withCumulativeTotals(
    (book?.asks ?? []).slice(0, maxLevels),
  ).reverse();
  const bids = withCumulativeTotals((book?.bids ?? []).slice(0, maxLevels));

  const isEmpty = !isLoading && !error && asks.length === 0 && bids.length === 0;

  return (
    <Card className={className}>
      <CardHeader className="border-b">
        <CardTitle>Order book</CardTitle>
        <CardDescription>
          {book && !isLoading
            ? `Spread ${formatAmount(spread.absolute, 4)} (${(spread.percentage * 100).toFixed(2)}%)`
            : "Bids and asks for this asset"}
        </CardDescription>
      </CardHeader>

      <CardContent className="px-0">
        {isLoading && <SkeletonTable rows={6} cols={3} />}

        {!isLoading && error && (
          <div className="flex flex-col items-center gap-3 px-4 py-10 text-center">
            <TriangleAlert className="size-5 text-destructive" aria-hidden="true" />
            <p className="text-sm text-muted-foreground">{error}</p>
            {onRetry && (
              <Button variant="outline" size="sm" onClick={onRetry}>
                <RefreshCw className="size-3.5" aria-hidden="true" />
                Try again
              </Button>
            )}
          </div>
        )}

        {isEmpty && (
          <p className="px-4 py-10 text-center text-sm text-muted-foreground">
            No open orders for this asset yet. Place a limit order to start the
            book.
          </p>
        )}

        {!isLoading && !error && !isEmpty && (
          <div>
            <div className="grid grid-cols-3 gap-2 px-2 pb-1 text-right text-[0.7rem] text-muted-foreground">
              <span className="text-left">Price</span>
              <span>Amount ({assetCode})</span>
              <span>Total</span>
            </div>

            <div className="flex flex-col">
              {asks.map((level) => (
                <LevelRow
                  key={`ask-${level.price}`}
                  {...level}
                  side="sell"
                  onSelect={
                    onSelectPrice
                      ? () => onSelectPrice(level.price, "buy")
                      : undefined
                  }
                />
              ))}
            </div>

            <div className="my-1 flex items-center justify-between border-y bg-muted/40 px-2 py-1.5">
              <span className="text-xs text-muted-foreground">Last price</span>
              <span className="font-mono text-sm font-medium">
                {formatAmount(book?.lastPrice ?? 0, 4)}
              </span>
            </div>

            <div className="flex flex-col">
              {bids.map((level) => (
                <LevelRow
                  key={`bid-${level.price}`}
                  {...level}
                  side="buy"
                  onSelect={
                    onSelectPrice
                      ? () => onSelectPrice(level.price, "sell")
                      : undefined
                  }
                />
              ))}
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
