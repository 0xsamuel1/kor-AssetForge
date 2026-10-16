'use client'

import { useState, useEffect } from "react";
import { Header } from "@/components/Header";
import { Portfolio } from "@/components/Portfolio";
import { WalletConnect } from "@/components/WalletConnect";
import { StellarWallet, stellarService } from "@/lib/stellar";

export default function DashboardPage() {
  const [wallet, setWallet] = useState<StellarWallet | undefined>();

  useEffect(() => {
    const connect = async () => {
      try {
        const w = await stellarService.connectWallet();
        setWallet(w);
      } catch {}
    };
    connect();
  }, []);

  return (
    <div className="min-h-screen bg-background">
      <Header wallet={wallet} />

      <main className="container mx-auto px-4 py-8">
        <div className="mb-8">
          <h1 className="text-3xl font-bold mb-2">Portfolio Dashboard</h1>
          <p className="text-muted-foreground">
            Track your holdings, performance, and transaction history
          </p>
        </div>

        {!wallet ? (
          <div className="flex justify-center py-12">
            <WalletConnect
              onWalletConnected={setWallet}
              onWalletDisconnected={() => setWallet(undefined)}
              wallet={wallet}
            />
          </div>
        ) : (
          <Portfolio />
        )}
      </main>
    </div>
  );
}
