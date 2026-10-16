"use client"

import { useState, useEffect, useCallback } from "react"
import { Header } from "@/components/Header"
import { MetricsCard } from "@/components/MetricsCard"
import { VolumeChart, VolumeDataPoint } from "@/components/charts/VolumeChart"
import { SkeletonMetric, SkeletonChart } from "@/components/ui/skeleton"
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { StellarWallet } from "@/lib/stellar"
import { formatCurrency } from "@/lib/utils"
import {
  TrendingUp,
  Users,
  Activity,
  BarChart2,
  RefreshCw,
} from "lucide-react"

type TimeRange = "24h" | "7d" | "30d" | "all"

interface PlatformMetrics {
  tvl: number
  tvlChange: number
  volume24h: number
  volumeChange: number
  activeUsers: number
  userGrowth: number
  totalTransactions: number
  txGrowth: number
  volumeHistory: VolumeDataPoint[]
  topAssets: { name: string; symbol: string; volume: number; change: number }[]
}

// Generate deterministic mock data for a given time range so the UI
// doesn't flicker on re-render while there's no real API endpoint.
function generateMockMetrics(range: TimeRange): PlatformMetrics {
  const seed: Record<TimeRange, number> = { "24h": 1, "7d": 7, "30d": 30, all: 365 }
  const multiplier = seed[range]

  const points = range === "24h" ? 24 : range === "7d" ? 7 : range === "30d" ? 30 : 52
  const labels =
    range === "24h"
      ? Array.from({ length: 24 }, (_, i) => `${i}:00`)
      : range === "7d"
      ? ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"]
      : range === "30d"
      ? Array.from({ length: 30 }, (_, i) => `D${i + 1}`)
      : Array.from({ length: 52 }, (_, i) => `W${i + 1}`)

  const volumeHistory: VolumeDataPoint[] = labels.map((label, i) => ({
    label,
    volume: Math.round(50_000 + Math.sin(i * 0.7) * 30_000 + i * multiplier * 200),
  }))

  return {
    tvl: 12_400_000 + multiplier * 80_000,
    tvlChange: 3.4 + multiplier * 0.01,
    volume24h: 2_100_000 + multiplier * 15_000,
    volumeChange: 8.7 - multiplier * 0.05,
    activeUsers: 4_823 + multiplier * 12,
    userGrowth: 12.1 + multiplier * 0.02,
    totalTransactions: 98_341 + multiplier * 500,
    txGrowth: 5.2,
    volumeHistory,
    topAssets: [
      { name: "Downtown Office Tower", symbol: "DOT", volume: 420_000, change: 14.2 },
      { name: "Harbor Warehouse Fund", symbol: "HWF", volume: 310_000, change: -3.1 },
      { name: "Retail Plaza Token",    symbol: "RPT", volume: 280_000, change: 7.8 },
      { name: "Luxury Residences",     symbol: "LRX", volume: 195_000, change: 21.4 },
      { name: "Industrial Park Token", symbol: "IPT", volume: 160_000, change: -1.5 },
    ],
  }
}

const TIME_RANGES: { label: string; value: TimeRange }[] = [
  { label: "24h",  value: "24h" },
  { label: "7d",   value: "7d" },
  { label: "30d",  value: "30d" },
  { label: "All",  value: "all" },
]

