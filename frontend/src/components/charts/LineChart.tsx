"use client";

import React, { useCallback } from "react";
import {
  LineChart as RechartsLineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ResponsiveContainer,
  Brush,
} from "recharts";
import { largestTriangleThreeBuckets } from "./utils";

export interface LineChartDataPoint {
  [key: string]: string | number;
}

export interface LineConfig {
  dataKey: string;
  color?: string;
  label?: string;
  strokeWidth?: number;
  dot?: boolean;
}

interface AssetLineChartProps {
  data: LineChartDataPoint[];
  lines: LineConfig[];
  xDataKey: string;
  xLabel?: string;
  yLabel?: string;
  title?: string;
  height?: number;
  /** Decimate large datasets for performance. Provide target bucket count. */
  decimateThreshold?: number;
  /** Called with base64 PNG when export is triggered */
  onExportImage?: (dataUrl: string) => void;
  className?: string;
}

const DEFAULT_COLORS = [
  "#6366f1",
  "#10b981",
  "#f59e0b",
  "#ef4444",
  "#8b5cf6",
  "#06b6d4",
];

export const AssetLineChart: React.FC<AssetLineChartProps> = ({
  data,
  lines,
  xDataKey,
  xLabel,
  yLabel,
  title,
  height = 320,
  decimateThreshold,
  onExportImage,
  className = "",
}) => {
  const displayData =
    decimateThreshold && data.length > decimateThreshold
      ? largestTriangleThreeBuckets(data, decimateThreshold, lines[0]?.dataKey)
      : data;

  const handleExport = useCallback(() => {
    const svgEl = document.querySelector<SVGSVGElement>(
      `.asset-line-chart-${title?.replace(/\s+/g, "-")} svg`
    );
    if (!svgEl || !onExportImage) return;

    const svgData = new XMLSerializer().serializeToString(svgEl);
    const canvas = document.createElement("canvas");
    const ctx = canvas.getContext("2d");
    const img = new Image();
    img.onload = () => {
      canvas.width = img.width;
      canvas.height = img.height;
      ctx?.drawImage(img, 0, 0);
      onExportImage(canvas.toDataURL("image/png"));
    };
    img.src = `data:image/svg+xml;base64,${btoa(svgData)}`;
  }, [title, onExportImage]);

  return (
    <div
      className={`asset-line-chart-${title?.replace(/\s+/g, "-")} w-full ${className}`}
    >
      {title && (
        <div className="flex items-center justify-between mb-3">
          <h3 className="text-sm font-semibold text-gray-700 dark:text-gray-200">
            {title}
          </h3>
          {onExportImage && (
            <button
              onClick={handleExport}
              className="text-xs text-indigo-500 hover:text-indigo-700 dark:text-indigo-400 transition-colors"
              aria-label="Export chart as image"
            >
              Export ↓
            </button>
          )}
        </div>
      )}
      <ResponsiveContainer width="100%" height={height}>
        <RechartsLineChart
          data={displayData}
          margin={{ top: 4, right: 20, left: 0, bottom: xLabel ? 24 : 4 }}
        >
          <CartesianGrid
            strokeDasharray="3 3"
            stroke="currentColor"
            className="text-gray-200 dark:text-gray-700"
            opacity={0.6}
          />
          <XAxis
            dataKey={xDataKey}
            tick={{ fontSize: 11 }}
            label={
              xLabel
                ? { value: xLabel, position: "insideBottom", offset: -8, fontSize: 11 }
                : undefined
            }
          />
          <YAxis
            tick={{ fontSize: 11 }}
            label={
              yLabel
                ? { value: yLabel, angle: -90, position: "insideLeft", fontSize: 11 }
                : undefined
            }
          />
          <Tooltip
            contentStyle={{
              fontSize: 12,
              borderRadius: 8,
              border: "1px solid #e5e7eb",
            }}
          />
          <Legend wrapperStyle={{ fontSize: 12 }} />
          {lines.map((line, idx) => (
            <Line
              key={line.dataKey}
              type="monotone"
              dataKey={line.dataKey}
              name={line.label ?? line.dataKey}
              stroke={line.color ?? DEFAULT_COLORS[idx % DEFAULT_COLORS.length]}
              strokeWidth={line.strokeWidth ?? 2}
              dot={line.dot ?? false}
              activeDot={{ r: 4 }}
              isAnimationActive={false}
            />
          ))}
          {data.length > 30 && <Brush dataKey={xDataKey} height={20} stroke="#6366f1" />}
        </RechartsLineChart>
      </ResponsiveContainer>
    </div>
  );
};

export default AssetLineChart;
