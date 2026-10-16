'use client'

import { useParams } from "next/navigation";
import { Header } from "@/components/Header";
import { AssetDetail } from "@/components/AssetDetail";
import { useState, useEffect } from "react";
import { StellarWallet, stellarService } from "@/lib/stellar";

export default function AssetDetailPage() {
  const params = useParams();
  const assetId = params.id as string;
  const [wallet, setWallet] = useState<StellarWallet | undefined>();

  useEffect(() => {
    const connect = async () => {
      try {
        const w = await stellarService.connectWallet();
        setWallet(w);
      } catch {}
    };
    connect();
  }, []);

  return (
    <div className="min-h-screen bg-background">
      <Header wallet={wallet} />
      <main className="container mx-auto px-4 py-8">
        <AssetDetail assetId={assetId} />
      </main>
    </div>
  );
}
