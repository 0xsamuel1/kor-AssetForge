"use client";

import { useState, useEffect, useCallback } from "react";
import { Header } from "@/components/Header";
import { WalletConnect } from "@/components/WalletConnect";
import { BundleForm } from "@/components/bundles/BundleForm";
import { BundleList, BundleRecord } from "@/components/bundles/BundleList";
import { BundleAnalytics } from "@/components/bundles/BundleAnalytics";
import { StellarWallet } from "@/lib/stellar";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Package, PlusCircle, BarChart2 } from "lucide-react";
import { CardGridSkeleton } from "@/components/LoadingSkeleton";

export default function BundlesPage() {
  const [wallet, setWallet] = useState<StellarWallet | undefined>();
  const [bundles, setBundles] = useState<BundleRecord[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const api = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";

  const loadBundles = useCallback(async () => {
    setIsLoading(true);
    try {
      const res = await fetch(`${api}/api/bundles`);
      if (res.ok) setBundles(await res.json());
    } catch {
      // ignore
    } finally {
      setIsLoading(false);
    }
  }, [api]);

  useEffect(() => { loadBundles(); }, [loadBundles]);

  const myBundles = wallet
    ? bundles.filter((b) => b.creator === wallet.publicKey)
    : [];

  return (
    <div className="min-h-screen bg-background">
      <Header wallet={wallet} />
      <main className="container mx-auto px-4 py-8 max-w-5xl">
        <div className="flex items-center justify-between mb-8">
          <div>
            <h1 className="text-3xl font-bold">Asset Bundles</h1>
            <p className="text-muted-foreground mt-1">
              Create and trade portfolios of multiple assets as a single unit.
            </p>
          </div>
          {!wallet && (
            <WalletConnect
              onWalletConnected={(w) => setWallet(w)}
              onWalletDisconnected={() => setWallet(undefined)}
              wallet={wallet}
            />
          )}
        </div>

        <Tabs defaultValue="browse">
          <TabsList className="mb-6">
            <TabsTrigger value="browse" className="flex items-center gap-2">
              <Package className="h-4 w-4" />
              Browse Bundles
            </TabsTrigger>
            {wallet && (
              <>
                <TabsTrigger value="create" className="flex items-center gap-2">
                  <PlusCircle className="h-4 w-4" />
                  Create
                </TabsTrigger>
                <TabsTrigger value="analytics" className="flex items-center gap-2">
                  <BarChart2 className="h-4 w-4" />
                  My Analytics
                </TabsTrigger>
              </>
            )}
          </TabsList>

          <TabsContent value="browse">
            {isLoading ? (
              <CardGridSkeleton count={4} className="lg:grid-cols-2" label="Loading bundles" />
            ) : (
              <BundleList bundles={bundles} wallet={wallet} onUpdated={loadBundles} />
            )}
          </TabsContent>

          {wallet && (
            <>
              <TabsContent value="create">
                <div className="max-w-lg">
                  <BundleForm wallet={wallet} onCreated={loadBundles} />
                </div>
              </TabsContent>
              <TabsContent value="analytics">
                <BundleAnalytics bundles={myBundles} />
              </TabsContent>
            </>
          )}
        </Tabs>
      </main>
    </div>
  );
}