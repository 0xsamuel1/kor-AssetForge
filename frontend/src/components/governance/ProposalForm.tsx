"use client";

import { useState } from "react";
import { StellarWallet } from "@/lib/stellar";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { toast } from "@/lib/toast";
import { FilePlus } from "lucide-react";
import { ButtonSpinner } from "@/components/LoadingSpinner";

interface ProposalFormProps {
  wallet: StellarWallet;
  onCreated: () => void;
}

export function ProposalForm({ wallet, onCreated }: ProposalFormProps) {
  const [assetId, setAssetId] = useState("");
  const [description, setDescription] = useState("");
  const [durationHours, setDurationHours] = useState("72");
  const [isSubmitting, setIsSubmitting] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!assetId || !description) {
      toast.error("All fields are required.");
      return;
    }
    setIsSubmitting(true);
    try {
      const res = await fetch(
        `${process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080"}/api/governance/proposals`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            proposer: wallet.publicKey,
            asset_id: Number(assetId),
            description,
            duration_seconds: Number(durationHours) * 3600,
          }),
        }
      );
      if (!res.ok) throw new Error(await res.text());
      toast.success("Proposal created successfully.");
      setAssetId("");
      setDescription("");
      setDurationHours("72");
      onCreated();
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Failed to create proposal.");
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <FilePlus className="h-5 w-5" />
          Create Proposal
        </CardTitle>
        <CardDescription>
          Submit a governance proposal for an asset. A deposit is required to prevent spam.
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
              value={assetId}
              onChange={(e) => setAssetId(e.target.value)}
              required
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="description">Description</Label>
            <Textarea
              id="description"
              placeholder="Describe what this proposal aims to achieve..."
              rows={4}
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              required
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="duration">Voting Duration (hours)</Label>
            <Input
              id="duration"
              type="number"
              min="1"
              max="720"
              value={durationHours}
              onChange={(e) => setDurationHours(e.target.value)}
            />
          </div>
          <Button type="submit" disabled={isSubmitting} className="w-full">
            {isSubmitting && <ButtonSpinner />}
            {isSubmitting ? "Submitting…" : "Submit Proposal"}
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}