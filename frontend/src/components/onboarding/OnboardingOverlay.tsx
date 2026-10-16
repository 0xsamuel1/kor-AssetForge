"use client";

import { useEffect } from "react";
import { useOnboarding } from "./OnboardingProvider";
import { WelcomeStep } from "./steps/WelcomeStep";
import { PlatformFeaturesStep } from "./steps/PlatformFeaturesStep";
import { WalletSetupStep } from "./steps/WalletSetupStep";
import { KYCStep } from "./steps/KYCStep";
import { CompletionStep } from "./steps/CompletionStep";
import { Progress } from "@/components/ui/progress";
import { X } from "lucide-react";
import { StellarWallet } from "@/lib/stellar";
import { stellarService } from "@/lib/stellar";

interface OnboardingOverlayProps {
  wallet?: StellarWallet
  onWalletConnected: (wallet: StellarWallet) => void
  kycStatus?: string
}

export function OnboardingOverlay({ wallet, onWalletConnected, kycStatus }: OnboardingOverlayProps) {
  const {
    isOpen,
    currentStep,
    stepIndex,
    totalSteps,
    nextStep,
    prevStep,
    skipOnboarding,
    completeOnboarding,
  } = useOnboarding();

  useEffect(() => {
    if (isOpen) {
      document.body.style.overflow = "hidden";
    }
    return () => {
      document.body.style.overflow = "";
    };
  }, [isOpen]);

  if (!isOpen) return null;

  const progressPercent = ((stepIndex + 1) / totalSteps) * 100;

  const stepLabels = ["Welcome", "Features", "Wallet", "KYC", "Done"];

  const renderStep = () => {
    switch (currentStep) {
      case "welcome":
        return <WelcomeStep onNext={nextStep} onSkip={skipOnboarding} />;
      case "features":
        return <PlatformFeaturesStep onNext={nextStep} onPrev={prevStep} />;
      case "wallet":
        return (
          <WalletSetupStep
            onNext={nextStep}
            onPrev={prevStep}
            wallet={wallet}
            onWalletConnected={onWalletConnected}
          />
        );
      case "kyc":
        return (
          <KYCStep
            onNext={nextStep}
            onPrev={prevStep}
            walletConnected={!!wallet?.connected}
            kycStatus={kycStatus}
          />
        );
      case "completion":
        return (
          <CompletionStep
            onComplete={completeOnboarding}
            walletConnected={!!wallet?.connected}
            kycStatus={kycStatus}
          />
        );
      default:
        return null;
    }
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm"
      role="dialog"
      aria-modal="true"
      aria-label="Onboarding tour"
    >
      <div className="relative w-full max-w-lg mx-4 rounded-xl bg-background shadow-2xl border">
        <div className="flex items-center justify-between px-6 pt-5 pb-2">
          <div className="flex items-center gap-2">
            <span className="text-xs font-medium text-muted-foreground">
              Step {stepIndex + 1} of {totalSteps}
            </span>
            <span className="text-xs text-muted-foreground/60">|</span>
            <span className="text-xs font-medium">{stepLabels[stepIndex]}</span>
          </div>
          <button
            onClick={skipOnboarding}
            className="rounded-md p-1 text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
            aria-label="Close onboarding"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        <Progress value={progressPercent} className="mx-6" />

        <div className="px-6 py-4 max-h-[70vh] overflow-y-auto">
          {renderStep()}
        </div>
      </div>
    </div>
  );
}
