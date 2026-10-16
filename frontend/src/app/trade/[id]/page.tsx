"use client";

import * as React from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { ArrowLeft, RefreshCw, TriangleAlert } from "lucide-react";

import { Header } from "@/components/Header";
import { OrderBook } from "@/components/OrderBook";
import { OrderForm, type OrderFormHandle } from "@/components/OrderForm";
import { WalletConnect } from "@/components/WalletConnect";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Skeleton, SkeletonList } from "@/components/ui/skeleton";
import { assetApi, type AssetDetail } from "@/lib/asset-api";
import type { StellarWallet } from "@/lib/stellar";
import {
  subscribeToOrderBook,
  tradingApi,
  type OrderBookSnapshot,
  type OrderSide,
  type RecentTrade,
} from "@/lib/trading";
import type { WSStatus } from "@/lib/websocket";
import { cn, formatAmount, formatCurrency } from "@/lib/utils";

/** How often the book is re-fetched when the WebSocket isn't delivering. */
const POLL_INTERVAL = 15_000;

/** Minimal stand-in so the ticket stays usable when the assets API is down. */
function sampleAsset(assetId: string): AssetDetail {
  return {
    id: assetId,
    code: assetId.slice(0, 4).toUpperCase() || "ASSET",
    issuer: "",
    name: "Sample asset",
    description: "",
    totalSupply: "0",
    decimals: 7,
    price: 100,
    priceChange24h: 0,
    marketCap: 0,
    volume24h: 0,
    allTimeHigh: 0,
    allTimeLow: 0,
    createdAt: Date.now(),
    verified: false,
    documents: [],
    metadata: { category: "", location: "", condition: "", tags: [] },
  };
}

function LiveIndicator({ status }: { status: WSStatus }) {
  const label =
    status === "connected"
      ? "Live"
      : status === "connecting"
        ? "Connecting…"
        : "Polling";

  return (
    <span
      role="status"
      className="inline-flex items-center gap-1.5 text-xs text-muted-foreground"
    >
      <span
        aria-hidden="true"
        className={cn(
          "size-1.5 rounded-full",
          status === "connected"
            ? "bg-emerald-500"
            : status === "connecting"
              ? "animate-pulse bg-amber-400"
              : "bg-muted-foreground/50",
        )}
      />
      {label}
    </span>
  );
}

