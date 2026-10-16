"use client";

import { useState } from "react";
import { StellarWallet } from "@/lib/stellar";
import { truncateAddress } from "@/lib/utils";
import { VerificationBadge } from "./VerificationBadge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { toast } from "@/lib/toast";
import { ShieldCheck, ShieldOff, ExternalLink } from "lucide-react";

export interface VerificationRequest {
  asset_id: number;
  requester: string;
  evidence_url: string;
  submitted_at: number;
  status: "Pending" | "Approved" | "Rejected";
  reviewed_by?: string;
  reviewed_at?: number;
  notes?: string;
}

interface VerifierDashboardProps {
  wallet: StellarWallet;
  requests: VerificationRequest[];
  onReviewed: () => void;
}

export function VerifierDashboard({ wallet, requests, onReviewed }: VerifierDashboardProps) {
  const [notes, setNotes] = useState<Record<number, string>>({});
  const [processing, setProcessing] = useState<number | null>(null);
  const api = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";

  const review = async (assetId: number, approve: boolean) => {
    setProcessing(assetId);
    try {
      const res = await fetch(`${api}/api/verification/review`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          verifier: wallet.publicKey,
          asset_id: assetId,
          approve,
          notes: notes[assetId] ?? "",
        }),
      });
      if (!res.ok) throw new Error(await res.text());
      toast.success(`Asset ${assetId} ${approve ? "approved" : "rejected"}.`);
      onReviewed();
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Review failed.");
    } finally {
      setProcessing(null);
    }
  };

  const pending = requests.filter((r) => r.status === "Pending");
  const reviewed = requests.filter((r) => r.status !== "Pending");

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-xl font-semibold mb-4">Pending Requests ({pending.length})</h2>
        {pending.length === 0 ? (
          <p className="text-muted-foreground text-sm">No pending requests.</p>
        ) : (
          <div className="space-y-4">
            {pending.map((req) => (
              <Card key={req.asset_id}>
                <CardHeader className="pb-2">
                  <CardTitle className="text-base flex items-center justify-between">
                    <span>Asset #{req.asset_id}</span>
                    <VerificationBadge status="pending" size="sm" />
                  </CardTitle>
                </CardHeader>
                <CardContent className="space-y-3">
                  <div className="text-sm text-muted-foreground">
                    Requested by{" "}
                    <span className="font-mono">{truncateAddress(req.requester)}</span>
                    {" · "}
                    {new Date(req.submitted_at * 1000).toLocaleDateString()}
                  </div>
                  <a
                    href={req.evidence_url}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="flex items-center gap-1 text-sm text-blue-600 hover:underline"
                  >
                    <ExternalLink className="h-3.5 w-3.5" />
                    View evidence
                  </a>
                  <div className="space-y-1">
                    <Label htmlFor={`notes-${req.asset_id}`} className="text-xs">
                      Review notes (optional)
                    </Label>
                    <Textarea
                      id={`notes-${req.asset_id}`}
                      rows={2}
                      placeholder="Notes for the requester..."
                      value={notes[req.asset_id] ?? ""}
                      onChange={(e) =>
                        setNotes((n) => ({ ...n, [req.asset_id]: e.target.value }))
                      }
                    />
                  </div>
                  <div className="flex gap-3">
                    <Button
                      size="sm"
                      className="flex-1 bg-green-600 hover:bg-green-700 text-white"
                      disabled={processing === req.asset_id}
                      onClick={() => review(req.asset_id, true)}
                    >
                      <ShieldCheck className="h-4 w-4 mr-1" />
                      Approve
                    </Button>
                    <Button
                      size="sm"
                      variant="destructive"
                      className="flex-1"
                      disabled={processing === req.asset_id}
                      onClick={() => review(req.asset_id, false)}
                    >
                      <ShieldOff className="h-4 w-4 mr-1" />
                      Reject
                    </Button>
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>
        )}
      </div>

      {reviewed.length > 0 && (
        <div>
          <h2 className="text-xl font-semibold mb-4">Reviewed ({reviewed.length})</h2>
          <div className="space-y-3">
            {reviewed.map((req) => (
              <Card key={req.asset_id} className="opacity-75">
                <CardContent className="pt-4 flex items-center justify-between">
                  <div>
                    <span className="font-medium">Asset #{req.asset_id}</span>
                    {req.notes && (
                      <p className="text-xs text-muted-foreground mt-0.5">{req.notes}</p>
                    )}
                  </div>
                  <VerificationBadge
                    status={req.status === "Approved" ? "verified" : "unverified"}
                    size="sm"
                  />
                </CardContent>
              </Card>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}