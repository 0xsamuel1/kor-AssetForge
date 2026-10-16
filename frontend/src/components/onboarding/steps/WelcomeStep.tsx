"use client";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Sparkles, ArrowRight, X } from "lucide-react";

interface WelcomeStepProps {
  onNext: () => void
  onSkip: () => void
}

export function WelcomeStep({ onNext, onSkip }: WelcomeStepProps) {
  return (
    <div className="flex flex-col items-center text-center space-y-6 py-6">
      <div className="rounded-full bg-primary/10 p-4">
        <Sparkles className="h-10 w-10 text-primary" />
      </div>

      <div className="space-y-2">
        <h2 className="text-2xl font-bold">Welcome to kor-AssetForge</h2>
        <p className="text-muted-foreground max-w-md">
          The decentralized marketplace for tokenizing and trading real-world assets on the Stellar network.
        </p>
      </div>

      <div className="grid gap-3 w-full max-w-sm">
        <div className="rounded-lg border p-3 text-left">
          <p className="font-medium text-sm">Tokenize Real-World Assets</p>
          <p className="text-xs text-muted-foreground mt-1">
            Convert real estate, art, and commodities into digital tokens
          </p>
        </div>
        <div className="rounded-lg border p-3 text-left">
          <p className="font-medium text-sm">Trade on a Decentralized Marketplace</p>
          <p className="text-xs text-muted-foreground mt-1">
            Buy, sell, and trade fractional ownership seamlessly
          </p>
        </div>
        <div className="rounded-lg border p-3 text-left">
          <p className="font-medium text-sm">Earn Through Staking & Dividends</p>
          <p className="text-xs text-muted-foreground mt-1">
            Stake your tokens and earn rewards from asset performance
          </p>
        </div>
      </div>

      <div className="flex gap-3">
        <Button variant="outline" onClick={onSkip}>
          <X className="h-4 w-4 mr-1" />
          Skip Tour
        </Button>
        <Button onClick={onNext}>
          Get Started
          <ArrowRight className="h-4 w-4 ml-1" />
        </Button>
      </div>
    </div>
  );
}
