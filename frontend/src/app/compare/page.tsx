"use client";

import React, { useState, useCallback } from "react";
import { ComparisonTable, AssetData, AssetMetric } from "@/components/ComparisonTable";
import { AssetLineChart } from "@/components/charts/LineChart";
import { AssetBarChart } from "@/components/charts/BarChart";

// ── Metric definitions ────────────────────────────────────────────────────────

const ALL_METRICS: AssetMetric[] = [
  { key: "price", label: "Current Price", format: "currency", higherIsBetter: true },
  { key: "marketCap", label: "Market Cap", format: "currency", higherIsBetter: true },
  { key: "change24h", label: "24h Change", format: "percent", higherIsBetter: true },
  { key: "change7d", label: "7d Change", format: "percent", higherIsBetter: true },
  { key: "change30d", label: "30d Change", format: "percent", higherIsBetter: true },
  { key: "volume", label: "Volume (24h)", format: "currency", higherIsBetter: true },
  { key: "pe", label: "P/E Ratio", format: "number", higherIsBetter: false },
  { key: "dividendYield", label: "Dividend Yield", format: "percent", higherIsBetter: true },
  { key: "beta", label: "Beta", format: "number", higherIsBetter: false },
  { key: "rsi", label: "RSI (14)", format: "number", higherIsBetter: false },
];

// ── Mock asset pool (replace with real API data) ──────────────────────────────

const ASSET_POOL: AssetData[] = [
  {
    id: "btc",
    name: "Bitcoin",
    symbol: "BTC",
    metrics: {
      price: 68420, marketCap: 1340000000000, change24h: 2.3, change7d: -1.1,
      change30d: 14.2, volume: 28000000000, pe: null as unknown as number,
      dividendYield: 0, beta: 1.4, rsi: 58,
    },
  },
  {
    id: "eth",
    name: "Ethereum",
    symbol: "ETH",
    metrics: {
      price: 3812, marketCap: 458000000000, change24h: 1.8, change7d: 3.2,
      change30d: 18.7, volume: 14000000000, pe: null as unknown as number,
      dividendYield: 0, beta: 1.6, rsi: 62,
    },
  },
  {
    id: "aapl",
    name: "Apple Inc.",
    symbol: "AAPL",
    metrics: {
      price: 192, marketCap: 2960000000000, change24h: 0.4, change7d: 1.2,
      change30d: 5.1, volume: 4200000000, pe: 31.2,
      dividendYield: 0.5, beta: 1.2, rsi: 55,
    },
  },
  {
    id: "gold",
    name: "Gold (XAU)",
    symbol: "XAU",
    metrics: {
      price: 2328, marketCap: null as unknown as number, change24h: -0.2, change7d: 1.4,
      change30d: 7.8, volume: 120000000, pe: null as unknown as number,
      dividendYield: 0, beta: 0.1, rsi: 51,
    },
  },
  {
    id: "sol",
    name: "Solana",
    symbol: "SOL",
    metrics: {
      price: 174, marketCap: 80000000000, change24h: 4.1, change7d: 8.3,
      change30d: 28.4, volume: 3000000000, pe: null as unknown as number,
      dividendYield: 0, beta: 2.1, rsi: 71,
    },
  },
];

// ── Mock performance data ─────────────────────────────────────────────────────

const generatePerf = (base: number, points = 30) =>
  Array.from({ length: points }, (_, i) => ({
    day: `Day ${i + 1}`,
    value: +(base * (1 + (Math.random() - 0.48) * 0.06) ** (i + 1)).toFixed(2),
  }));

const PERF_DATA: Record<string, { day: string; value: number }[]> = {
  btc: generatePerf(60000),
  eth: generatePerf(3400),
  aapl: generatePerf(185),
  gold: generatePerf(2200),
  sol: generatePerf(130),
};

// ── Page ──────────────────────────────────────────────────────────────────────

const METRIC_GROUPS = {
  Performance: ["price", "change24h", "change7d", "change30d"],
  Fundamentals: ["marketCap", "volume", "pe", "dividendYield"],
  Risk: ["beta", "rsi"],
};