export default function AnalyticsPage() {
  const [wallet, setWallet] = useState<StellarWallet | undefined>()
  const [range, setRange] = useState<TimeRange>("7d")
  const [metrics, setMetrics] = useState<PlatformMetrics | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  const [lastUpdated, setLastUpdated] = useState<Date | null>(null)

  const api = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080"

  const loadMetrics = useCallback(async () => {
    setIsLoading(true)
    try {
      const res = await fetch(`${api}/api/analytics/metrics?range=${range}`)
      if (res.ok) {
        const data = await res.json()
        setMetrics(data)
      } else {
        // Fall back to mock data when the endpoint isn't available yet
        setMetrics(generateMockMetrics(range))
      }
    } catch {
      setMetrics(generateMockMetrics(range))
    } finally {
      setIsLoading(false)
      setLastUpdated(new Date())
    }
  }, [api, range])

  useEffect(() => {
    loadMetrics()
  }, [loadMetrics])

  return (
    <div className="min-h-screen bg-background">
      <Header wallet={wallet} />

      <main className="container mx-auto px-4 py-8 max-w-6xl">
        {/* Page header */}
        <div className="flex items-start justify-between mb-8 gap-4 flex-wrap">
          <div>
            <h1 className="text-3xl font-bold">Analytics</h1>
            <p className="text-muted-foreground mt-1">
              Platform metrics, trading volume, and growth trends
            </p>
            {lastUpdated && (
              <p className="text-xs text-muted-foreground mt-1">
                Last updated: {lastUpdated.toLocaleTimeString()}
              </p>
            )}
          </div>

          <div className="flex items-center gap-2 flex-wrap">
            {/* Time range selector */}
            <div className="flex rounded-lg overflow-hidden border border-border">
              {TIME_RANGES.map(({ label, value }) => (
                <button
                  key={value}
                  onClick={() => setRange(value)}
                  className={`px-3 py-1.5 text-sm font-medium transition-colors ${
                    range === value
                      ? "bg-primary text-primary-foreground"
                      : "bg-background text-muted-foreground hover:bg-muted"
                  }`}
                >
                  {label}
                </button>
              ))}
            </div>

            <Button
              variant="outline"
              size="icon"
              onClick={loadMetrics}
              disabled={isLoading}
              aria-label="Refresh metrics"
            >
              <RefreshCw className={`h-4 w-4 ${isLoading ? "animate-spin" : ""}`} />
            </Button>
          </div>
        </div>

        {/* Top metrics row */}
        {isLoading ? (
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 mb-8">
            {Array.from({ length: 4 }).map((_, i) => (
              <SkeletonMetric key={i} />
            ))}
          </div>
        ) : metrics ? (
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 mb-8">
            <MetricsCard
              title="Total Value Locked"
              value={formatCurrency(metrics.tvl)}
              change={metrics.tvlChange}
              changeLabel="vs previous period"
              icon={<BarChart2 className="h-5 w-5" />}
            />
            <MetricsCard
              title="24h Volume"
              value={formatCurrency(metrics.volume24h)}
              change={metrics.volumeChange}
              changeLabel="vs previous period"
              icon={<Activity className="h-5 w-5" />}
            />
            <MetricsCard
              title="Active Users"
              value={metrics.activeUsers.toLocaleString()}
              change={metrics.userGrowth}
              changeLabel="growth"
              icon={<Users className="h-5 w-5" />}
            />
            <MetricsCard
              title="Total Transactions"
              value={metrics.totalTransactions.toLocaleString()}
              change={metrics.txGrowth}
              changeLabel="vs previous period"
              icon={<TrendingUp className="h-5 w-5" />}
            />
          </div>
        ) : null}

        {/* Volume chart */}
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6 mb-6">
          <Card className="lg:col-span-2">
            <CardHeader>
              <CardTitle className="text-lg">Trading Volume</CardTitle>
              <CardDescription>
                Volume over the selected time range
              </CardDescription>
            </CardHeader>
            <CardContent>
              {isLoading ? (
                <SkeletonChart height={180} />
              ) : metrics ? (
                <VolumeChart data={metrics.volumeHistory} height={180} />
              ) : null}
            </CardContent>
          </Card>

          {/* Top assets */}
          <Card>
            <CardHeader>
              <CardTitle className="text-lg">Top Assets</CardTitle>
              <CardDescription>By trading volume</CardDescription>
            </CardHeader>
            <CardContent className="p-0">
              {isLoading ? (
                <div className="px-4 pb-4 space-y-3 pt-2">
                  {Array.from({ length: 5 }).map((_, i) => (
                    <div key={i} className="flex justify-between gap-2">
                      <div className="flex-1 h-4 bg-muted rounded animate-pulse" />
                      <div className="w-16 h-4 bg-muted rounded animate-pulse" />
                    </div>
                  ))}
                </div>
              ) : metrics ? (
                <div className="divide-y">
                  {metrics.topAssets.map((asset, i) => (
                    <div
                      key={asset.symbol}
                      className="flex items-center justify-between px-4 py-3 hover:bg-muted/50"
                    >
                      <div className="flex items-center gap-3 min-w-0">
                        <span className="text-sm text-muted-foreground w-4 shrink-0">
                          {i + 1}
                        </span>
                        <div className="min-w-0">
                          <p className="text-sm font-medium truncate">
                            {asset.name}
                          </p>
                          <p className="text-xs text-muted-foreground">
                            {asset.symbol}
                          </p>
                        </div>
                      </div>
                      <div className="text-right shrink-0 ml-2">
                        <p className="text-sm font-medium tabular-nums">
                          {formatCurrency(asset.volume)}
                        </p>
                        <p
                          className={`text-xs tabular-nums ${
                            asset.change >= 0
                              ? "text-green-500"
                              : "text-red-500"
                          }`}
                        >
                          {asset.change >= 0 ? "+" : ""}
                          {asset.change.toFixed(1)}%
                        </p>
                      </div>
                    </div>
                  ))}
                </div>
              ) : null}
            </CardContent>
          </Card>
        </div>
      </main>
    </div>
  )
}
