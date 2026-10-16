"use client";

import { useMemo, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { CalendarClock, Coins, LineChart, Lock, Unlock } from "lucide-react";

export type StakingPool = {
  id: string;
  asset: string;
  name: string;
  apy: number;
  totalStaked: number;
  userStaked: number;
  pendingRewards: number;
  lockPeriodDays: number;
  rewardToken: string;
  history: {
    date: string;
    action: string;
    amount: number;
  }[];
};

interface StakingCardProps {
  pool: StakingPool;
}

const number = new Intl.NumberFormat("en-US", {
  maximumFractionDigits: 2,
});

export function StakingCard({ pool }: StakingCardProps) {
  const [stakeAmount, setStakeAmount] = useState("250");
  const [projectionDays, setProjectionDays] = useState(90);

  const projectedRewards = useMemo(() => {
    const principal = pool.userStaked + (Number(stakeAmount) || 0);
    return principal * (pool.apy / 100) * (projectionDays / 365);
  }, [pool.apy, pool.userStaked, projectionDays, stakeAmount]);

  return (
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <CardTitle>{pool.name}</CardTitle>
            <CardDescription>{pool.asset} staking pool</CardDescription>
          </div>
          <Badge>{pool.apy.toFixed(2)}% APY</Badge>
        </div>
      </CardHeader>
      <CardContent className="space-y-5">
        <div className="grid gap-3 sm:grid-cols-3">
          <Metric
            icon={Lock}
            label="Your stake"
            value={`${number.format(pool.userStaked)} ${pool.asset}`}
          />
          <Metric
            icon={Coins}
            label="Pending"
            value={`${number.format(pool.pendingRewards)} ${pool.rewardToken}`}
          />
          <Metric
            icon={CalendarClock}
            label="Lock"
            value={`${pool.lockPeriodDays} days`}
          />
        </div>

        <div className="grid gap-3 sm:grid-cols-[1fr_auto]">
          <div className="space-y-2">
            <Label htmlFor={`${pool.id}-stake`}>Stake amount</Label>
            <Input
              id={`${pool.id}-stake`}
              type="number"
              min="0"
              value={stakeAmount}
              onChange={(event) => setStakeAmount(event.target.value)}
            />
          </div>
          <div className="grid grid-cols-2 gap-2 self-end">
            <Button>
              <Lock className="h-4 w-4" />
              Stake
            </Button>
            <Button variant="outline">
              <Unlock className="h-4 w-4" />
              Unstake
            </Button>
          </div>
        </div>

        <div className="rounded-lg border p-3">
          <div className="flex items-center justify-between gap-3">
            <div className="flex items-center gap-2 text-sm font-medium">
              <LineChart className="h-4 w-4" />
              Projected earnings
            </div>
            <div className="flex items-center gap-2">
              <Input
                aria-label="Projection days"
                className="w-20"
                type="number"
                min="1"
                value={projectionDays}
                onChange={(event) =>
                  setProjectionDays(Math.max(1, Number(event.target.value) || 1))
                }
              />
              <span className="text-sm text-muted-foreground">days</span>
            </div>
          </div>
          <div className="mt-3 text-2xl font-semibold">
            {number.format(projectedRewards)} {pool.rewardToken}
          </div>
          <div className="text-xs text-muted-foreground">
            Based on current stake plus the draft stake amount.
          </div>
        </div>

        <div className="space-y-2">
          <div className="text-sm font-medium">Recent staking history</div>
          <div className="divide-y rounded-lg border">
            {pool.history.map((event) => (
              <div
                key={`${event.date}-${event.action}`}
                className="grid grid-cols-[1fr_auto] gap-3 p-3 text-sm"
              >
                <div>
                  <div className="font-medium">{event.action}</div>
                  <div className="text-xs text-muted-foreground">{event.date}</div>
                </div>
                <div className="font-medium">
                  {number.format(event.amount)} {pool.asset}
                </div>
              </div>
            ))}
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

function Metric({
  icon: Icon,
  label,
  value,
}: {
  icon: typeof Lock;
  label: string;
  value: string;
}) {
  return (
    <div className="rounded-lg bg-muted p-3">
      <div className="flex items-center gap-2 text-xs text-muted-foreground">
        <Icon className="h-4 w-4" />
        {label}
      </div>
      <div className="mt-2 font-semibold">{value}</div>
    </div>
  );
}
