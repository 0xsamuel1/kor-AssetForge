"use client";

import { useState } from "react";
import { StellarWallet } from "@/lib/stellar";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { toast } from "@/lib/toast";
import { Package, Plus, X } from "lucide-react";
import { ButtonSpinner } from "@/components/LoadingSpinner";

interface BundleFormProps {
  wallet: StellarWallet;
  onCreated: () => void;
}

export function BundleForm({ wallet, onCreated }: BundleFormProps) {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [assetIds, setAssetIds] = useState<string[]>([""]);
  const [isSubmitting, setIsSubmitting] = useState(false);

  const addRow = () => setAssetIds((ids) => [...ids, ""]);
  const removeRow = (i: number) => setAssetIds((ids) => ids.filter((_, idx) => idx !== i));
  const updateRow = (i: number, val: string) =>
    setAssetIds((ids) => ids.map((id, idx) => (idx === i ? val : id)));

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const parsed = assetIds.map(Number).filter((n) => n > 0);
    if (parsed.length === 0) {
      toast.error("Add at least one valid asset ID.");
      return;
    }
    setIsSubmitting(true);
    try {
      const res = await fetch(
        `${process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080"}/api/bundles`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            creator: wallet.publicKey,
            name,
            description,
            asset_ids: parsed,
          }),
        }
      );
      if (!res.ok) throw new Error(await res.text());
      toast.success("Bundle created successfully.");
      setName(""); setDescription(""); setAssetIds([""]);
      onCreated();
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Failed to create bundle.");
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Package className="h-5 w-5" />
          Create Asset Bundle
        </CardTitle>
        <CardDescription>
          Group multiple assets into a single tradeable unit. Bundles can be listed and sold as a portfolio.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="bundle-name">Bundle Name</Label>
            <Input id="bundle-name" placeholder="e.g. Real Estate Portfolio Q1"
              value={name} onChange={(e) => setName(e.target.value)} required />
          </div>
          <div className="space-y-2">
            <Label htmlFor="bundle-desc">Description</Label>
            <Textarea id="bundle-desc" rows={3} placeholder="Describe this portfolio..."
              value={description} onChange={(e) => setDescription(e.target.value)} />
          </div>
          <div className="space-y-2">
            <Label>Asset IDs</Label>
            <div className="space-y-2">
              {assetIds.map((id, i) => (
                <div key={i} className="flex gap-2">
                  <Input
                    type="number"
                    min="1"
                    placeholder={`Asset ID ${i + 1}`}
                    value={id}
                    onChange={(e) => updateRow(i, e.target.value)}
                    required
                  />
                  {assetIds.length > 1 && (
                    <Button type="button" variant="ghost" size="icon"
                      onClick={() => removeRow(i)}>
                      <X className="h-4 w-4" />
                    </Button>
                  )}
                </div>
              ))}
              <Button type="button" variant="outline" size="sm" onClick={addRow}
                className="w-full">
                <Plus className="h-4 w-4 mr-1" />
                Add Asset
              </Button>
            </div>
          </div>
          <Button type="submit" disabled={isSubmitting} className="w-full">
            {isSubmitting && <ButtonSpinner />}
            {isSubmitting ? "Creating…" : "Create Bundle"}
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}