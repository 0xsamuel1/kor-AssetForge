"use client";

import { useState, useEffect } from "react";
import { WalletConnect } from "@/components/WalletConnect";
import { AssetGrid } from "@/components/AssetGrid";
import { Header } from "@/components/Header";
import { stellarService, StellarWallet } from "@/lib/stellar";
import { OnboardingOverlay } from "@/components/onboarding/OnboardingOverlay";
import { AssetInfo } from "@/lib/stellar";

export default function HomePage() {
  const [wallet, setWallet] = useState<StellarWallet | undefined>();
  const [assets, setAssets] = useState<AssetInfo[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [kycStatus, setKycStatus] = useState<string | undefined>();

  useEffect(() => {
    const loadAssets = async () => {
      try {
        const assetData = await stellarService.getAssets();
        setAssets(assetData);
      } catch (error) {
        console.error("Failed to load assets:", error);
      } finally {
        setIsLoading(false);
      }
    };

    loadAssets();
  }, []);

  useEffect(() => {
    if (wallet?.connected) {
      stellarService.getKYCStatus(wallet.publicKey)
        .then(setKycStatus)
        .catch(() => setKycStatus(undefined));
    }
  }, [wallet]);

  const handleWalletConnected = (connectedWallet: StellarWallet) => {
    setWallet(connectedWallet);
  };

  const handleWalletDisconnected = () => {
    setWallet(undefined);
    setKycStatus(undefined);
  };

  return (
    <div className="min-h-screen bg-background">
      <Header wallet={wallet} />

      <main id="main-content" className="container mx-auto px-4 py-8">
        <div className="text-center mb-12">
          <h1 className="text-4xl font-bold mb-4">kor-AssetForge</h1>
          <p className="text-xl text-muted-foreground mb-8">
            Decentralized marketplace for tokenizing and trading real-world
            assets on Stellar
          </p>

          {!wallet && (
            <div className="flex justify-center">
              <WalletConnect
                onWalletConnected={handleWalletConnected}
                onWalletDisconnected={handleWalletDisconnected}
                wallet={wallet}
              />
            </div>
          )}
        </div>

        {wallet && (
          <div className="mb-8">
            <div className="flex justify-between items-center mb-6">
              <h2 className="text-2xl font-semibold">Marketplace Assets</h2>
              <WalletConnect
                onWalletConnected={handleWalletConnected}
                onWalletDisconnected={handleWalletDisconnected}
                wallet={wallet}
              />
            </div>
          </div>
        )}

        <AssetGrid assets={assets} isLoading={isLoading} wallet={wallet} />
      </main>

      <OnboardingOverlay
        wallet={wallet}
        onWalletConnected={handleWalletConnected}
        kycStatus={kycStatus}
      />
    </div>
  );
}
