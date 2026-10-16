"use client";

import { useState } from "react";
import { FractionalizationConfig } from "./FractionalizationWizard";
import { ButtonSpinner } from "@/components/LoadingSpinner";

interface ConfirmationStepProps {
  config: FractionalizationConfig;
  onSubmit: () => void;
  onBack: () => void;
  isSubmitting: boolean;
  walletAddress: string;
}

export function ConfirmationStep({
  config,
  onSubmit,
  onBack,
  isSubmitting,
  walletAddress,
}: ConfirmationStepProps) {
  const [agreedToTerms, setAgreedToTerms] = useState(false);
  const fractionValue = (config.totalSupply / config.fractionCount).toFixed(2);
  const totalValue = (config.fractionCount * config.pricePerFraction).toFixed(2);

  return (
    <div className="space-y-6">
      <div className="bg-card rounded-lg border p-6">
        <h2 className="text-2xl font-semibold mb-2">Step 4: Confirmation</h2>
        <p className="text-muted-foreground mb-6">
          Review and confirm your fractionalization request
        </p>

        <div className="space-y-6">
          <div className="bg-muted/50 rounded p-4">
            <h3 className="font-semibold mb-4">Transaction Details</h3>
            <div className="space-y-3 text-sm grid grid-cols-2 gap-4">
              <div>
                <p className="text-muted-foreground">Asset Name</p>
                <p className="font-medium">{config.assetName}</p>
              </div>
              <div>
                <p className="text-muted-foreground">Wallet Address</p>
                <p className="font-medium text-xs break-all">{walletAddress}</p>
              </div>
              <div>
                <p className="text-muted-foreground">Total Fractions</p>
                <p className="font-medium">{config.fractionCount.toLocaleString()}</p>
              </div>
              <div>
                <p className="text-muted-foreground">Price per Fraction</p>
                <p className="font-medium">{config.pricePerFraction} stroops</p>
              </div>
              <div>
                <p className="text-muted-foreground">Units per Fraction</p>
                <p className="font-medium">{fractionValue}</p>
              </div>
              <div>
                <p className="text-muted-foreground">Total Market Value</p>
                <p className="font-medium">{totalValue} stroops</p>
              </div>
            </div>
          </div>

          <div className="bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-900/50 rounded p-4">
            <h4 className="font-semibold text-yellow-900 dark:text-yellow-100 mb-2">Important Information</h4>
            <ul className="text-sm text-yellow-800 dark:text-yellow-200 space-y-1">
              <li>✓ Fractionalization is irreversible</li>
              <li>✓ All fractions will be tradeable on the marketplace</li>
              <li>✓ You will receive all fractions to your wallet</li>
              <li>✓ Network fees will apply to this transaction</li>
            </ul>
          </div>

          <div className="space-y-3">
            <label className="flex items-start gap-3 cursor-pointer">
              <input
                type="checkbox"
                checked={agreedToTerms}
                onChange={(e) => setAgreedToTerms(e.target.checked)}
                className="mt-1"
              />
              <span className="text-sm text-muted-foreground">
                I understand that fractionalization is irreversible and agree to proceed with this transaction
              </span>
            </label>
          </div>

          <div className="bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-900/50 rounded p-4">
            <h4 className="font-semibold text-blue-900 dark:text-blue-100 mb-2">Transaction Status</h4>
            <p className="text-sm text-blue-800 dark:text-blue-200">
              This action will create {config.fractionCount.toLocaleString()} fractions of your asset and list them on the marketplace.
              The transaction will be broadcast to the Stellar network for confirmation.
            </p>
          </div>
        </div>

        <div className="mt-8 flex gap-4">
          <button
            onClick={onBack}
            disabled={isSubmitting}
            className="flex-1 border border-input bg-background text-foreground py-2 rounded-lg font-medium hover:bg-muted transition disabled:opacity-50"
          >
            Back
          </button>
          <button
            onClick={onSubmit}
            disabled={!agreedToTerms || isSubmitting}
            className="flex-1 inline-flex items-center justify-center gap-2 bg-primary text-primary-foreground py-2 rounded-lg font-medium hover:bg-primary/90 transition disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {isSubmitting && <ButtonSpinner />}
            {isSubmitting ? "Processing…" : "Confirm Fractionalization"}
          </button>
        </div>
      </div>
    </div>
  );
}
