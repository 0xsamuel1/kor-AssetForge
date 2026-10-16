"use client";

import { useState } from "react";
import { StellarWallet } from "@/lib/stellar";
import { truncateAddress } from "@/lib/utils";
import { EscrowRecord } from "./EscrowList";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { toast } from "@/lib/toast";
import { Gavel } from "lucide-react";

interface DisputePanelProps {
  wallet: StellarWallet;
  disputes: EscrowRecord[];
  onResolved: () => void;
}

export function DisputePanel({ wallet, disputes, onResolved }: DisputePanelProps) {
  const [processing, setProcessing] = useState<number | null>(null);
  const api = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";

  const resolve = async (escrowId: number, releaseToSeller: boolean) => {
    setProcessing(escrowId);
    try {
      const res = await fetch(`${api}/api/escrow/${escrowId}/resolve`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          admin: wallet.publicKey,
          release_to_seller: releaseToSeller,
        }),
      });
      if (!res.ok) throw new Error(await res.text());
      toast.success(
        releaseToSeller
          ? `Escrow #${escrowId}: funds released to seller.`
          : `Escrow #${escrowId}: funds refunded to buyer.`
      );
      onResolved();
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Resolution failed.");
    } finally {
      setProcessing(null);
    }
  };

  if (disputes.length === 0) {
    return (
      <div className="text-center py-16 text-muted-foreground">
        No disputed escrows to review.
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {disputes.map((d) => (
        <Card key={d.escrow_id} className="border-destructive/40">
          <CardHeader className="pb-2">
            <div className="flex items-center justify-between">
              <CardTitle className="text-base">
                Dispute — Escrow #{d.escrow_id}
              </CardTitle>
              <Badge variant="destructive">Disputed</Badge>
            </div>
            <CardDescription>
              Asset #{d.asset_id} · {d.amount.toLocaleString()} stroops
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="grid grid-cols-2 gap-2 text-sm">
              <div>
                <span className="text-muted-foreground">Buyer </span>
                <span className="font-mono">{truncateAddress(d.buyer)}</span>
              </div>
              <div>
                <span className="text-muted-foreground">Seller </span>
                <span className="font-mono">{truncateAddress(d.seller)}</span>
              </div>
            </div>
            {d.dispute_notes && (
              <div className="rounded-md bg-destructive/10 p-3 text-sm">
                <p className="font-medium text-destructive mb-1">Buyer&apos;s complaint:</p>
                <p>{d.dispute_notes}</p>
              </div>
            )}
            <div className="flex gap-3">
              <Button
                size="sm"
                className="flex-1"
                disabled={processing === d.escrow_id}
                onClick={() => resolve(d.escrow_id, true)}
              >
                <Gavel className="h-4 w-4 mr-1" />
                Release to Seller
              </Button>
              <Button
                size="sm"
                variant="outline"
                className="flex-1"
                disabled={processing === d.escrow_id}
                onClick={() => resolve(d.escrow_id, false)}
              >
                <Gavel className="h-4 w-4 mr-1" />
                Refund Buyer
              </Button>
            </div>
          </CardContent>
        </Card>
      ))}
    </div>
  );
}