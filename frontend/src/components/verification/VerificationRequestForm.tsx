"use client";

import { useState } from "react";
import { StellarWallet } from "@/lib/stellar";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { toast } from "@/lib/toast";
import { ShieldCheck } from "lucide-react";
import { ButtonSpinner } from "@/components/LoadingSpinner";

interface VerificationRequestFormProps {
  wallet: StellarWallet;
  assetId?: number;
  onSubmitted: () => void;
}

export function VerificationRequestForm({
  wallet,
  assetId,
  onSubmitted,
}: VerificationRequestFormProps) {
  const [localAssetId, setLocalAssetId] = useState(assetId?.toString() ?? "");
  const [evidenceUrl, setEvidenceUrl] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!localAssetId || !evidenceUrl) {
      toast.error("Asset ID and evidence URL are required.");
      return;
    }
    setIsSubmitting(true);
    try {
      const res = await fetch(
        `${process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080"}/api/verification/request`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            requester: wallet.publicKey,
            asset_id: Number(localAssetId),
            evidence_url: evidenceUrl,
          }),
        }
      );
      if (!res.ok) throw new Error(await res.text());
      toast.success("Verification request submitted. A verifier will review it shortly.");
      setEvidenceUrl("");
      onSubmitted();
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Submission failed.");
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ShieldCheck className="h-5 w-5" />
          Request Asset Verification
        </CardTitle>
        <CardDescription>
          Provide a link to supporting documentation (e.g. legal title, appraisal report, audit
          certificate). A registered verifier will review and approve or reject your request.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="asset-id">Asset ID</Label>
            <Input
              id="asset-id"
              type="number"
              min="1"
              placeholder="e.g. 42"
              value={localAssetId}
              onChange={(e) => setLocalAssetId(e.target.value)}
              disabled={!!assetId}
              required
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="evidence">Evidence URL</Label>
            <Input
              id="evidence"
              type="url"
              placeholder="https://docs.example.com/asset-proof.pdf"
              value={evidenceUrl}
              onChange={(e) => setEvidenceUrl(e.target.value)}
              required
            />
            <p className="text-xs text-muted-foreground">
              Link to publicly accessible documentation proving asset authenticity and ownership.
            </p>
          </div>
          <Button type="submit" disabled={isSubmitting} className="w-full">
            {isSubmitting && <ButtonSpinner />}
            {isSubmitting ? "Submitting…" : "Submit Verification Request"}
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}