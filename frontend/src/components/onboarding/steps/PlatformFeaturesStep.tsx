"use client";

import { Button } from "@/components/ui/button";
import { ArrowLeft, ArrowRight, Building2, ShoppingCart, BarChart3, ShieldCheck, Split, Wallet } from "lucide-react";

interface PlatformFeaturesStepProps {
  onNext: () => void
  onPrev: () => void
}

const features = [
  {
    icon: Building2,
    title: "Asset Tokenization",
    description: "Tokenize real-world assets like real estate, art, and commodities into fractional digital tokens on the Stellar network.",
  },
  {
    icon: ShoppingCart,
    title: "Decentralized Marketplace",
    description: "List, buy, and sell tokenized assets in a peer-to-peer marketplace with automated settlement.",
  },
  {
    icon: Split,
    title: "Fractional Ownership",
    description: "Own fractions of high-value assets, making investment accessible to everyone.",
  },
  {
    icon: BarChart3,
    title: "Analytics & Reporting",
    description: "Track portfolio performance, asset valuations, and generate compliance reports.",
  },
  {
    icon: ShieldCheck,
    title: "KYC & Compliance",
    description: "Built-in identity verification and AML screening to ensure regulatory compliance.",
  },
  {
    icon: Wallet,
    title: "Staking & Rewards",
    description: "Stake your tokens to earn rewards and participate in platform governance.",
  },
];

export function PlatformFeaturesStep({ onNext, onPrev }: PlatformFeaturesStepProps) {
  return (
    <div className="flex flex-col space-y-6 py-4">
      <div className="text-center space-y-2">
        <h2 className="text-2xl font-bold">Platform Features</h2>
        <p className="text-muted-foreground max-w-md mx-auto">
          Discover everything you can do on kor-AssetForge
        </p>
      </div>

      <div className="grid gap-3 max-h-80 overflow-y-auto pr-1">
        {features.map((feature) => (
          <div key={feature.title} className="flex gap-3 rounded-lg border p-3">
            <div className="mt-0.5 shrink-0 rounded-md bg-primary/10 p-2">
              <feature.icon className="h-4 w-4 text-primary" />
            </div>
            <div>
              <p className="font-medium text-sm">{feature.title}</p>
              <p className="text-xs text-muted-foreground mt-0.5">{feature.description}</p>
            </div>
          </div>
        ))}
      </div>

      <div className="flex justify-between">
        <Button variant="outline" onClick={onPrev}>
          <ArrowLeft className="h-4 w-4 mr-1" />
          Back
        </Button>
        <Button onClick={onNext}>
          Next
          <ArrowRight className="h-4 w-4 ml-1" />
        </Button>
      </div>
    </div>
  );
}
