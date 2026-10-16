"use client";

import { useState } from "react";
import { Header } from "@/components/Header";
import { RewardsClaim } from "@/components/RewardsClaim";
import { StakingCard, StakingPool } from "@/components/StakingCard";
import { Card, CardContent } from "@/components/ui/card";
import { WalletConnect } from "@/components/WalletConnect";
import { StellarWallet } from "@/lib/stellar";

const stakingPools: StakingPool[] = [
  {
    id: "kor-core",
    asset: "KOR",
    name: "Core rewards",
    apy: 12.4,
    totalStaked: 4200000,
    userStaked: 18500,
    pendingRewards: 128.42,
    lockPeriodDays: 14,
    rewardToken: "KOR",
    history: [
      { date: "2026-06-21", action: "Claimed rewards", amount: 94.2 },
      { date: "2026-06-12", action: "Added stake", amount: 2500 },
      { date: "2026-05-28", action: "Unstaked", amount: 1000 },
    ],
  },
  {
    id: "estate-yield",
    asset: "ESTATE",
    name: "Real estate yield",
    apy: 8.9,
    totalStaked: 1275000,
    userStaked: 6200,
    pendingRewards: 43.18,
    lockPeriodDays: 30,
    rewardToken: "KOR",
    history: [
      { date: "2026-06-19", action: "Added stake", amount: 1200 },
      { date: "2026-06-01", action: "Claimed rewards", amount: 31.4 },
      { date: "2026-05-11", action: "Added stake", amount: 5000 },
    ],
  },
  {
    id: "art-vault",
    asset: "ART",
    name: "Collectibles vault",
    apy: 16.1,
    totalStaked: 530000,
    userStaked: 3100,
    pendingRewards: 76.03,
    lockPeriodDays: 45,
    rewardToken: "KOR",
    history: [
      { date: "2026-06-23", action: "Added stake", amount: 600 },
      { date: "2026-06-05", action: "Claimed rewards", amount: 52.8 },
      { date: "2026-05-17", action: "Added stake", amount: 2500 },
    ],
  },
];

const number = new Intl.NumberFormat("en-US", {
  maximumFractionDigits: 2,
});

export default function StakingPage() {
  const [wallet, setWallet] = useState<StellarWallet | undefined>();
  const totalStaked = stakingPools.reduce((sum, pool) => sum + pool.userStaked, 0);
  const averageApy =
    stakingPools.reduce((sum, pool) => sum + pool.apy, 0) / stakingPools.length;
  const projectedAnnualRewards = stakingPools.reduce(
    (sum, pool) => sum + pool.userStaked * (pool.apy / 100),
    0,
  );

  return (
    <div className="min-h-screen bg-background">
      <Header wallet={wallet} />
      <main className="container mx-auto max-w-6xl px-4 py-8">
        <div className="mb-8 flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <h1 className="text-3xl font-bold">Staking</h1>
            <p className="mt-1 text-muted-foreground">
              View staked assets, claim rewards, unstake, and compare pool analytics.
            </p>
          </div>
          <WalletConnect
            onWalletConnected={setWallet}
            onWalletDisconnected={() => setWallet(undefined)}
            wallet={wallet}
          />
        </div>

        <div className="mb-6 grid gap-3 sm:grid-cols-3">
          <Summary label="Total staked" value={number.format(totalStaked)} />
          <Summary label="Average APY" value={`${averageApy.toFixed(2)}%`} />
          <Summary
            label="Projected annual rewards"
            value={`${number.format(projectedAnnualRewards)} KOR`}
          />
        </div>

        <div className="grid gap-6 lg:grid-cols-[1fr_340px]">
          <div className="space-y-6">
            {stakingPools.map((pool) => (
              <StakingCard key={pool.id} pool={pool} />
            ))}
          </div>
          <RewardsClaim pools={stakingPools} />
        </div>
      </main>
    </div>
  );
}

function Summary({ label, value }: { label: string; value: string }) {
  return (
    <Card>
      <CardContent className="py-4">
        <div className="text-sm text-muted-foreground">{label}</div>
        <div className="mt-1 text-2xl font-semibold">{value}</div>
      </CardContent>
    </Card>
  );
}