export default function ComparePage() {
  const [selected, setSelected] = useState<string[]>(["btc", "eth"]);
  const [selectedMetrics, setSelectedMetrics] = useState<string[]>(
    METRIC_GROUPS.Performance
  );
  const [shareMsg, setShareMsg] = useState<string | null>(null);

  const toggleAsset = (id: string) => {
    setSelected((prev) =>
      prev.includes(id)
        ? prev.filter((a) => a !== id)
        : prev.length < 4
        ? [...prev, id]
        : prev
    );
  };

  const handleShare = useCallback(() => {
    const url = new URL(window.location.href);
    url.searchParams.set("assets", selected.join(","));
    url.searchParams.set("metrics", selectedMetrics.join(","));
    navigator.clipboard
      .writeText(url.toString())
      .then(() => {
        setShareMsg("Link copied!");
        setTimeout(() => setShareMsg(null), 2500);
      })
      .catch(() => setShareMsg("Could not copy link."));
  }, [selected, selectedMetrics]);

  const selectedAssets = ASSET_POOL.filter((a) => selected.includes(a.id));

  // Build relative-performance chart data
  const perfChartData = Array.from({ length: 30 }, (_, i) => {
    const row: Record<string, string | number> = { day: `Day ${i + 1}` };
    selectedAssets.forEach((a) => {
      const pts = PERF_DATA[a.id];
      const base = pts[0].value;
      row[a.symbol] = +(((pts[i]?.value ?? base) / base - 1) * 100).toFixed(2);
    });
    return row;
  });

  const barData = selectedAssets.map((a) => ({
    name: a.symbol,
    "30d Return": Number(a.metrics.change30d),
  }));

  return (
    <main className="min-h-screen bg-gray-50 dark:bg-gray-950 px-4 py-8 max-w-6xl mx-auto">
      {/* Page header */}
      <div className="flex flex-wrap items-center justify-between gap-4 mb-8">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 dark:text-white">
            Asset Comparison
          </h1>
          <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
            Compare up to 4 assets side by side
          </p>
        </div>
        <button
          onClick={handleShare}
          className="inline-flex items-center gap-2 text-sm font-medium text-indigo-600 dark:text-indigo-400 hover:text-indigo-800 transition-colors"
        >
          <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8.684 13.342C8.886 12.938 9 12.482 9 12c0-.482-.114-.938-.316-1.342m0 2.684a3 3 0 110-2.684m0 2.684l6.632 3.316m-6.632-6l6.632-3.316m0 0a3 3 0 105.367-2.684 3 3 0 00-5.367 2.684zm0 9.316a3 3 0 105.368 2.684 3 3 0 00-5.368-2.684z" />
          </svg>
          Share
        </button>
        {shareMsg && (
          <span className="text-xs text-emerald-600 font-medium">{shareMsg}</span>
        )}
      </div>

      {/* Asset selector */}
      <section className="mb-6">
        <h2 className="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400 mb-3">
          Select assets ({selected.length}/4)
        </h2>
        <div className="flex flex-wrap gap-2">
          {ASSET_POOL.map((asset) => {
            const isOn = selected.includes(asset.id);
            return (
              <button
                key={asset.id}
                onClick={() => toggleAsset(asset.id)}
                disabled={!isOn && selected.length >= 4}
                className={[
                  "px-3 py-1.5 rounded-lg text-sm font-medium border transition-all",
                  isOn
                    ? "bg-indigo-600 text-white border-indigo-600"
                    : "bg-white dark:bg-gray-800 text-gray-700 dark:text-gray-300 border-gray-200 dark:border-gray-700 hover:border-indigo-400",
                  !isOn && selected.length >= 4 ? "opacity-40 cursor-not-allowed" : "",
                ].join(" ")}
              >
                {asset.symbol}
                <span className="ml-1 text-xs opacity-70">{asset.name}</span>
              </button>
            );
          })}
        </div>
      </section>

      {/* Metric criteria selector */}
      <section className="mb-6">
        <h2 className="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400 mb-3">
          Comparison criteria
        </h2>
        <div className="flex flex-wrap gap-2">
          {Object.entries(METRIC_GROUPS).map(([group, keys]) => (
            <button
              key={group}
              onClick={() => setSelectedMetrics(keys)}
              className={[
                "px-3 py-1.5 rounded-lg text-sm border transition-all",
                JSON.stringify(selectedMetrics) === JSON.stringify(keys)
                  ? "bg-indigo-50 border-indigo-400 text-indigo-700 dark:bg-indigo-900/40 dark:text-indigo-300"
                  : "bg-white dark:bg-gray-800 text-gray-600 dark:text-gray-300 border-gray-200 dark:border-gray-700",
              ].join(" ")}
            >
              {group}
            </button>
          ))}
        </div>
      </section>

      {/* Charts */}
      {selectedAssets.length >= 2 && (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-6 mb-8">
          <div className="bg-white dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-700 p-4">
            <AssetLineChart
              title="Relative Performance (30d)"
              data={perfChartData}
              xDataKey="day"
              yLabel="Return %"
              lines={selectedAssets.map((a, idx) => ({
                dataKey: a.symbol,
                label: a.symbol,
              }))}
              height={220}
            />
          </div>
          <div className="bg-white dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-700 p-4">
            <AssetBarChart
              title="30-Day Return (%)"
              data={barData}
              bars={[{ dataKey: "30d Return", label: "30d Return" }]}
              xDataKey="name"
              height={220}
              highlightAbove={0}
              highlightColor="#10b981"
            />
          </div>
        </div>
      )}

      {/* Comparison table */}
      {selectedAssets.length >= 2 ? (
        <div className="bg-white dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-700 p-4">
          <h2 className="text-sm font-semibold text-gray-700 dark:text-gray-200 mb-4">
            Side-by-Side Metrics
          </h2>
          <ComparisonTable
            assets={selectedAssets}
            metricDefinitions={ALL_METRICS}
            selectedMetrics={selectedMetrics}
          />
          <p className="text-xs text-gray-400 mt-3">
            <span className="font-semibold text-indigo-600">▲ Bold</span> = best value
            in row. Data is illustrative only.
          </p>
        </div>
      ) : (
        <div className="text-center py-16 text-gray-400 dark:text-gray-600">
          <p className="text-lg">Select at least 2 assets above to compare.</p>
        </div>
      )}
    </main>
  );
}
