"use client";

import { Button } from "@/components/ui/button";
import { ArrowLeft, ArrowRight, FileText, ShieldCheck, AlertTriangle, CheckCircle } from "lucide-react";

interface KYCStepProps {
  onNext: () => void
  onPrev: () => void
  walletConnected: boolean
  kycStatus?: string
}

const requirements = [
  "Government-issued ID (passport, driver's license, or national ID)",
  "Proof of address (utility bill or bank statement, less than 3 months old)",
  "Selfie/photo for identity verification",
  "Valid email address and phone number",
];

export function KYCStep({ onNext, onPrev, walletConnected, kycStatus }: KYCStepProps) {
  const isKycDone = kycStatus === "approved";

  return (
    <div className="flex flex-col space-y-6 py-4">
      <div className="text-center space-y-2">
        <h2 className="text-2xl font-bold">Identity Verification (KYC)</h2>
        <p className="text-muted-foreground max-w-md mx-auto">
          Complete KYC verification to unlock full platform features including trading and staking
        </p>
      </div>

      {isKycDone ? (
        <div className="flex items-center gap-3 rounded-lg bg-green-50 dark:bg-green-950/30 p-4">
          <CheckCircle className="h-6 w-6 text-green-600 dark:text-green-400 shrink-0" />
          <div>
            <p className="font-medium text-sm text-green-700 dark:text-green-400">KYC Verified</p>
            <p className="text-xs text-muted-foreground mt-0.5">
              Your identity has been verified. You have access to all platform features.
            </p>
          </div>
        </div>
      ) : (
        <>
          <div className="rounded-lg border p-4 space-y-3">
            <div className="flex items-center gap-2">
              <ShieldCheck className="h-5 w-5 text-primary" />
              <span className="font-medium text-sm">What You'll Need</span>
            </div>
            <ul className="space-y-2">
              {requirements.map((req) => (
                <li key={req} className="flex items-start gap-2 text-sm">
                  <FileText className="h-4 w-4 text-muted-foreground mt-0.5 shrink-0" />
                  <span>{req}</span>
                </li>
              ))}
            </ul>
          </div>

          <div className="rounded-lg border p-4 space-y-2">
            <div className="flex items-center gap-2">
              <AlertTriangle className="h-4 w-4 text-amber-500" />
              <span className="font-medium text-sm">Important Notes</span>
            </div>
            <ul className="text-xs text-muted-foreground space-y-1">
              <li>• Your personal data is encrypted and securely stored</li>
              <li>• KYC verification typically takes 1-2 business days</li>
              <li>• You will be notified via email once verification is complete</li>
              <li>• AML screening is performed as part of the verification process</li>
            </ul>
          </div>

          {walletConnected && (
            <Button
              variant="outline"
              className="w-full"
              onClick={() => window.location.href = "/verification"}
            >
              Go to KYC Page
              <ArrowRight className="h-4 w-4 ml-1" />
            </Button>
          )}
        </>
      )}

      <div className="flex justify-between">
        <Button variant="outline" onClick={onPrev}>
          <ArrowLeft className="h-4 w-4 mr-1" />
          Back
        </Button>
        <Button onClick={onNext}>
          {isKycDone ? "Continue" : "Skip for Now"}
          <ArrowRight className="h-4 w-4 ml-1" />
        </Button>
      </div>
    </div>
  );
}
