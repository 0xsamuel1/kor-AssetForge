/**
 * Trading domain: order book access, quote maths and order submission.
 *
 * All pricing logic lives here (rather than in the components) so the order
 * form, the confirmation modal and any future automation agree on the numbers.
 */

import {
  WebSocketClient,
  getWebSocketUrl,
  type WSStatus,
} from "@/lib/websocket";

export type OrderSide = "buy" | "sell";
export type OrderType = "market" | "limit";

export interface OrderBookEntry {
  price: number;
  amount: number;
}

export interface OrderBookSnapshot {
  /** Buy-side interest, best (highest) price first. */
  bids: OrderBookEntry[];
  /** Sell-side interest, best (lowest) price first. */
  asks: OrderBookEntry[];
  lastPrice: number;
  updatedAt: number;
  /** True when the data is locally generated because the API isn't available. */
  isSample?: boolean;
}

export interface RecentTrade {
  id: string;
  side: OrderSide;
  price: number;
  amount: number;
  timestamp: number;
}

export interface QuoteInput {
  side: OrderSide;
  type: OrderType;
  /** Units of the asset being bought or sold. */
  amount: number;
  /** Required for limit orders; ignored for market orders. */
  limitPrice?: number;
  /** Fraction, e.g. 0.005 for 0.5%. */
  slippageTolerance: number;
  book: OrderBookSnapshot | null;
}

export interface TradeQuote {
  side: OrderSide;
  type: OrderType;
  amount: number;
  /** Volume-weighted fill price for market orders, limit price otherwise. */
  averagePrice: number;
  /** amount × averagePrice, before fees. */
  subtotal: number;
  platformFee: number;
  /** Stellar base fee, denominated in XLM and quoted separately from `total`. */
  networkFee: number;
  /** What the wallet pays (buy) or receives (sell), platform fee included. */
  total: number;
  /** Difference between the best price and the average fill, as a fraction. */
  priceImpact: number;
  /** Worst price still accepted once slippage tolerance is applied. */
  worstPrice: number;
  /** Buy: maximum spend. Sell: minimum proceeds. Both after fees. */
  slippageLimit: number;
  /** How much of `amount` the visible book can fill. */
  fillableAmount: number;
  hasEnoughLiquidity: boolean;
  estimatedSeconds: number;
}

export interface OrderRequest {
  assetId: string;
  side: OrderSide;
  type: OrderType;
  amount: number;
  price: number;
  slippageTolerance: number;
  account: string;
}

export interface OrderReceipt {
  id: string;
  status: "pending" | "filled" | "partially_filled" | "rejected";
  txHash?: string;
  filledAmount?: number;
  submittedAt: number;
}

/** Platform trading fee (0.30%). */
export const PLATFORM_FEE_RATE = 0.003;

/** Stellar base fee per operation, in XLM. */
export const NETWORK_FEE = 0.00001;

/** Selectable slippage tolerances, as fractions. */
export const SLIPPAGE_PRESETS = [0.001, 0.005, 0.01, 0.03] as const;

/** Above this price impact the confirmation modal warns before submitting. */
export const HIGH_IMPACT_THRESHOLD = 0.02;

/** Average Stellar ledger close time, used for completion estimates. */
const LEDGER_SECONDS = 5;

const API_URL =
  process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

// ── Quote maths ──────────────────────────────────────────────────────────────

/** Best available price for a side: lowest ask when buying, highest bid when selling. */
export function getBestPrice(
  book: OrderBookSnapshot | null,
  side: OrderSide,
): number {
  if (!book) return 0;
  const levels = side === "buy" ? book.asks : book.bids;
  return levels[0]?.price ?? book.lastPrice;
}

/** Spread between best bid and best ask, absolute and as a fraction of the mid. */
export function getSpread(book: OrderBookSnapshot | null): {
  absolute: number;
  percentage: number;
} {
  const bestBid = book?.bids[0]?.price ?? 0;
  const bestAsk = book?.asks[0]?.price ?? 0;
  if (!bestBid || !bestAsk) return { absolute: 0, percentage: 0 };

  const absolute = bestAsk - bestBid;
  const mid = (bestAsk + bestBid) / 2;
  return { absolute, percentage: mid > 0 ? absolute / mid : 0 };
}

/**
 * Walks the book to work out what a market order would actually fill at.
 * Returns the volume-weighted average price and how much of the request the
 * visible depth can cover.
 */
function walkBook(
  levels: OrderBookEntry[],
  amount: number,
): { averagePrice: number; fillableAmount: number } {
  let remaining = amount;
  let cost = 0;
  let filled = 0;

  for (const level of levels) {
    if (remaining <= 0) break;
    const take = Math.min(remaining, level.amount);
    cost += take * level.price;
    filled += take;
    remaining -= take;
  }

  return {
    averagePrice: filled > 0 ? cost / filled : 0,
    fillableAmount: filled,
  };
}

