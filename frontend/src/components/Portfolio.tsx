'use client'

import { useState, useEffect } from "react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { AssetAllocationChart } from "@/components/charts/AssetAllocationChart";
import { PerformanceChart } from "@/components/charts/PerformanceChart";
import { assetApi, PortfolioData } from "@/lib/asset-api";
import { formatCurrency, formatDate, truncateAddress } from "@/lib/utils";
import {
  TrendingUp, TrendingDown, Wallet, Clock, Download, ArrowUpRight,
  ArrowDownRight, RefreshCw, PieChart, Activity, Bell,
} from "lucide-react";

export function Portfolio() {
  const [data, setData] = useState<PortfolioData | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [timeRange, setTimeRange] = useState<"7d" | "30d" | "1y">("30d");

  useEffect(() => {
    const load = async () => {
      try {
        const d = await assetApi.getDashboardData();
        setData(d);
      } catch {
        console.error("Failed to load dashboard");
      } finally {
        setIsLoading(false);
      }
    };
    load();
  }, []);

  const handleExport = async () => {
    try {
      const blob = await assetApi.exportTransactionsCsv();
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `transactions-${new Date().toISOString().split("T")[0]}.csv`;
      a.click();
      URL.revokeObjectURL(url);
    } catch {
      console.error("Export failed");
    }
  };

  if (isLoading) {
    return (
      <div className="space-y-6 animate-pulse">
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          {[...Array(3)].map((_, i) => (
            <Card key={i}><CardContent className="p-6"><div className="h-16 bg-muted rounded" /></CardContent></Card>
          ))}
        </div>
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          <Card><CardContent className="p-6"><div className="h-64 bg-muted rounded" /></CardContent></Card>
          <Card><CardContent className="p-6"><div className="h-64 bg-muted rounded" /></CardContent></Card>
        </div>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="text-center py-12">
        <Wallet className="h-12 w-12 mx-auto text-muted-foreground mb-4" />
        <h3 className="text-lg font-semibold mb-2">No Portfolio Data</h3>
        <p className="text-muted-foreground">Connect your wallet to view your portfolio.</p>
      </div>
    );
  }

  const allocationData = data.holdings.map((h) => ({
    name: h.symbol,
    value: h.value,
    percentage: h.allocation,
  }));

  return (
    <div className="space-y-6">
      {/* Summary Cards */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <Card>
          <CardHeader className="pb-2">
            <CardDescription className="flex items-center gap-2">
              <Wallet className="h-4 w-4" />
              Portfolio Value
            </CardDescription>
            <CardTitle className="text-2xl">{formatCurrency(data.totalValue)}</CardTitle>
          </CardHeader>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardDescription className="flex items-center gap-2">
              {data.pnlPercentage >= 0 ? <TrendingUp className="h-4 w-4 text-green-500" /> : <TrendingDown className="h-4 w-4 text-red-500" />}
              P&L (24h)
            </CardDescription>
            <CardTitle className={`text-2xl ${data.pnlPercentage >= 0 ? "text-green-500" : "text-red-500"}`}>
              {data.pnlPercentage >= 0 ? "+" : ""}{formatCurrency(data.pnl24h)}
              <span className="text-sm ml-2">
                ({data.pnlPercentage >= 0 ? "+" : ""}{data.pnlPercentage.toFixed(2)}%)
              </span>
            </CardTitle>
          </CardHeader>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardDescription className="flex items-center gap-2">
              <PieChart className="h-4 w-4" />
              Holdings
            </CardDescription>
            <CardTitle className="text-2xl">{data.holdings.length}</CardTitle>
          </CardHeader>
        </Card>
      </div>

      {/* Charts */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <Card className="lg:col-span-2">
          <CardHeader>
            <div className="flex items-center justify-between">
              <div>
                <CardTitle className="text-lg">Performance</CardTitle>
                <CardDescription>Portfolio value over time</CardDescription>
              </div>
              <div className="flex gap-1">
                {(["7d", "30d", "1y"] as const).map((range) => (
                  <Button
                    key={range}
                    variant={timeRange === range ? "default" : "outline"}
                    size="sm"
                    onClick={() => setTimeRange(range)}
                  >
                    {range}
                  </Button>
                ))}
              </div>
            </div>
          </CardHeader>
          <CardContent>
            <PerformanceChart data={data.performanceHistory} height={220} showVolume />
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-lg">Asset Allocation</CardTitle>
            <CardDescription>By value</CardDescription>
          </CardHeader>
          <CardContent>
            <AssetAllocationChart data={allocationData} />
          </CardContent>
        </Card>
      </div>

      {/* Holdings Table + Activity */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Holdings */}
        <Card className="lg:col-span-2">
          <CardHeader>
            <CardTitle className="text-lg">Holdings</CardTitle>
            <CardDescription>Your asset positions</CardDescription>
          </CardHeader>
          <CardContent className="p-0">
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead>
                  <tr className="border-b text-sm text-muted-foreground">
                    <th className="text-left p-4 font-medium">Asset</th>
                    <th className="text-right p-4 font-medium">Balance</th>
                    <th className="text-right p-4 font-medium">Value</th>
                    <th className="text-right p-4 font-medium">24h</th>
                    <th className="text-right p-4 font-medium">Allocation</th>
                  </tr>
                </thead>
                <tbody>
                  {data.holdings.map((h) => (
                    <tr key={h.assetId} className="border-b last:border-0 hover:bg-muted/50">
                      <td className="p-4">
                        <div className="font-medium">{h.name}</div>
                        <div className="text-sm text-muted-foreground">{h.symbol}</div>
                      </td>
                      <td className="p-4 text-right tabular-nums">{h.balance.toLocaleString()}</td>
                      <td className="p-4 text-right tabular-nums">{formatCurrency(h.value)}</td>
                      <td className={`p-4 text-right tabular-nums ${h.change24h >= 0 ? "text-green-500" : "text-red-500"}`}>
                        {h.change24h >= 0 ? "+" : ""}{h.change24h}%
                      </td>
                      <td className="p-4 text-right tabular-nums">{h.allocation.toFixed(1)}%</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </CardContent>
        </Card>

        {/* Quick Actions + Pending */}
        <div className="space-y-6">
          {/* Quick Actions */}
          <Card>
            <CardHeader>
              <CardTitle className="text-lg">Quick Actions</CardTitle>
            </CardHeader>
            <CardContent className="space-y-2">
              <Button className="w-full justify-start" variant="outline">
                <RefreshCw className="h-4 w-4 mr-2" /> Deposit Funds
              </Button>
              <Button className="w-full justify-start" variant="outline">
                <ArrowUpRight className="h-4 w-4 mr-2" /> Withdraw
              </Button>
              <Button className="w-full justify-start" variant="outline">
                <Download className="h-4 w-4 mr-2" onClick={handleExport} /> Export CSV
              </Button>
            </CardContent>
          </Card>

          {/* Pending Actions */}
          <Card>
            <CardHeader>
              <CardTitle className="text-lg flex items-center gap-2">
                <Clock className="h-4 w-4" /> Pending
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              {data.pendingActions.length === 0 ? (
                <p className="text-sm text-muted-foreground">No pending actions</p>
              ) : (
                data.pendingActions.map((action) => (
                  <div key={action.id} className="flex items-start gap-3 p-2 rounded-lg bg-muted/50">
                    <Bell className="h-4 w-4 mt-0.5 text-muted-foreground" />
                    <div className="flex-1 min-w-0">
                      <p className="text-sm font-medium">{action.action}</p>
                      <p className="text-xs text-muted-foreground truncate">{action.assetName}</p>
                      <p className="text-xs text-muted-foreground">{formatDate(action.created)}</p>
                    </div>
                  </div>
                ))
              )}
            </CardContent>
          </Card>
        </div>
      </div>

      {/* Recent Activity Feed */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle className="text-lg">Recent Activity</CardTitle>
              <CardDescription>Your transaction history</CardDescription>
            </div>
            <Button variant="outline" size="sm" onClick={handleExport}>
              <Download className="h-4 w-4 mr-2" /> Export CSV
            </Button>
          </div>
        </CardHeader>
        <CardContent className="p-0">
          <div className="divide-y">
            {data.recentActivity.map((tx) => (
              <div key={tx.id} className="flex items-center justify-between p-4 hover:bg-muted/50">
                <div className="flex items-center gap-3">
                  <div className={`p-2 rounded-full ${tx.type === "buy" ? "bg-green-500/10 text-green-500" : "bg-muted text-muted-foreground"}`}>
                    <Activity className="h-4 w-4" />
                  </div>
                  <div>
                    <p className="text-sm font-medium capitalize">{tx.type}</p>
                    <p className="text-xs text-muted-foreground">{tx.assetName}</p>
                  </div>
                </div>
                <div className="text-right">
                  <p className="text-sm font-medium tabular-nums">{tx.amount.toLocaleString()}</p>
                  <p className="text-xs text-muted-foreground">{formatDate(tx.timestamp)}</p>
                </div>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
