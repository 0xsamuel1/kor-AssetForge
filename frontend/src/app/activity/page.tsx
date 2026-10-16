"use client";

import { useState } from "react";

import { ActivityFeed } from "@/components/ActivityFeed";
import { Header } from "@/components/Header";
import type { StellarWallet } from "@/lib/stellar";

export default function ActivityPage() {
  const [wallet, setWallet] = useState<StellarWallet | undefined>();

  return (
    <div className="min-h-screen bg-background">
      <Header
        wallet={wallet}
        onWalletConnected={setWallet}
        onWalletDisconnected={() => setWallet(undefined)}
      />

      <main id="main-content" className="container mx-auto max-w-3xl px-4 py-8">
        <div className="mb-6">
          <h1 className="text-3xl font-bold">Activity</h1>
          <p className="mt-1 text-muted-foreground">
            Live trades, listings, proposals and verifications from across the
            platform.
          </p>
        </div>

        <ActivityFeed />
      </main>
    </div>
  );
}
