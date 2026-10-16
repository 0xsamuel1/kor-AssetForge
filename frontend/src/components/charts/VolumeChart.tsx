"use client"

import { useMemo } from "react"
import { cn, formatCurrency } from "@/lib/utils"

export interface VolumeDataPoint {
  label: string   // e.g. "Jan 1", "Mon", "Week 1"
  volume: number
}

interface VolumeChartProps {
  data: VolumeDataPoint[]
  height?: number
  className?: string
}

export function VolumeChart({ data, height = 180, className }: VolumeChartProps) {
  const { bars, max } = useMemo(() => {
    if (data.length === 0) return { bars: [], max: 0 }
    const maxVol = Math.max(...data.map((d) => d.volume))
    return {
      bars: data.map((d) => ({
        ...d,
        heightPct: maxVol > 0 ? (d.volume / maxVol) * 100 : 0,
      })),
      max: maxVol,
    }
  }, [data])

  if (data.length === 0) {
    return (
      <div
        className={cn(
          "flex items-center justify-center text-sm text-muted-foreground",
          className
        )}
        style={{ height }}
      >
        No volume data
      </div>
    )
  }

  // Show at most every Nth label to avoid crowding
  const labelStep = Math.ceil(data.length / 8)

  return (
    <div className={cn("space-y-2", className)}>
      <div
        className="flex items-end gap-px w-full"
        style={{ height }}
        aria-label="Volume bar chart"
        role="img"
      >
        {bars.map((bar, i) => (
          <div
            key={i}
            className="group relative flex-1 flex flex-col justify-end"
            style={{ height: "100%" }}
          >
            <div
              className="w-full bg-primary/60 hover:bg-primary transition-colors rounded-t"
              style={{ height: `${bar.heightPct}%` }}
            />
            {/* tooltip */}
            <div className="absolute bottom-full mb-1 left-1/2 -translate-x-1/2 z-10 hidden group-hover:block pointer-events-none">
              <div className="bg-popover text-popover-foreground text-xs rounded px-2 py-1 whitespace-nowrap shadow-md ring-1 ring-border">
                <span className="font-medium">{bar.label}</span>
                <br />
                {formatCurrency(bar.volume)}
              </div>
            </div>
          </div>
        ))}
      </div>

      {/* x-axis labels */}
      <div className="flex w-full text-xs text-muted-foreground">
        {bars.map((bar, i) => (
          <div key={i} className="flex-1 text-center truncate">
            {i % labelStep === 0 ? bar.label : ""}
          </div>
        ))}
      </div>

      {/* y-axis max reference */}
      <div className="text-xs text-muted-foreground text-right">
        Peak: {formatCurrency(max)}
      </div>
    </div>
  )
}
