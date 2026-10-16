"use client";

import { useState } from "react";
import { StellarWallet } from "@/lib/stellar";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { toast } from "@/lib/toast";
import { Lock } from "lucide-react";
import { ButtonSpinner } from "@/components/LoadingSpinner";

interface EscrowFormProps {
  wallet: StellarWallet;
  onCreated: () => void;
}

export function EscrowForm({ wallet, onCreated }: EscrowFormProps) {
  const [seller, setSeller] = useState("");
  const [tokenAddress, setTokenAddress] = useState("");
  const [amount, setAmount] = useState("");
  const [assetId, setAssetId] = useState("");
  const [deadlineHours, setDeadlineHours] = useState("72");
  const [isSubmitting, setIsSubmitting] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsSubmitting(true);
    try {
      const releaseDeadline =
        Math.floor(Date.now() / 1000) + Number(deadlineHours) * 3600;
      const res = await fetch(
        `${process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080"}/api/escrow`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            buyer: wallet.publicKey,
            seller,
            token: tokenAddress,
            amount: Number(amount),
            asset_id: Number(assetId),
            release_deadline: releaseDeadline,
          }),
        }
      );
      if (!res.ok) throw new Error(await res.text());
      toast.success("Escrow created. Funds are held securely until you confirm receipt.");
      setSeller(""); setTokenAddress(""); setAmount(""); setAssetId("");
      setDeadlineHours("72");
      onCreated();
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Failed to create escrow.");
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Lock className="h-5 w-5" />
          Create Escrow
        </CardTitle>
        <CardDescription>
          Lock funds in the escrow contract. The seller receives payment only after you confirm
          receipt of the asset, or automatically after the release deadline.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="seller">Seller Address</Label>
            <Input id="seller" placeholder="G…" value={seller}
              onChange={(e) => setSeller(e.target.value)} required />
          </div>
          <div className="space-y-2">
            <Label htmlFor="token">Token Contract Address</Label>
            <Input id="token" placeholder="C…" value={tokenAddress}
              onChange={(e) => setTokenAddress(e.target.value)} required />
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="amount">Amount (stroops)</Label>
              <Input id="amount" type="number" min="1" value={amount}
                onChange={(e) => setAmount(e.target.value)} required />
            </div>
            <div className="space-y-2">
              <Label htmlFor="asset-id">Asset ID</Label>
              <Input id="asset-id" type="number" min="1" value={assetId}
                onChange={(e) => setAssetId(e.target.value)} required />
            </div>
          </div>
          <div className="space-y-2">
            <Label htmlFor="deadline">Release Deadline (hours from now)</Label>
            <Input id="deadline" type="number" min="1" max="8760" value={deadlineHours}
              onChange={(e) => setDeadlineHours(e.target.value)} />
            <p className="text-xs text-muted-foreground">
              If you raise no dispute before this deadline, the seller can claim funds automatically.
            </p>
          </div>
          <Button type="submit" disabled={isSubmitting} className="w-full">
            {isSubmitting && <ButtonSpinner />}
            {isSubmitting ? "Creating…" : "Create Escrow & Lock Funds"}
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}