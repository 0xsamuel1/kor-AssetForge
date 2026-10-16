"use client";

import React, { useCallback } from "react";
import {
  PieChart as RechartsPieChart,
  Pie,
  Cell,
  Tooltip,
  Legend,
  ResponsiveContainer,
  type PieLabelRenderProps,
} from "recharts";

export interface PieChartDataPoint {
  name: string;
  value: number;
  color?: string;
}

interface AssetPieChartProps {
  data: PieChartDataPoint[];
  title?: string;
  height?: number;
  innerRadius?: number; // >0 = donut chart
  showLegend?: boolean;
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
  "#f97316",
  "#84cc16",
];

const renderCustomLabel = (props: PieLabelRenderProps) => {
  // Recharts types these as optional / number | string; they are always
  // resolved to numbers by the time a label is rendered.
  const cx = Number(props.cx ?? 0);
  const cy = Number(props.cy ?? 0);
  const midAngle = Number(props.midAngle ?? 0);
  const outerRadius = Number(props.outerRadius ?? 0);
  const percent = Number(props.percent ?? 0);
  if (percent < 0.04) return null;
  const RADIAN = Math.PI / 180;
  const radius = outerRadius * 1.15;
  const x = cx + radius * Math.cos(-midAngle * RADIAN);
  const y = cy + radius * Math.sin(-midAngle * RADIAN);
  return (
    <text
      x={x}
      y={y}
      textAnchor={x > cx ? "start" : "end"}
      dominantBaseline="central"
      fontSize={11}
      fill="currentColor"
      className="text-gray-600 dark:text-gray-300"
    >
      {`${(percent * 100).toFixed(1)}%`}
    </text>
  );
};

export const AssetPieChart: React.FC<AssetPieChartProps> = ({
  data,
  title,
  height = 300,
  innerRadius = 0,
  showLegend = true,
  onExportImage,
  className = "",
}) => {
  const slug = title?.replace(/\s+/g, "-") ?? "pie";

  const handleExport = useCallback(() => {
    const svgEl = document.querySelector<SVGSVGElement>(
      `.asset-pie-chart-${slug} svg`
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
    <div className={`asset-pie-chart-${slug} w-full ${className}`}>
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
        <RechartsPieChart>
          <Pie
            data={data}
            dataKey="value"
            nameKey="name"
            cx="50%"
            cy="50%"
            innerRadius={innerRadius}
            outerRadius="70%"
            labelLine={false}
            label={renderCustomLabel}
            isAnimationActive={false}
          >
            {data.map((entry, idx) => (
              <Cell
                key={entry.name}
                fill={entry.color ?? DEFAULT_COLORS[idx % DEFAULT_COLORS.length]}
              />
            ))}
          </Pie>
          <Tooltip
            formatter={(value) => Number(value).toLocaleString()}
            contentStyle={{ fontSize: 12, borderRadius: 8 }}
          />
          {showLegend && <Legend wrapperStyle={{ fontSize: 12 }} />}
        </RechartsPieChart>
      </ResponsiveContainer>
    </div>
  );
};

export default AssetPieChart;
