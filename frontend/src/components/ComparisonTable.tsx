"use client";

import React from "react";

export interface AssetMetric {
  key: string;
  label: string;
  format?: "currency" | "percent" | "number" | "text";
  /** Higher is better (used to highlight best value) */
  higherIsBetter?: boolean;
}

export interface AssetData {
  id: string;
  name: string;
  symbol: string;
  /** Map of metric key → raw value */
  metrics: Record<string, number | string>;
}

interface ComparisonTableProps {
  assets: AssetData[];
  metricDefinitions: AssetMetric[];
  /** Criteria to show (default: all) */
  selectedMetrics?: string[];
  className?: string;
}

const formatValue = (
  value: number | string,
  format: AssetMetric["format"] = "text"
): string => {
  if (value === null || value === undefined || value === "") return "—";
  if (format === "currency" && typeof value === "number")
    return value.toLocaleString("en-US", { style: "currency", currency: "USD" });
  if (format === "percent" && typeof value === "number")
    return `${value >= 0 ? "+" : ""}${value.toFixed(2)}%`;
  if (format === "number" && typeof value === "number")
    return value.toLocaleString("en-US");
  return String(value);
};

const ASSET_COLORS = [
  "bg-indigo-50 border-indigo-300 text-indigo-800",
  "bg-emerald-50 border-emerald-300 text-emerald-800",
  "bg-amber-50 border-amber-300 text-amber-800",
  "bg-rose-50 border-rose-300 text-rose-800",
];

const BEST_HIGHLIGHT = "font-bold text-indigo-700 dark:text-indigo-300";
const WORST_HIGHLIGHT = "text-red-500 dark:text-red-400";

function getBestWorstIndices(
  assets: AssetData[],
  metricKey: string,
  higherIsBetter = true
): { best: number; worst: number } | null {
  const values = assets
    .map((a, idx) => ({ idx, val: Number(a.metrics[metricKey]) }))
    .filter((v) => !isNaN(v.val));

  if (values.length < 2) return null;

  const sorted = [...values].sort((a, b) =>
    higherIsBetter ? b.val - a.val : a.val - b.val
  );

  return { best: sorted[0].idx, worst: sorted[sorted.length - 1].idx };
}

export const ComparisonTable: React.FC<ComparisonTableProps> = ({
  assets,
  metricDefinitions,
  selectedMetrics,
  className = "",
}) => {
  if (assets.length < 2 || assets.length > 4) {
    return (
      <p className="text-sm text-gray-500">Select 2–4 assets to compare.</p>
    );
  }

  const metrics = selectedMetrics
    ? metricDefinitions.filter((m) => selectedMetrics.includes(m.key))
    : metricDefinitions;

  return (
    <div className={`overflow-x-auto rounded-xl border border-gray-200 dark:border-gray-700 ${className}`}>
      <table className="w-full text-sm border-collapse">
        <thead>
          <tr className="bg-gray-50 dark:bg-gray-800">
            <th className="text-left px-4 py-3 font-semibold text-gray-500 dark:text-gray-400 w-40 border-b border-gray-200 dark:border-gray-700">
              Metric
            </th>
            {assets.map((asset, idx) => (
              <th
                key={asset.id}
                className={`text-center px-4 py-3 border-b border-gray-200 dark:border-gray-700`}
              >
                <div
                  className={`inline-block px-2 py-0.5 rounded-full text-xs font-semibold border ${ASSET_COLORS[idx % ASSET_COLORS.length]}`}
                >
                  {asset.symbol}
                </div>
                <div className="text-xs font-medium text-gray-700 dark:text-gray-200 mt-1 truncate max-w-[120px] mx-auto">
                  {asset.name}
                </div>
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {metrics.map((metric, mIdx) => {
            const bw = getBestWorstIndices(assets, metric.key, metric.higherIsBetter ?? true);

            return (
              <tr
                key={metric.key}
                className={
                  mIdx % 2 === 0
                    ? "bg-white dark:bg-gray-900"
                    : "bg-gray-50/60 dark:bg-gray-800/50"
                }
              >
                <td className="px-4 py-3 text-gray-600 dark:text-gray-400 font-medium text-xs whitespace-nowrap">
                  {metric.label}
                </td>
                {assets.map((asset, aIdx) => {
                  const raw = asset.metrics[metric.key];
                  const formatted = formatValue(raw, metric.format);
                  const isBest = bw?.best === aIdx;
                  const isWorst = bw?.worst === aIdx;

                  return (
                    <td
                      key={asset.id}
                      className={`text-center px-4 py-3 tabular-nums ${isBest ? BEST_HIGHLIGHT : isWorst ? WORST_HIGHLIGHT : "text-gray-800 dark:text-gray-200"}`}
                    >
                      {formatted}
                      {isBest && (
                        <span
                          className="ml-1 text-[10px] text-indigo-400"
                          aria-label="best"
                        >
                          ▲
                        </span>
                      )}
                    </td>
                  );
                })}
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
};

export default ComparisonTable;
