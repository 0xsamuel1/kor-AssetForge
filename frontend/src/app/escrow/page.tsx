"use client";

import { useState, useEffect, useCallback } from "react";
import { Header } from "@/components/Header";
import { WalletConnect } from "@/components/WalletConnect";
import { EscrowForm } from "@/components/escrow/EscrowForm";
import { EscrowList, EscrowRecord } from "@/components/escrow/EscrowList";
import { DisputePanel } from "@/components/escrow/DisputePanel";
import { StellarWallet } from "@/lib/stellar";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { PlusCircle, List, Gavel } from "lucide-react";
import { ListSkeleton } from "@/components/LoadingSkeleton";

export default function EscrowPage() {
  const [wallet, setWallet] = useState<StellarWallet | undefined>();
  const [escrows, setEscrows] = useState<EscrowRecord[]>([]);
  const [isAdmin, setIsAdmin] = useState(false);
  const [isLoading, setIsLoading] = useState(true);
  const api = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";

  const loadEscrows = useCallback(async () => {
    try {
      const url = wallet
        ? `${api}/api/escrow?participant=${wallet.publicKey}`
        : `${api}/api/escrow`;
      const res = await fetch(url);
      if (res.ok) setEscrows(await res.json());
    } catch {
      // ignore
    } finally {
      setIsLoading(false);
    }
  }, [wallet, api]);

  const checkAdmin = useCallback(async () => {
    if (!wallet) return;
    try {
      const res = await fetch(`${api}/api/escrow/is-admin/${wallet.publicKey}`);
      if (res.ok) setIsAdmin((await res.json()).is_admin);
    } catch {
      // ignore
    }
  }, [wallet, api]);

  useEffect(() => { loadEscrows(); }, [loadEscrows]);
  useEffect(() => { checkAdmin(); }, [checkAdmin]);

  const disputed = escrows.filter((e) => e.status === "Disputed");

  return (
    <div className="min-h-screen bg-background">
      <Header wallet={wallet} />
      <main className="container mx-auto px-4 py-8 max-w-4xl">
        <div className="flex items-center justify-between mb-8">
          <div>
            <h1 className="text-3xl font-bold">Escrow</h1>
            <p className="text-muted-foreground mt-1">
              Secure high-value trades by locking funds until you confirm receipt of the asset.
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

        <Tabs defaultValue="list">
          <TabsList className="mb-6">
            <TabsTrigger value="list" className="flex items-center gap-2">
              <List className="h-4 w-4" />
              My Escrows
            </TabsTrigger>
            {wallet && (
              <TabsTrigger value="create" className="flex items-center gap-2">
                <PlusCircle className="h-4 w-4" />
                New Escrow
              </TabsTrigger>
            )}
            {isAdmin && disputed.length > 0 && (
              <TabsTrigger value="disputes" className="flex items-center gap-2">
                <Gavel className="h-4 w-4" />
                Disputes ({disputed.length})
              </TabsTrigger>
            )}
          </TabsList>

          <TabsContent value="list">
            {isLoading ? (
              <ListSkeleton rows={3} label="Loading escrows" />
            ) : (
              <EscrowList escrows={escrows} wallet={wallet} onUpdated={loadEscrows} />
            )}
          </TabsContent>

          {wallet && (
            <TabsContent value="create">
              <div className="max-w-lg">
                <EscrowForm wallet={wallet} onCreated={loadEscrows} />
              </div>
            </TabsContent>
          )}

          {isAdmin && (
            <TabsContent value="disputes">
              <DisputePanel
                wallet={wallet!}
                disputes={disputed}
                onResolved={loadEscrows}
              />
            </TabsContent>
          )}
        </Tabs>
      </main>
    </div>
  );
}