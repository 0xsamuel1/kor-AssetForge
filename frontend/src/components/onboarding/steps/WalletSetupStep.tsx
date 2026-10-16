"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { ArrowLeft, ArrowRight, ExternalLink, AlertCircle, CheckCircle, Wallet } from "lucide-react";
import { stellarService, StellarWallet } from "@/lib/stellar";
import { ButtonSpinner } from "@/components/LoadingSpinner";

interface WalletSetupStepProps {
  onNext: () => void
  onPrev: () => void
  wallet?: StellarWallet
  onWalletConnected: (wallet: StellarWallet) => void
}

export function WalletSetupStep({ onNext, onPrev, wallet, onWalletConnected }: WalletSetupStepProps) {
  const [isConnecting, setIsConnecting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleConnect = async () => {
    setIsConnecting(true);
    setError(null);
    try {
      const connected = await stellarService.connectWallet();
      onWalletConnected(connected);
    } catch {
      setError("Could not connect to Freighter wallet. Make sure Freighter extension is installed.");
    } finally {
      setIsConnecting(false);
    }
  };

  return (
    <div className="flex flex-col space-y-6 py-4">
      <div className="text-center space-y-2">
        <h2 className="text-2xl font-bold">Wallet Setup</h2>
        <p className="text-muted-foreground max-w-md mx-auto">
          Connect your Stellar wallet to interact with the platform
        </p>
      </div>

      <div className="rounded-lg border p-4 space-y-3">
        <div className="flex items-start gap-3">
          <div className="rounded-full bg-primary/10 p-2 mt-0.5">
            <Wallet className="h-5 w-5 text-primary" />
          </div>
          <div className="space-y-1">
            <p className="font-medium text-sm">Freighter Wallet</p>
            <p className="text-xs text-muted-foreground">
              kor-AssetForge uses Freighter, the official Stellar browser extension wallet.
              If you don't have it installed, you can get it from the Chrome Web Store.
            </p>
          </div>
        </div>

        <a
          href="https://chromewebstore.google.com/detail/freighter/bcacfldlkkdogcmkkibnjlakofdplcbk"
          target="_blank"
          rel="noopener noreferrer"
          className="inline-flex items-center gap-1 text-xs text-primary hover:underline"
        >
          <ExternalLink className="h-3 w-3" />
          Install Freighter
        </a>
      </div>

      {error && (
        <div className="flex items-start gap-2 rounded-lg bg-destructive/10 p-3 text-destructive text-sm">
          <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
          <span>{error}</span>
        </div>
      )}

      {wallet?.connected ? (
        <div className="flex items-center gap-2 rounded-lg bg-green-50 dark:bg-green-950/30 p-3 text-green-700 dark:text-green-400 text-sm">
          <CheckCircle className="h-4 w-4 shrink-0" />
          <span>Wallet connected: {wallet.publicKey.slice(0, 6)}...{wallet.publicKey.slice(-4)}</span>
        </div>
      ) : (
        <Button onClick={handleConnect} disabled={isConnecting} className="w-full">
          {isConnecting && <ButtonSpinner />}
          {isConnecting ? "Connecting…" : "Connect Freighter Wallet"}
        </Button>
      )}

      <div className="rounded-lg bg-muted p-3 space-y-1">
        <p className="text-xs font-medium">Security Tips:</p>
        <ul className="text-xs text-muted-foreground space-y-1">
          <li>• Never share your secret recovery phrase with anyone</li>
          <li>• Always verify transaction details before signing</li>
          <li>• Use a hardware wallet for large holdings</li>
          <li>• Keep your browser and Freighter updated</li>
        </ul>
      </div>

      <div className="flex justify-between">
        <Button variant="outline" onClick={onPrev}>
          <ArrowLeft className="h-4 w-4 mr-1" />
          Back
        </Button>
        <Button onClick={onNext} disabled={!wallet?.connected}>
          Next
          <ArrowRight className="h-4 w-4 ml-1" />
        </Button>
      </div>
    </div>
  );
}
