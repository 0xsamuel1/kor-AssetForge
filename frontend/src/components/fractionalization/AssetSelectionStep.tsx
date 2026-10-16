"use client";

import { useState, useEffect } from "react";
import Image from "next/image";
import { FractionalizationConfig } from "./FractionalizationWizard";
import { CardGridSkeleton } from "@/components/LoadingSkeleton";

interface Asset {
  id: number;
  name: string;
  symbol: string;
  total_supply: number;
  image_url: string;
  description: string;
}

interface AssetSelectionStepProps {
  onAssetSelected: (config: FractionalizationConfig) => void;
}

export function AssetSelectionStep({ onAssetSelected }: AssetSelectionStepProps) {
  const [assets, setAssets] = useState<Asset[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const fetchAssets = async () => {
      try {
        const response = await fetch("/api/assets");
        if (!response.ok) throw new Error("Failed to fetch assets");
        const data = await response.json();
        setAssets(data.assets || []);
      } catch (err) {
        setError("Failed to load assets");
        console.error(err);
      } finally {
        setIsLoading(false);
      }
    };

    fetchAssets();
  }, []);

  const handleSelectAsset = (asset: Asset) => {
    onAssetSelected({
      assetId: asset.id,
      assetName: asset.name,
      assetSymbol: asset.symbol,
      totalSupply: asset.total_supply,
      fractionCount: 1000,
      pricePerFraction: 100,
      description: asset.description,
    });
  };

  if (isLoading) {
    return (
      <CardGridSkeleton count={4} className="lg:grid-cols-2" label="Loading assets" />
    );
  }

  if (error) {
    return (
      <div className="bg-destructive/10 border border-destructive rounded-lg p-6">
        <p className="text-destructive">{error}</p>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="bg-card rounded-lg border p-6">
        <h2 className="text-2xl font-semibold mb-2">Step 1: Select Asset</h2>
        <p className="text-muted-foreground mb-6">
          Choose an asset from your portfolio to fractionalize
        </p>

        {assets.length === 0 ? (
          <div className="text-center py-12">
            <p className="text-muted-foreground">No assets available for fractionalization</p>
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            {assets.map((asset) => (
              <button
                key={asset.id}
                onClick={() => handleSelectAsset(asset)}
                className="text-left p-4 border rounded-lg hover:border-primary hover:bg-muted transition-all"
              >
                <div className="flex items-start gap-4">
                  {asset.image_url && (
                    <Image
                      src={asset.image_url}
                      alt={asset.name}
                      width={64}
                      height={64}
                      sizes="64px"
                      className="w-16 h-16 rounded object-cover"
                    />
                  )}
                  <div className="flex-1">
                    <h3 className="font-semibold">{asset.name}</h3>
                    <p className="text-sm text-muted-foreground">{asset.symbol}</p>
                    <p className="text-xs text-muted-foreground mt-2">
                      Supply: {asset.total_supply.toLocaleString()} units
                    </p>
                  </div>
                </div>
              </button>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
