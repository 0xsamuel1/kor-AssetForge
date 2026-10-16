"use client";

import { useMemo, useState } from "react";
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { LiquidityPool } from "@/components/PoolStats";
import { ArrowDownUp, Plus, Trash2 } from "lucide-react";

interface AddLiquidityProps {
  pools: LiquidityPool[];
  selectedPoolId: string;
  onPoolChange: (poolId: string) => void;
}

export function AddLiquidity({
  pools,
  selectedPoolId,
  onPoolChange,
}: AddLiquidityProps) {
  const [mode, setMode] = useState<"add" | "remove">("add");
  const [tokenAAmount, setTokenAAmount] = useState("1000");
  const [slippage, setSlippage] = useState("0.5");
  const [removePercent, setRemovePercent] = useState(25);

  const selectedPool = pools.find((pool) => pool.id === selectedPoolId) ?? pools[0];
  const tokenA = Number(tokenAAmount) || 0;

  const quote = useMemo(() => {
    const poolRatio = selectedPool.reserveB / selectedPool.reserveA;
    const tokenB = tokenA * poolRatio;
    const depositValue = tokenA + tokenB;
    const mintedShare =
      depositValue / (selectedPool.reserveA + selectedPool.reserveB + depositValue);

    return {
      tokenB,
      lpTokens: selectedPool.lpTokenSupply * mintedShare,
      sharePercent: mintedShare * 100,
    };
  }, [selectedPool, tokenA]);

  const removeQuote = useMemo(() => {
    const lpTokens = selectedPool.userLpTokens * (removePercent / 100);
    const ownership = lpTokens / selectedPool.lpTokenSupply;

    return {
      lpTokens,
      tokenA: selectedPool.reserveA * ownership,
      tokenB: selectedPool.reserveB * ownership,
    };
  }, [removePercent, selectedPool]);

  return (
    <Card>
      <CardHeader>
        <CardTitle>Manage liquidity</CardTitle>
        <CardDescription>
          Add balanced deposits or remove liquidity from your LP position.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-5">
        <div className="grid gap-3 sm:grid-cols-[1fr_auto]">
          <div className="space-y-2">
            <Label htmlFor="pool">Pool</Label>
            <Select value={selectedPool.id} onValueChange={onPoolChange}>
              <SelectTrigger id="pool" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {pools.map((pool) => (
                  <SelectItem key={pool.id} value={pool.id}>
                    {pool.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="grid grid-cols-2 gap-2 self-end">
            <Button
              type="button"
              variant={mode === "add" ? "default" : "outline"}
              onClick={() => setMode("add")}
            >
              <Plus className="h-4 w-4" />
              Add
            </Button>
            <Button
              type="button"
              variant={mode === "remove" ? "default" : "outline"}
              onClick={() => setMode("remove")}
            >
              <Trash2 className="h-4 w-4" />
              Remove
            </Button>
          </div>
        </div>

        {mode === "add" ? (
          <div className="space-y-4">
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="token-a">{selectedPool.tokenA} amount</Label>
                <Input
                  id="token-a"
                  type="number"
                  min="0"
                  value={tokenAAmount}
                  onChange={(event) => setTokenAAmount(event.target.value)}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="token-b">{selectedPool.tokenB} required</Label>
                <Input
                  id="token-b"
                  readOnly
                  value={quote.tokenB.toFixed(2)}
                />
              </div>
            </div>

            <div className="space-y-2">
              <Label htmlFor="slippage">Max slippage</Label>
              <Input
                id="slippage"
                type="number"
                min="0.1"
                step="0.1"
                value={slippage}
                onChange={(event) => setSlippage(event.target.value)}
              />
            </div>

            <div className="rounded-lg border p-3 text-sm">
              <div className="flex items-center gap-2 font-medium">
                <ArrowDownUp className="h-4 w-4" />
                Deposit preview
              </div>
              <div className="mt-3 grid gap-2 sm:grid-cols-3">
                <Preview label="LP tokens" value={quote.lpTokens.toFixed(2)} />
                <Preview
                  label="Pool share"
                  value={`${quote.sharePercent.toFixed(3)}%`}
                />
                <Preview label="Slippage" value={`${slippage || "0"}%`} />
              </div>
            </div>

            <Button className="w-full">Add liquidity</Button>
          </div>
        ) : (
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="remove-percent">Remove percentage</Label>
              <Input
                id="remove-percent"
                type="number"
                min="1"
                max="100"
                value={removePercent}
                onChange={(event) =>
                  setRemovePercent(Math.min(100, Number(event.target.value) || 0))
                }
              />
            </div>
            <div className="rounded-lg border p-3 text-sm">
              <div className="font-medium">Withdrawal preview</div>
              <div className="mt-3 grid gap-2 sm:grid-cols-3">
                <Preview
                  label="LP burned"
                  value={removeQuote.lpTokens.toFixed(2)}
                />
                <Preview
                  label={selectedPool.tokenA}
                  value={removeQuote.tokenA.toFixed(2)}
                />
                <Preview
                  label={selectedPool.tokenB}
                  value={removeQuote.tokenB.toFixed(2)}
                />
              </div>
            </div>
            <Button className="w-full" variant="destructive">
              Remove liquidity
            </Button>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function Preview({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="mt-1 font-semibold">{value}</div>
    </div>
  );
}
