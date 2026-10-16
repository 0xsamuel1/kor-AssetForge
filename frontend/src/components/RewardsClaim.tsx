"use client";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { StakingPool } from "@/components/StakingCard";
import { Gift, Sparkles } from "lucide-react";

interface RewardsClaimProps {
  pools: StakingPool[];
}

const number = new Intl.NumberFormat("en-US", {
  maximumFractionDigits: 2,
});

export function RewardsClaim({ pools }: RewardsClaimProps) {
  const totalRewards = pools.reduce((sum, pool) => sum + pool.pendingRewards, 0);
  const activePools = pools.filter((pool) => pool.pendingRewards > 0);

  return (
    <Card>
      <CardHeader>
        <CardTitle>Rewards</CardTitle>
        <CardDescription>
          Claim pending rewards across every active staking pool.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="rounded-lg bg-muted p-4">
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Gift className="h-4 w-4" />
            Total claimable
          </div>
          <div className="mt-2 text-3xl font-semibold">
            {number.format(totalRewards)} KOR
          </div>
        </div>

        <Button className="w-full" disabled={totalRewards === 0}>
          <Sparkles className="h-4 w-4" />
          Claim all rewards
        </Button>

        <div className="space-y-2">
          {activePools.map((pool) => (
            <div
              key={pool.id}
              className="flex items-center justify-between gap-3 rounded-lg border p-3 text-sm"
            >
              <span className="font-medium">{pool.asset}</span>
              <span>
                {number.format(pool.pendingRewards)} {pool.rewardToken}
              </span>
            </div>
          ))}
        </div>
      </CardContent>
    </Card>
  );
}
