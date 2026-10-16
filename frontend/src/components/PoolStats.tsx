"use client";

import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Activity, Coins, Percent, TrendingUp } from "lucide-react";

export type LiquidityPool = {
  id: string;
  name: string;
  tokenA: string;
  tokenB: string;
  reserveA: number;
  reserveB: number;
  tvl: number;
  volume24h: number;
  fees24h: number;
  apr: number;
  lpTokenSupply: number;
  userLpTokens: number;
  userFeesEarned: number;
};

interface PoolStatsProps {
  pool: LiquidityPool;
}

const currency = new Intl.NumberFormat("en-US", {
  style: "currency",
  currency: "USD",
  maximumFractionDigits: 0,
});

const number = new Intl.NumberFormat("en-US", {
  maximumFractionDigits: 2,
});

export function PoolStats({ pool }: PoolStatsProps) {
  const tokenAShare = pool.reserveA / (pool.reserveA + pool.reserveB);
  const tokenBShare = 1 - tokenAShare;
  const userShare = (pool.userLpTokens / pool.lpTokenSupply) * 100;
  const userLiquidityValue = pool.tvl * (userShare / 100);

  return (
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <CardTitle>{pool.name}</CardTitle>
            <CardDescription>
              Pool composition, volume, fees, and your LP position.
            </CardDescription>
          </div>
          <Badge variant="secondary">{pool.apr.toFixed(2)}% APR</Badge>
        </div>
      </CardHeader>
      <CardContent className="space-y-6">
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <Stat label="TVL" value={currency.format(pool.tvl)} icon={Coins} />
          <Stat
            label="24h volume"
            value={currency.format(pool.volume24h)}
            icon={Activity}
          />
          <Stat
            label="24h fees"
            value={currency.format(pool.fees24h)}
            icon={TrendingUp}
          />
          <Stat
            label="Your share"
            value={`${userShare.toFixed(3)}%`}
            icon={Percent}
          />
        </div>

        <div className="space-y-3">
          <div className="flex items-center justify-between text-sm">
            <span className="font-medium">Pool composition</span>
            <span className="text-muted-foreground">
              {pool.tokenA}/{pool.tokenB}
            </span>
          </div>
          <div className="h-3 overflow-hidden rounded-full bg-muted">
            <div
              className="h-full bg-primary"
              style={{ width: `${tokenAShare * 100}%` }}
            />
          </div>
          <div className="grid gap-2 text-sm sm:grid-cols-2">
            <div className="rounded-lg border p-3">
              <div className="text-muted-foreground">{pool.tokenA}</div>
              <div className="mt-1 font-medium">
                {number.format(pool.reserveA)} reserves
              </div>
              <div className="text-xs text-muted-foreground">
                {(tokenAShare * 100).toFixed(1)}% of pool
              </div>
            </div>
            <div className="rounded-lg border p-3">
              <div className="text-muted-foreground">{pool.tokenB}</div>
              <div className="mt-1 font-medium">
                {number.format(pool.reserveB)} reserves
              </div>
              <div className="text-xs text-muted-foreground">
                {(tokenBShare * 100).toFixed(1)}% of pool
              </div>
            </div>
          </div>
        </div>

        <div className="grid gap-3 sm:grid-cols-3">
          <PositionMetric
            label="LP tokens"
            value={number.format(pool.userLpTokens)}
          />
          <PositionMetric
            label="Position value"
            value={currency.format(userLiquidityValue)}
          />
          <PositionMetric
            label="Fees earned"
            value={currency.format(pool.userFeesEarned)}
          />
        </div>
      </CardContent>
    </Card>
  );
}

function Stat({
  label,
  value,
  icon: Icon,
}: {
  label: string;
  value: string;
  icon: typeof Coins;
}) {
  return (
    <div className="rounded-lg border p-3">
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs text-muted-foreground">{label}</span>
        <Icon className="h-4 w-4 text-muted-foreground" />
      </div>
      <div className="mt-2 text-lg font-semibold">{value}</div>
    </div>
  );
}

function PositionMetric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg bg-muted p-3">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="mt-1 text-base font-semibold">{value}</div>
    </div>
  );
}
