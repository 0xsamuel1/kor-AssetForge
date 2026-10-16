"use client";

import { Button } from "@/components/ui/button";
import { CheckCircle2, ArrowRight, ExternalLink } from "lucide-react";

interface CompletionStepProps {
  onComplete: () => void
  walletConnected: boolean
  kycStatus?: string
}

export function CompletionStep({ onComplete, walletConnected, kycStatus }: CompletionStepProps) {
  const items = [
    { label: "Platform Overview", done: true },
    { label: "Wallet Setup", done: walletConnected },
    { label: "Identity Verification (KYC)", done: kycStatus === "approved" },
  ];

  const incompleteCount = items.filter((i) => !i.done).length;

  return (
    <div className="flex flex-col items-center text-center space-y-6 py-6">
      <div className="rounded-full bg-green-100 dark:bg-green-950/30 p-4">
        <CheckCircle2 className="h-10 w-10 text-green-600 dark:text-green-400" />
      </div>

      <div className="space-y-2">
        <h2 className="text-2xl font-bold">You're All Set!</h2>
        <p className="text-muted-foreground max-w-sm mx-auto">
          You've completed the onboarding tour. Here's a summary of your progress:
        </p>
      </div>

      <div className="w-full max-w-sm space-y-2">
        {items.map((item) => (
          <div
            key={item.label}
            className="flex items-center justify-between rounded-lg border p-3"
          >
            <span className="text-sm">{item.label}</span>
            {item.done ? (
              <CheckCircle2 className="h-4 w-4 text-green-600 dark:text-green-400" />
            ) : (
              <span className="text-xs text-muted-foreground">Pending</span>
            )}
          </div>
        ))}
      </div>

      {incompleteCount > 0 && (
        <p className="text-xs text-muted-foreground max-w-xs">
          You have {incompleteCount} incomplete item{incompleteCount > 1 ? "s" : ""}.
          You can complete {incompleteCount > 1 ? "them" : "it"} anytime from the dashboard.
        </p>
      )}

      <div className="flex flex-col gap-2 w-full max-w-sm">
        <Button onClick={onComplete} className="w-full">
          Start Using kor-AssetForge
          <ArrowRight className="h-4 w-4 ml-1" />
        </Button>
        {!walletConnected && (
          <Button variant="outline" onClick={() => window.location.href = "/"} className="w-full">
            Connect Wallet Now
          </Button>
        )}
        {walletConnected && kycStatus !== "approved" && (
          <Button variant="outline" onClick={() => window.location.href = "/verification"} className="w-full">
            <ExternalLink className="h-4 w-4 mr-1" />
            Complete KYC Verification
          </Button>
        )}
      </div>
    </div>
  );
}