/** Builds the full cost breakdown shown in the form and the confirmation modal. */
export function calculateQuote({
  side,
  type,
  amount,
  limitPrice,
  slippageTolerance,
  book,
}: QuoteInput): TradeQuote {
  const levels = side === "buy" ? (book?.asks ?? []) : (book?.bids ?? []);
  const bestPrice = getBestPrice(book, side);

  const marketFill = walkBook(levels, amount);
  const isLimit = type === "limit";

  const averagePrice = isLimit
    ? (limitPrice ?? 0)
    : marketFill.averagePrice || bestPrice;

  const fillableAmount = isLimit ? amount : marketFill.fillableAmount;
  const subtotal = amount * averagePrice;
  const platformFee = subtotal * PLATFORM_FEE_RATE;

  // Buyers pay the fee on top; sellers have it deducted from the proceeds.
  // The network fee is XLM-denominated, so it is reported separately rather
  // than mixed into the quote-currency total.
  const total =
    side === "buy" ? subtotal + platformFee : subtotal - platformFee;

  const priceImpact =
    bestPrice > 0 && averagePrice > 0
      ? Math.abs(averagePrice - bestPrice) / bestPrice
      : 0;

  const worstPrice =
    side === "buy"
      ? averagePrice * (1 + slippageTolerance)
      : averagePrice * (1 - slippageTolerance);

  const slippageSubtotal = amount * worstPrice;
  const slippageLimit =
    side === "buy"
      ? slippageSubtotal + slippageSubtotal * PLATFORM_FEE_RATE
      : slippageSubtotal - slippageSubtotal * PLATFORM_FEE_RATE;

  // Market orders clear in a ledger or two; limit orders wait for a match.
  const estimatedSeconds = isLimit ? LEDGER_SECONDS * 12 : LEDGER_SECONDS * 2;

  return {
    side,
    type,
    amount,
    averagePrice,
    subtotal,
    platformFee,
    networkFee: NETWORK_FEE,
    total,
    priceImpact,
    worstPrice,
    slippageLimit,
    fillableAmount,
    hasEnoughLiquidity: isLimit || fillableAmount >= amount,
    estimatedSeconds,
  };
}

/** Cumulative depth per level, used for the order book's background bars. */
export function withCumulativeTotals(
  levels: OrderBookEntry[],
): { price: number; amount: number; total: number; depth: number }[] {
  let running = 0;
  const rows = levels.map((level) => {
    running += level.amount;
    return { ...level, total: running };
  });

  const max = running || 1;
  return rows.map((row) => ({ ...row, depth: row.total / max }));
}

// ── Sample data ──────────────────────────────────────────────────────────────

/**
 * Deterministic order book used when the marketplace API isn't reachable, so
 * the trading UI stays explorable in local development. Flagged with
 * `isSample` so the page can label it.
 */
export function generateSampleOrderBook(
  midPrice: number,
  levels = 8,
): OrderBookSnapshot {
  const price = midPrice > 0 ? midPrice : 100;
  const tick = price * 0.002;

  const asks: OrderBookEntry[] = Array.from({ length: levels }, (_, i) => ({
    price: Number((price + tick * (i + 1)).toFixed(4)),
    amount: Number((25 + ((i * 37) % 60)).toFixed(2)),
  }));

  const bids: OrderBookEntry[] = Array.from({ length: levels }, (_, i) => ({
    price: Number((price - tick * (i + 1)).toFixed(4)),
    amount: Number((30 + ((i * 29) % 55)).toFixed(2)),
  }));

  return { bids, asks, lastPrice: price, updatedAt: Date.now(), isSample: true };
}

// ── API ──────────────────────────────────────────────────────────────────────

class TradingApi {
  /**
   * Loads the order book. Falls back to sample depth (clearly flagged) when the
   * endpoint isn't available yet.
   */
  async getOrderBook(
    assetId: string,
    fallbackPrice: number,
  ): Promise<OrderBookSnapshot> {
    try {
      const res = await fetch(
        `${API_URL}/api/v1/marketplace/orderbook/${assetId}`,
      );
      if (!res.ok) return generateSampleOrderBook(fallbackPrice);

      const data = (await res.json()) as OrderBookSnapshot;
      return {
        bids: data.bids ?? [],
        asks: data.asks ?? [],
        lastPrice: data.lastPrice ?? fallbackPrice,
        updatedAt: data.updatedAt ?? Date.now(),
      };
    } catch {
      return generateSampleOrderBook(fallbackPrice);
    }
  }

  async getRecentTrades(assetId: string): Promise<RecentTrade[]> {
    try {
      const res = await fetch(
        `${API_URL}/api/v1/marketplace/trades?asset_id=${assetId}&limit=20`,
      );
      if (!res.ok) return [];
      return (await res.json()) as RecentTrade[];
    } catch {
      return [];
    }
  }

  /** Submits an order. Throws with a readable message so the UI can toast it. */
  async submitOrder(request: OrderRequest): Promise<OrderReceipt> {
    const res = await fetch(`${API_URL}/api/v1/marketplace/orders`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        asset_id: request.assetId,
        side: request.side,
        order_type: request.type,
        amount: request.amount,
        price: request.price,
        slippage_tolerance: request.slippageTolerance,
        account: request.account,
      }),
    });

    if (!res.ok) {
      const detail = await res.text().catch(() => "");
      throw new Error(
        detail || `Order rejected by the marketplace (${res.status}).`,
      );
    }

    return (await res.json()) as OrderReceipt;
  }
}

export const tradingApi = new TradingApi();

// ── Live updates ─────────────────────────────────────────────────────────────

/**
 * Streams order book updates for one asset. Returns an unsubscribe function.
 * The caller keeps polling as a fallback — the socket is an optimisation, not
 * a requirement.
 */
export function subscribeToOrderBook(
  assetId: string,
  onUpdate: (book: OrderBookSnapshot) => void,
  onStatusChange?: (status: WSStatus) => void,
): () => void {
  const client = new WebSocketClient({
    url: getWebSocketUrl(),
    onStatusChange,
  });

  const unsubscribe = client.subscribe<OrderBookSnapshot & { assetId?: string }>(
    "orderbook_update",
    ({ payload }) => {
      if (payload.assetId && payload.assetId !== assetId) return;
      onUpdate({
        bids: payload.bids ?? [],
        asks: payload.asks ?? [],
        lastPrice: payload.lastPrice,
        updatedAt: payload.updatedAt ?? Date.now(),
      });
    },
  );

  client.connect();
  client.send("subscribe", { channel: "orderbook", assetId });

  return () => {
    unsubscribe();
    client.disconnect();
  };
}
