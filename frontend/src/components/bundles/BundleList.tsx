"use client";

import { useState } from "react";
import { StellarWallet } from "@/lib/stellar";
import { truncateAddress } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardFooter, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { toast } from "@/lib/toast";
import { Package, Tag, ShoppingCart } from "lucide-react";

export interface BundleRecord {
  bundle_id: number;
  name: string;
  description: string;
  creator: string;
  asset_ids: number[];
  list_price: number;
  status: "Active" | "Listed" | "Sold";
  created_at: number;
  buyer?: string;
}

interface BundleListProps {
  bundles: BundleRecord[];
  wallet?: StellarWallet;
  onUpdated: () => void;
}

const statusVariant: Record<BundleRecord["status"], "default" | "secondary" | "destructive"> = {
  Active: "secondary",
  Listed: "default",
  Sold: "destructive",
};

export function BundleList({ bundles, wallet, onUpdated }: BundleListProps) {
  const [prices, setPrices] = useState<Record<number, string>>({});
  const [processing, setProcessing] = useState<number | null>(null);
  const [tokenAddress, setTokenAddress] = useState("");
  const api = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";

  const listBundle = async (bundleId: number) => {
    const price = Number(prices[bundleId]);
    if (!price || price <= 0) { toast.error("Enter a valid price."); return; }
    setProcessing(bundleId);
    try {
      const res = await fetch(`${api}/api/bundles/${bundleId}/list`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ creator: wallet!.publicKey, price }),
      });
      if (!res.ok) throw new Error(await res.text());
      toast.success("Bundle listed for sale.");
      onUpdated();
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Listing failed.");
    } finally { setProcessing(null); }
  };

  const buyBundle = async (bundleId: number) => {
    if (!tokenAddress) { toast.error("Enter the token contract address."); return; }
    setProcessing(bundleId);
    try {
      const res = await fetch(`${api}/api/bundles/${bundleId}/buy`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ buyer: wallet!.publicKey, token: tokenAddress }),
      });
      if (!res.ok) throw new Error(await res.text());
      toast.success("Bundle purchased!");
      onUpdated();
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Purchase failed.");
    } finally { setProcessing(null); }
  };

  if (bundles.length === 0) {
    return <div className="text-center py-16 text-muted-foreground">No bundles found.</div>;
  }

  return (
    <div className="space-y-4">
      {bundles.map((b) => {
        const isOwner = wallet?.publicKey === b.creator;
        return (
          <Card key={b.bundle_id}>
            <CardHeader className="pb-2">
              <div className="flex items-start justify-between gap-4">
                <div className="flex-1 min-w-0">
                  <CardTitle className="text-base flex items-center gap-2">
                    <Package className="h-4 w-4 shrink-0" />
                    {b.name}
                  </CardTitle>
                  {b.description && (
                    <p className="text-sm text-muted-foreground mt-1 line-clamp-2">
                      {b.description}
                    </p>
                  )}
                </div>
                <Badge variant={statusVariant[b.status]}>{b.status}</Badge>
              </div>
            </CardHeader>
            <CardContent className="space-y-3">
              <div className="flex flex-wrap gap-1">
                {b.asset_ids.map((id) => (
                  <span key={id} className="text-xs bg-muted rounded px-2 py-0.5 font-mono">
                    #{id}
                  </span>
                ))}
              </div>
              <div className="flex justify-between text-sm text-muted-foreground">
                <span>{b.asset_ids.length} asset(s)</span>
                <span>By {truncateAddress(b.creator)}</span>
              </div>
              {b.status === "Listed" && (
                <div className="font-medium text-lg">
                  {b.list_price.toLocaleString()} stroops
                </div>
              )}
            </CardContent>
            <CardFooter className="flex flex-col gap-3">
              {isOwner && b.status === "Active" && (
                <div className="flex gap-2 w-full">
                  <Input
                    type="number"
                    min="1"
                    placeholder="List price (stroops)"
                    value={prices[b.bundle_id] ?? ""}
                    onChange={(e) =>
                      setPrices((p) => ({ ...p, [b.bundle_id]: e.target.value }))
                    }
                  />
                  <Button
                    size="sm"
                    disabled={processing === b.bundle_id}
                    onClick={() => listBundle(b.bundle_id)}
                  >
                    <Tag className="h-4 w-4 mr-1" />
                    List
                  </Button>
                </div>
              )}
              {!isOwner && b.status === "Listed" && wallet && (
                <div className="flex gap-2 w-full">
                  <Input
                    placeholder="Token contract address"
                    value={tokenAddress}
                    onChange={(e) => setTokenAddress(e.target.value)}
                  />
                  <Button
                    size="sm"
                    disabled={processing === b.bundle_id}
                    onClick={() => buyBundle(b.bundle_id)}
                  >
                    <ShoppingCart className="h-4 w-4 mr-1" />
                    Buy
                  </Button>
                </div>
              )}
            </CardFooter>
          </Card>
        );
      })}
    </div>
  );
}