export default function TradePage() {
  const params = useParams();
  const assetId = String(params.id ?? "");

  const [wallet, setWallet] = React.useState<StellarWallet | undefined>();
  const [asset, setAsset] = React.useState<AssetDetail | null>(null);
  const [book, setBook] = React.useState<OrderBookSnapshot | null>(null);
  const [trades, setTrades] = React.useState<RecentTrade[]>([]);
  const [isLoading, setIsLoading] = React.useState(true);
  const [isRefreshing, setIsRefreshing] = React.useState(false);
  const [assetError, setAssetError] = React.useState<string | null>(null);
  const [wsStatus, setWsStatus] = React.useState<WSStatus>("disconnected");

  const formRef = React.useRef<OrderFormHandle>(null);

  const load = React.useCallback(async () => {
    let detail: AssetDetail;
    let error: string | null = null;

    try {
      detail = await assetApi.getAssetDetail(assetId);
    } catch {
      // The marketplace API isn't reachable — keep the ticket usable with
      // clearly-labelled sample pricing instead of a dead page.
      detail = sampleAsset(assetId);
      error =
        "Live asset data is unavailable right now. Prices below are sample data.";
    }

    const [nextBook, nextTrades] = await Promise.all([
      tradingApi.getOrderBook(assetId, detail.price),
      tradingApi.getRecentTrades(assetId),
    ]);

    setAsset(detail);
    setAssetError(error);
    setBook(nextBook);
    setTrades(nextTrades);
    setIsLoading(false);
  }, [assetId]);

  // Initial load plus a polling fallback, so the book stays fresh even without
  // a WebSocket server.
  React.useEffect(() => {
    if (!assetId) return;

    // The first load is queued as a task so the effect body itself performs no
    // state updates (see react-hooks/set-state-in-effect).
    const initial = window.setTimeout(load, 0);
    const timer = window.setInterval(load, POLL_INTERVAL);

    return () => {
      window.clearTimeout(initial);
      window.clearInterval(timer);
    };
  }, [assetId, load]);

  // Live book updates when the platform WebSocket is available.
  React.useEffect(() => {
    if (!assetId) return;
    return subscribeToOrderBook(assetId, setBook, setWsStatus);
  }, [assetId]);

  const handleRefresh = async () => {
    setIsRefreshing(true);
    await load();
    setIsRefreshing(false);
  };

  const handleSelectPrice = (price: number, side: OrderSide) => {
    formRef.current?.applyPrice(price, side);
  };

  const priceChange = asset?.priceChange24h ?? 0;

  return (
    <div className="min-h-screen bg-background">
      <Header
        wallet={wallet}
        onWalletConnected={setWallet}
        onWalletDisconnected={() => setWallet(undefined)}
      />

      <main id="main-content" className="container mx-auto max-w-6xl px-4 py-8">
        <Link
          href={`/assets/${assetId}`}
          className="mb-4 inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground"
        >
          <ArrowLeft className="size-4" aria-hidden="true" />
          Back to asset
        </Link>

        {/* ── Asset summary ─────────────────────────────────────────────── */}
        <div className="mb-6 flex flex-wrap items-start justify-between gap-4">
          <div className="min-w-0">
            {isLoading ? (
              <div className="space-y-2">
                <Skeleton className="h-8 w-56" />
                <Skeleton className="h-4 w-32" />
              </div>
            ) : (
              <>
                <div className="flex flex-wrap items-center gap-2">
                  <h1 className="text-3xl font-bold">{asset?.name}</h1>
                  <Badge variant="secondary">{asset?.code}</Badge>
                  {asset?.verified && <Badge>Verified</Badge>}
                </div>
                <p className="mt-1 flex flex-wrap items-center gap-3 text-sm text-muted-foreground">
                  <span className="font-mono text-base text-foreground">
                    {formatCurrency(asset?.price ?? 0)}
                  </span>
                  <span
                    className={cn(
                      "font-medium",
                      priceChange >= 0
                        ? "text-emerald-600 dark:text-emerald-400"
                        : "text-destructive",
                    )}
                  >
                    {priceChange >= 0 ? "+" : ""}
                    {priceChange.toFixed(2)}% 24h
                  </span>
                  <LiveIndicator status={wsStatus} />
                </p>
              </>
            )}
          </div>

          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={handleRefresh}
              disabled={isRefreshing}
              aria-busy={isRefreshing}
            >
              <RefreshCw
                className={cn("size-3.5", isRefreshing && "animate-spin")}
                aria-hidden="true"
              />
              Refresh
            </Button>
            <WalletConnect
              variant="button"
              wallet={wallet}
              onWalletConnected={setWallet}
              onWalletDisconnected={() => setWallet(undefined)}
            />
          </div>
        </div>

        {(assetError || book?.isSample) && !isLoading && (
          <div
            role="status"
            className="mb-6 flex items-start gap-2 rounded-lg bg-amber-500/10 p-3 text-sm text-amber-700 dark:text-amber-400"
          >
            <TriangleAlert className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
            <p>
              {assetError ??
                "The marketplace order book API is unavailable — showing sample depth so the interface stays explorable."}
            </p>
          </div>
        )}

        {/* ── Book + ticket ─────────────────────────────────────────────── */}
        <div className="grid gap-6 lg:grid-cols-[1fr_360px]">
          <div className="space-y-6">
            <OrderBook
              book={book}
              isLoading={isLoading}
              onRetry={handleRefresh}
              onSelectPrice={handleSelectPrice}
              assetCode={asset?.code ?? ""}
            />

            <Card>
              <CardHeader className="border-b">
                <CardTitle>Recent trades</CardTitle>
                <CardDescription>
                  The latest fills for this asset
                </CardDescription>
              </CardHeader>
              <CardContent>
                {isLoading ? (
                  <SkeletonList rows={3} />
                ) : trades.length === 0 ? (
                  <p className="py-6 text-center text-sm text-muted-foreground">
                    No trades recorded yet.
                  </p>
                ) : (
                  <ul className="divide-y">
                    {trades.map((trade) => (
                      <li
                        key={trade.id}
                        className="flex items-center justify-between gap-4 py-2 text-sm"
                      >
                        <span
                          className={cn(
                            "font-medium",
                            trade.side === "buy"
                              ? "text-emerald-600 dark:text-emerald-400"
                              : "text-destructive",
                          )}
                        >
                          {trade.side === "buy" ? "Buy" : "Sell"}
                        </span>
                        <span className="font-mono text-xs">
                          {formatAmount(trade.amount, 2)} @{" "}
                          {formatCurrency(trade.price)}
                        </span>
                        <span className="text-xs text-muted-foreground">
                          {new Date(trade.timestamp).toLocaleTimeString()}
                        </span>
                      </li>
                    ))}
                  </ul>
                )}
              </CardContent>
            </Card>
          </div>

          <div className="lg:sticky lg:top-6 lg:self-start">
            <OrderForm
              ref={formRef}
              assetId={assetId}
              assetCode={asset?.code ?? ""}
              assetName={asset?.name}
              book={book}
              wallet={wallet}
              onSubmitted={handleRefresh}
            />
          </div>
        </div>
      </main>
    </div>
  );
}
