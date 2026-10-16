"use client";

import { useState } from "react";
import { StellarWallet } from "@/lib/stellar";
import { truncateAddress } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { toast } from "@/lib/toast";
import { Users, UserMinus } from "lucide-react";
import { ButtonSpinner } from "@/components/LoadingSpinner";

interface DelegatePanelProps {
  wallet: StellarWallet;
  currentDelegate?: string;
  delegatedPower: number;
  delegators: string[];
  onUpdated: () => void;
}

export function DelegatePanel({
  wallet,
  currentDelegate,
  delegatedPower,
  delegators,
  onUpdated,
}: DelegatePanelProps) {
  const [delegatee, setDelegatee] = useState("");
  const [isLoading, setIsLoading] = useState(false);
  const api = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";

  const delegate = async () => {
    if (!delegatee.trim()) {
      toast.error("Enter a delegatee address.");
      return;
    }
    setIsLoading(true);
    try {
      const res = await fetch(`${api}/api/governance/delegate`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ delegator: wallet.publicKey, delegatee: delegatee.trim() }),
      });
      if (!res.ok) throw new Error(await res.text());
      toast.success(`Voting power delegated to ${truncateAddress(delegatee)}`);
      setDelegatee("");
      onUpdated();
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Delegation failed.");
    } finally {
      setIsLoading(false);
    }
  };

  const revoke = async () => {
    setIsLoading(true);
    try {
      const res = await fetch(`${api}/api/governance/delegate/revoke`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ delegator: wallet.publicKey }),
      });
      if (!res.ok) throw new Error(await res.text());
      toast.success("Delegation revoked.");
      onUpdated();
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Revoke failed.");
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Users className="h-5 w-5" />
          Voting Delegation
        </CardTitle>
        <CardDescription>
          Delegate your voting power to another address, or revoke an existing delegation.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-5">
        {currentDelegate ? (
          <div className="rounded-md bg-muted p-3 flex items-center justify-between">
            <div>
              <p className="text-xs text-muted-foreground">Currently delegating to</p>
              <p className="font-mono text-sm font-medium">{truncateAddress(currentDelegate)}</p>
            </div>
            <Button variant="destructive" size="sm" onClick={revoke} disabled={isLoading}>
              <UserMinus className="h-4 w-4 mr-1" />
              Revoke
            </Button>
          </div>
        ) : (
          <div className="space-y-3">
            <div className="space-y-2">
              <Label htmlFor="delegatee">Delegate To (address)</Label>
              <Input
                id="delegatee"
                placeholder="GABC...XYZ"
                value={delegatee}
                onChange={(e) => setDelegatee(e.target.value)}
              />
            </div>
            <Button onClick={delegate} disabled={isLoading} className="w-full">
              {isLoading && <ButtonSpinner />}
              {isLoading ? "Delegating…" : "Delegate Voting Power"}
            </Button>
          </div>
        )}

        {delegatedPower > 0 && (
          <div className="rounded-md bg-muted p-3">
            <p className="text-xs text-muted-foreground">Voting power delegated to you</p>
            <p className="text-lg font-bold">{delegatedPower.toLocaleString()}</p>
            <p className="text-xs text-muted-foreground">{delegators.length} delegator(s)</p>
          </div>
        )}
      </CardContent>
    </Card>
  );
}