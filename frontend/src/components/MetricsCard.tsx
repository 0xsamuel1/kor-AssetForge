"use client"

import { cn } from "@/lib/utils"
import { Card, CardContent, CardHeader } from "@/components/ui/card"
import { TrendingUp, TrendingDown } from "lucide-react"

export interface MetricsCardProps {
  title: string
  value: string
  change?: number   // percentage change, positive or negative
  changeLabel?: string
  icon?: React.ReactNode
  className?: string
}

export function MetricsCard({
  title,
  value,
  change,
  changeLabel,
  icon,
  className,
}: MetricsCardProps) {
  const isPositive = change !== undefined && change >= 0
  const hasChange = change !== undefined

  return (
    <Card className={cn("", className)}>
      <CardHeader className="pb-2">
        <div className="flex items-start justify-between gap-2">
          <p className="text-sm text-muted-foreground">{title}</p>
          {icon && (
            <div className="shrink-0 text-muted-foreground">{icon}</div>
          )}
        </div>
        <p className="text-2xl font-bold tabular-nums">{value}</p>
      </CardHeader>
      {hasChange && (
        <CardContent>
          <div
            className={cn(
              "flex items-center gap-1 text-sm font-medium",
              isPositive ? "text-green-500" : "text-red-500"
            )}
          >
            {isPositive ? (
              <TrendingUp className="h-3.5 w-3.5" />
            ) : (
              <TrendingDown className="h-3.5 w-3.5" />
            )}
            <span>
              {isPositive ? "+" : ""}
              {change.toFixed(1)}%
            </span>
            {changeLabel && (
              <span className="text-muted-foreground font-normal">
                {changeLabel}
              </span>
            )}
          </div>
        </CardContent>
      )}
    </Card>
  )
}
