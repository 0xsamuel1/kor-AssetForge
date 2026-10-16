"use client";

import { useState } from "react";
import { StellarWallet } from "@/lib/stellar";
import { AssetSelectionStep } from "./AssetSelectionStep";
import { FractionConfigStep } from "./FractionConfigStep";
import { PreviewStep } from "./PreviewStep";
import { ConfirmationStep } from "./ConfirmationStep";

export interface FractionalizationConfig {
  assetId: number;
  assetName: string;
  assetSymbol: string;
  totalSupply: number;
  fractionCount: number;
  pricePerFraction: number;
  description: string;
}

interface FractionalizationWizardProps {
  wallet: StellarWallet;
}

export function FractionalizationWizard({ wallet }: FractionalizationWizardProps) {
  const [step, setStep] = useState<"select" | "configure" | "preview" | "confirm">("select");
  const [config, setConfig] = useState<FractionalizationConfig | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  const handleAssetSelected = (selectedConfig: FractionalizationConfig) => {
    setConfig(selectedConfig);
    setStep("configure");
  };

  const handleConfigurationComplete = (updatedConfig: FractionalizationConfig) => {
    setConfig(updatedConfig);
    setStep("preview");
  };

  const handlePreviewComplete = () => {
    setStep("confirm");
  };

  const handleConfirmSubmit = async () => {
    if (!config) return;
    
    setIsSubmitting(true);
    try {
      const response = await fetch("/api/assets/fractionalize", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          asset_id: config.assetId,
          fraction_count: config.fractionCount,
          price_per_fraction: config.pricePerFraction,
          wallet_address: wallet.publicKey,
        }),
      });

      if (response.ok) {
        const result = await response.json();
        alert("Asset fractionalization successful!");
        setStep("select");
        setConfig(null);
      } else {
        alert("Failed to fractionalize asset");
      }
    } catch (error) {
      console.error("Error during fractionalization:", error);
      alert("Error during fractionalization");
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleBack = () => {
    if (step === "configure") {
      setStep("select");
      setConfig(null);
    } else if (step === "preview") {
      setStep("configure");
    } else if (step === "confirm") {
      setStep("preview");
    }
  };

  return (
    <div className="w-full">
      <div className="mb-8">
        <div className="flex justify-between items-center">
          {["select", "configure", "preview", "confirm"].map((s, idx) => (
            <div key={s} className="flex items-center">
              <div
                className={`w-8 h-8 rounded-full flex items-center justify-center font-semibold text-sm ${
                  ["select", "configure", "preview", "confirm"].indexOf(step) >= idx
                    ? "bg-primary text-primary-foreground"
                    : "bg-muted text-muted-foreground"
                }`}
              >
                {idx + 1}
              </div>
              {idx < 3 && <div className="w-12 h-0.5 mx-2 bg-muted"></div>}
            </div>
          ))}
        </div>
      </div>

      {step === "select" && (
        <AssetSelectionStep onAssetSelected={handleAssetSelected} />
      )}

      {step === "configure" && config && (
        <FractionConfigStep
          config={config}
          onConfigurationComplete={handleConfigurationComplete}
        />
      )}

      {step === "preview" && config && (
        <PreviewStep
          config={config}
          onPreviewComplete={handlePreviewComplete}
          onBack={handleBack}
        />
      )}

      {step === "confirm" && config && (
        <ConfirmationStep
          config={config}
          onSubmit={handleConfirmSubmit}
          onBack={handleBack}
          isSubmitting={isSubmitting}
          walletAddress={wallet.publicKey}
        />
      )}
    </div>
  );
}
