'use client'

import { useMemo } from "react";
import { cn } from "@/lib/utils";

const COLORS = [
  "#3b82f6", "#10b981", "#f59e0b", "#ef4444", "#8b5cf6",
  "#ec4899", "#06b6d4", "#84cc16", "#f97316", "#6366f1",
];

interface AllocationSlice {
  name: string;
  value: number;
  percentage: number;
}

interface AssetAllocationChartProps {
  data: AllocationSlice[];
  className?: string;
}

export function AssetAllocationChart({ data, className }: AssetAllocationChartProps) {
  const total = useMemo(() => data.reduce((acc, s) => acc + s.value, 0), [data]);

  if (data.length === 0) {
    return (
      <div className={cn("flex items-center justify-center h-48 text-muted-foreground", className)}>
        No allocation data available
      </div>
    );
  }

  const radius = 100;
  const center = 120;

  let currentAngle = -Math.PI / 2;
  const slices = data.map((slice, i) => {
    const sliceAngle = (slice.value / total) * 2 * Math.PI;
    const startAngle = currentAngle;
    currentAngle += sliceAngle;

    const x1 = center + radius * Math.cos(startAngle);
    const y1 = center + radius * Math.sin(startAngle);
    const x2 = center + radius * Math.cos(startAngle + sliceAngle);
    const y2 = center + radius * Math.sin(startAngle + sliceAngle);

    const largeArc = sliceAngle > Math.PI ? 1 : 0;

    return {
      ...slice,
      index: i,
      path: `M ${center} ${center} L ${x1} ${y1} A ${radius} ${radius} 0 ${largeArc} 1 ${x2} ${y2} Z`,
    };
  });

  return (
    <div className={cn("space-y-4", className)}>
      <div className="relative mx-auto" style={{ width: 240, height: 240 }}>
        <svg viewBox="0 0 240 240" className="w-full h-full">
          {slices.map((slice) => (
            <path
              key={slice.index}
              d={slice.path}
              fill={COLORS[slice.index % COLORS.length]}
              stroke="hsl(var(--background))"
              strokeWidth="2"
              className="hover:opacity-80 transition-opacity cursor-pointer"
            />
          ))}
        </svg>
        <div className="absolute inset-0 flex items-center justify-center">
          <div className="text-center">
            <div className="text-xl font-bold">{data.length}</div>
            <div className="text-xs text-muted-foreground">Asset Types</div>
          </div>
        </div>
      </div>
      <div className="space-y-2">
        {slices.map((slice) => (
          <div key={slice.index} className="flex items-center justify-between text-sm">
            <div className="flex items-center gap-2">
              <div
                className="h-3 w-3 rounded-full"
                style={{ backgroundColor: COLORS[slice.index % COLORS.length] }}
              />
              <span>{slice.name}</span>
            </div>
            <div className="flex items-center gap-3">
              <span className="text-muted-foreground">{slice.percentage.toFixed(1)}%</span>
              <span className="font-medium tabular-nums">
                ${slice.value.toLocaleString()}
              </span>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
