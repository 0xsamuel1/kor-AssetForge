"use client";

import { useState } from "react";
import { StellarWallet } from "@/lib/stellar";
import { truncateAddress } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Textarea } from "@/components/ui/textarea";
import { toast } from "@/lib/toast";
import { CheckCircle, AlertTriangle, Clock } from "lucide-react";

export interface EscrowRecord {
  escrow_id: number;
  buyer: string;
  seller: string;
  token: string;
  amount: number;
  asset_id: number;
  release_deadline: number;
  status: "Active" | "Released" | "Refunded" | "Disputed" | "Resolved";
  dispute_notes?: string;
  created_at: number;
}

interface EscrowListProps {
  escrows: EscrowRecord[];
  wallet?: StellarWallet;
  onUpdated: () => void;
}

const statusConfig: Record<
  EscrowRecord["status"],
  { label: string; variant: "default" | "secondary" | "destructive" }
> = {
  Active: { label: "Active", variant: "default" },
  Released: { label: "Released", variant: "secondary" },
  Refunded: { label: "Refunded", variant: "secondary" },
  Disputed: { label: "Disputed", variant: "destructive" },
  Resolved: { label: "Resolved", variant: "secondary" },
};

export function EscrowList({ escrows, wallet, onUpdated }: EscrowListProps) {
  const [disputeNotes, setDisputeNotes] = useState<Record<number, string>>({});
  const [processing, setProcessing] = useState<number | null>(null);
  const api = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";

  const post = async (path: string, body: object) => {
    const res = await fetch(`${api}${path}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    if (!res.ok) throw new Error(await res.text());
  };

  const release = async (id: number) => {
    setProcessing(id);
    try {
      await post(`/api/escrow/${id}/release`, { buyer: wallet!.publicKey });
      toast.success("Funds released to seller.");
      onUpdated();
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Release failed.");
    } finally {
      setProcessing(null);
    }
  };

  const dispute = async (id: number) => {
    setProcessing(id);
    try {
      await post(`/api/escrow/${id}/dispute`, {
        buyer: wallet!.publicKey,
        notes: disputeNotes[id] ?? "",
      });
      toast.success("Dispute raised. The arbiter will review.");
      onUpdated();
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Dispute failed.");
    } finally {
      setProcessing(null);
    }
  };

  if (escrows.length === 0) {
    return (
      <div className="text-center py-16 text-muted-foreground">No escrows found.</div>
    );
  }

  return (
    <div className="space-y-4">
      {escrows.map((e) => {
        const isBuyer = wallet?.publicKey === e.buyer;
        const deadline = new Date(e.release_deadline * 1000);
        const timeLeft = Math.max(0, e.release_deadline - Math.floor(Date.now() / 1000));
        const cfg = statusConfig[e.status];

        return (
          <Card key={e.escrow_id}>
            <CardHeader className="pb-2">
              <div className="flex items-center justify-between">
                <CardTitle className="text-base">
                  Escrow #{e.escrow_id} — Asset {e.asset_id}
                </CardTitle>
                <Badge variant={cfg.variant}>{cfg.label}</Badge>
              </div>
            </CardHeader>
            <CardContent className="space-y-3">
              <div className="grid grid-cols-2 gap-2 text-sm">
                <div>
                  <span className="text-muted-foreground">Buyer </span>
                  <span className="font-mono">{truncateAddress(e.buyer)}</span>
                </div>
                <div>
                  <span className="text-muted-foreground">Seller </span>
                  <span className="font-mono">{truncateAddress(e.seller)}</span>
                </div>
                <div>
                  <span className="text-muted-foreground">Amount </span>
                  <span className="font-medium">{e.amount.toLocaleString()} stroops</span>
                </div>
                <div className="flex items-center gap-1 text-muted-foreground">
                  <Clock className="h-3.5 w-3.5" />
                  {e.status === "Active"
                    ? `${Math.floor(timeLeft / 3600)}h ${Math.floor((timeLeft % 3600) / 60)}m left`
                    : deadline.toLocaleDateString()}
                </div>
              </div>

              {e.dispute_notes && (
                <div className="rounded-md bg-destructive/10 p-2 text-sm text-destructive">
                  {e.dispute_notes}
                </div>
              )}

              {e.status === "Active" && isBuyer && (
                <div className="space-y-2">
                  <Textarea
                    rows={2}
                    placeholder="Dispute reason (optional)..."
                    value={disputeNotes[e.escrow_id] ?? ""}
                    onChange={(v) =>
                      setDisputeNotes((n) => ({ ...n, [e.escrow_id]: v.target.value }))
                    }
                  />
                  <div className="flex gap-3">
                    <Button
                      size="sm"
                      className="flex-1 bg-green-600 hover:bg-green-700 text-white"
                      disabled={processing === e.escrow_id}
                      onClick={() => release(e.escrow_id)}
                    >
                      <CheckCircle className="h-4 w-4 mr-1" />
                      Confirm Receipt
                    </Button>
                    <Button
                      size="sm"
                      variant="destructive"
                      className="flex-1"
                      disabled={processing === e.escrow_id}
                      onClick={() => dispute(e.escrow_id)}
                    >
                      <AlertTriangle className="h-4 w-4 mr-1" />
                      Raise Dispute
                    </Button>
                  </div>
                </div>
              )}
            </CardContent>
          </Card>
        );
      })}
    </div>
  );
}