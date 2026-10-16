'use client'

import { useMemo } from "react";
import { cn, formatCurrency } from "@/lib/utils";

interface DataPoint {
  timestamp: number;
  price: number;
  volume?: number;
}

interface PerformanceChartProps {
  data: DataPoint[];
  height?: number;
  showVolume?: boolean;
  className?: string;
}

export function PerformanceChart({ data, height = 200, showVolume = false, className }: PerformanceChartProps) {
  const { path, points, areaPath, minVal, maxVal } = useMemo(() => {
    if (data.length < 2) {
      return { path: "", points: [] as { x: number; y: number; volume?: number }[], areaPath: "", minVal: 0, maxVal: 0 };
    }

    const values = data.map((d) => d.price);
    const min = Math.min(...values);
    const max = Math.max(...values);
    const range = max - min || 1;
    const width = 100;

    const pts = data.map((d, i) => ({
      x: (i / (data.length - 1)) * width,
      y: height - ((d.price - min) / range) * (height - 20) - 10,
      volume: d.volume,
    }));

    const d = pts
      .map((p, i) => `${i === 0 ? "M" : "L"} ${p.x} ${p.y}`)
      .join(" ");

    const area = `${d} L ${pts[pts.length - 1].x} ${height} L 0 ${height} Z`;

    return { path: d, areaPath: area, points: pts, minVal: min, maxVal: max };
  }, [data, height]);

  if (data.length < 2) {
    return (
      <div className={cn("flex items-center justify-center text-muted-foreground", className)} style={{ height }}>
        Insufficient data
      </div>
    );
  }

  const isPositive = data[data.length - 1].price >= data[0].price;
  const lineColor = isPositive ? "#22c55e" : "#ef4444";
  const areaColor = isPositive ? "#22c55e" : "#ef4444";

  return (
    <div className={cn("space-y-2", className)}>
      <svg
        viewBox={`0 0 100 ${height}`}
        className="w-full"
        style={{ height }}
        preserveAspectRatio="none"
      >
        {[0, 0.25, 0.5, 0.75, 1].map((ratio) => (
          <line
            key={ratio}
            x1="0"
            y1={height - ratio * (height - 20) - 10}
            x2="100"
            y2={height - ratio * (height - 20) - 10}
            stroke="hsl(var(--border))"
            strokeWidth="0.5"
            strokeDasharray="2 2"
          />
        ))}

        <path d={areaPath} fill={areaColor} opacity="0.1" />
        <path
          d={path}
          fill="none"
          stroke={lineColor}
          strokeWidth="1.5"
          strokeLinecap="round"
          strokeLinejoin="round"
        />

        {points.length > 0 && (
          <>
            <circle cx={points[0].x} cy={points[0].y} r="2" fill={lineColor} />
            <circle cx={points[points.length - 1].x} cy={points[points.length - 1].y} r="2" fill={lineColor} />
          </>
        )}
      </svg>

      <div className="flex justify-between text-xs text-muted-foreground">
        <span>{formatCurrency(maxVal)}</span>
        <span>{formatCurrency((maxVal + minVal) / 2)}</span>
        <span>{formatCurrency(minVal)}</span>
      </div>

      {showVolume && (
        <div className="flex items-end gap-px h-8 mt-1">
          {points.map((p, i) => {
            const maxVolume = Math.max(...points.map((pt) => pt.volume || 1));
            return (
              <div
                key={i}
                className="flex-1 bg-muted-foreground/20 rounded-t"
                style={{ height: `${Math.max(5, ((p.volume || 0) / maxVolume) * 100)}%` }}
              />
            );
          })}
        </div>
      )}
    </div>
  );
}
