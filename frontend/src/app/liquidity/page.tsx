"use client";

import { useMemo, useState } from "react";
import { AddLiquidity } from "@/components/AddLiquidity";
import { Header } from "@/components/Header";
import { LiquidityPool, PoolStats } from "@/components/PoolStats";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { WalletConnect } from "@/components/WalletConnect";
import { StellarWallet } from "@/lib/stellar";

const pools: LiquidityPool[] = [
  {
    id: "kor-usdc",
    name: "KOR / USDC",
    tokenA: "KOR",
    tokenB: "USDC",
    reserveA: 820000,
    reserveB: 615000,
    tvl: 1230000,
    volume24h: 184500,
    fees24h: 553,
    apr: 14.8,
    lpTokenSupply: 98000,
    userLpTokens: 1240,
    userFeesEarned: 318,
  },
  {
    id: "estate-usdc",
    name: "ESTATE / USDC",
    tokenA: "ESTATE",
    tokenB: "USDC",
    reserveA: 430000,
    reserveB: 752500,
    tvl: 1505000,
    volume24h: 96800,
    fees24h: 290,
    apr: 9.6,
    lpTokenSupply: 76000,
    userLpTokens: 850,
    userFeesEarned: 177,
  },
  {
    id: "art-usdc",
    name: "ART / USDC",
    tokenA: "ART",
    tokenB: "USDC",
    reserveA: 215000,
    reserveB: 387000,
    tvl: 774000,
    volume24h: 41200,
    fees24h: 124,
    apr: 7.1,
    lpTokenSupply: 41000,
    userLpTokens: 620,
    userFeesEarned: 83,
  },
];

export default function LiquidityPage() {
  const [wallet, setWallet] = useState<StellarWallet | undefined>();
  const [selectedPoolId, setSelectedPoolId] = useState(pools[0].id);
  const [initialPrice, setInitialPrice] = useState("1");
  const [futurePrice, setFuturePrice] = useState("1.25");

  const selectedPool =
    pools.find((pool) => pool.id === selectedPoolId) ?? pools[0];

  const impermanentLoss = useMemo(() => {
    const start = Number(initialPrice) || 1;
    const end = Number(futurePrice) || start;
    const ratio = end / start;
    const loss = (2 * Math.sqrt(ratio)) / (1 + ratio) - 1;
    return Math.abs(loss * 100);
  }, [futurePrice, initialPrice]);

  return (
    <div className="min-h-screen bg-background">
      <Header wallet={wallet} />
      <main className="container mx-auto max-w-6xl px-4 py-8">
        <div className="mb-8 flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <h1 className="text-3xl font-bold">Liquidity pools</h1>
            <p className="mt-1 text-muted-foreground">
              Add or remove liquidity, monitor pool health, and track LP returns.
            </p>
          </div>
          <WalletConnect
            onWalletConnected={setWallet}
            onWalletDisconnected={() => setWallet(undefined)}
            wallet={wallet}
          />
        </div>

        <div className="grid gap-6 lg:grid-cols-[1fr_360px]">
          <div className="space-y-6">
            <PoolStats pool={selectedPool} />

            <Card>
              <CardHeader>
                <CardTitle>Impermanent loss calculator</CardTitle>
                <CardDescription>
                  Estimate relative LP underperformance if prices move after deposit.
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="grid gap-3 sm:grid-cols-2">
                  <div className="space-y-2">
                    <Label htmlFor="initial-price">Initial price</Label>
                    <Input
                      id="initial-price"
                      type="number"
                      min="0.0001"
                      step="0.01"
                      value={initialPrice}
                      onChange={(event) => setInitialPrice(event.target.value)}
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="future-price">Future price</Label>
                    <Input
                      id="future-price"
                      type="number"
                      min="0.0001"
                      step="0.01"
                      value={futurePrice}
                      onChange={(event) => setFuturePrice(event.target.value)}
                    />
                  </div>
                </div>
                <div className="rounded-lg bg-muted p-4">
                  <div className="text-sm text-muted-foreground">
                    Estimated impermanent loss
                  </div>
                  <div className="mt-1 text-3xl font-semibold">
                    {impermanentLoss.toFixed(2)}%
                  </div>
                </div>
              </CardContent>
            </Card>
          </div>

          <AddLiquidity
            pools={pools}
            selectedPoolId={selectedPool.id}
            onPoolChange={setSelectedPoolId}
          />
        </div>
      </main>
    </div>
  );
}
