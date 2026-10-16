"use client";

import React, { useCallback } from "react";
import {
  BarChart as RechartsBarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ResponsiveContainer,
  Cell,
} from "recharts";

export interface BarChartDataPoint {
  [key: string]: string | number;
}

export interface BarConfig {
  dataKey: string;
  color?: string;
  label?: string;
}

interface AssetBarChartProps {
  data: BarChartDataPoint[];
  bars: BarConfig[];
  xDataKey: string;
  xLabel?: string;
  yLabel?: string;
  title?: string;
  height?: number;
  layout?: "vertical" | "horizontal";
  /** Highlight bars above this value */
  highlightAbove?: number;
  highlightColor?: string;
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

export const AssetBarChart: React.FC<AssetBarChartProps> = ({
  data,
  bars,
  xDataKey,
  xLabel,
  yLabel,
  title,
  height = 320,
  layout = "horizontal",
  highlightAbove,
  highlightColor = "#ef4444",
  onExportImage,
  className = "",
}) => {
  const slug = title?.replace(/\s+/g, "-") ?? "bar";

  const handleExport = useCallback(() => {
    const svgEl = document.querySelector<SVGSVGElement>(
      `.asset-bar-chart-${slug} svg`
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
  }, [slug, onExportImage]);

  return (
    <div className={`asset-bar-chart-${slug} w-full ${className}`}>
      {title && (
        <div className="flex items-center justify-between mb-3">
          <h3 className="text-sm font-semibold text-gray-700 dark:text-gray-200">
            {title}
          </h3>
          {onExportImage && (
            <button
              onClick={handleExport}
              className="text-xs text-indigo-500 hover:text-indigo-700 transition-colors"
            >
              Export ↓
            </button>
          )}
        </div>
      )}
      <ResponsiveContainer width="100%" height={height}>
        <RechartsBarChart
          data={data}
          layout={layout}
          margin={{ top: 4, right: 20, left: 0, bottom: xLabel ? 24 : 4 }}
        >
          <CartesianGrid
            strokeDasharray="3 3"
            stroke="currentColor"
            className="text-gray-200 dark:text-gray-700"
            opacity={0.6}
          />
          {layout === "horizontal" ? (
            <>
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
            </>
          ) : (
            <>
              <XAxis type="number" tick={{ fontSize: 11 }} />
              <YAxis dataKey={xDataKey} type="category" tick={{ fontSize: 11 }} width={80} />
            </>
          )}
          <Tooltip
            contentStyle={{ fontSize: 12, borderRadius: 8 }}
          />
          <Legend wrapperStyle={{ fontSize: 12 }} />
          {bars.map((bar, idx) => {
            const baseColor = bar.color ?? DEFAULT_COLORS[idx % DEFAULT_COLORS.length];
            return (
              <Bar
                key={bar.dataKey}
                dataKey={bar.dataKey}
                name={bar.label ?? bar.dataKey}
                fill={baseColor}
                radius={[3, 3, 0, 0]}
                isAnimationActive={false}
              >
                {highlightAbove !== undefined &&
                  data.map((entry, i) => (
                    <Cell
                      key={`cell-${i}`}
                      fill={
                        Number(entry[bar.dataKey]) > highlightAbove
                          ? highlightColor
                          : baseColor
                      }
                    />
                  ))}
              </Bar>
            );
          })}
        </RechartsBarChart>
      </ResponsiveContainer>
    </div>
  );
};

export default AssetBarChart;
