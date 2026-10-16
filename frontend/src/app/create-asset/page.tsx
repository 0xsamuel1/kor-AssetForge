"use client";

import { useState } from "react";
import { AssetWizard } from "@/components/AssetWizard";
import { Header } from "@/components/Header";
import { WalletConnect } from "@/components/WalletConnect";
import type { StellarWallet } from "@/lib/stellar";

export default function CreateAssetPage() {
  const [wallet, setWallet] = useState<StellarWallet | undefined>();

  return (
    <div className="min-h-screen bg-background">
      <Header wallet={wallet} />

      <main className="container mx-auto max-w-6xl px-4 py-8">
        <div className="mb-8 border-b pb-6">
          <div>
            <h1 className="text-3xl font-bold">Asset Tokenization</h1>
            <p className="mt-2 max-w-2xl text-muted-foreground">
              Guide a real-world asset from draft metadata to transaction-ready token issuance.
            </p>
          </div>
        </div>

        {wallet ? (
          <AssetWizard wallet={wallet} />
        ) : (
          <div className="mx-auto max-w-md">
            <WalletConnect
              wallet={wallet}
              onWalletConnected={setWallet}
              onWalletDisconnected={() => setWallet(undefined)}
            />
          </div>
        )}
      </main>
    </div>
  );
}
