'use client'

import { useState, useEffect, useCallback } from "react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { PerformanceChart } from "@/components/charts/PerformanceChart";
import { AssetAllocationChart } from "@/components/charts/AssetAllocationChart";
import { assetApi, AssetDetail as AssetDetailType, PricePoint, OwnershipEntry, TransactionEntry, SearchResult } from "@/lib/asset-api";
import { formatCurrency, formatDate, truncateAddress } from "@/lib/utils";
import {
  ArrowLeft, TrendingUp, TrendingDown, Clock, FileText, Share2,
  ChevronDown, ChevronUp, ExternalLink, Copy, Check, Building2,
  MapPin, Tag, Shield, DollarSign, BarChart3, Users,
} from "lucide-react";
import Link from "next/link";

interface AssetDetailProps {
  assetId: string;
}

export function AssetDetail({ assetId }: AssetDetailProps) {
  const [asset, setAsset] = useState<AssetDetailType | null>(null);
  const [priceHistory, setPriceHistory] = useState<PricePoint[]>([]);
  const [ownership, setOwnership] = useState<OwnershipEntry[]>([]);
  const [transactions, setTransactions] = useState<TransactionEntry[]>([]);
  const [related, setRelated] = useState<SearchResult[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [pricePeriod, setPricePeriod] = useState<"7d" | "30d" | "1y">("30d");
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    const load = async () => {
      setIsLoading(true);
      try {
        const [detail, prices, own, txs, rel] = await Promise.all([
          assetApi.getAssetDetail(assetId),
          assetApi.getPriceHistory(assetId, pricePeriod),
          assetApi.getOwnershipDistribution(assetId),
          assetApi.getTransactionHistory(assetId),
          assetApi.getRelatedAssets(assetId),
        ]);
        setAsset(detail);
        setPriceHistory(prices);
        setOwnership(own);
        setTransactions(txs);
        setRelated(rel);
      } catch {
        console.error("Failed to load asset detail");
      } finally {
        setIsLoading(false);
      }
    };
    load();
  }, [assetId, pricePeriod]);

  const handleShare = useCallback(() => {
    const url = window.location.href;
    navigator.clipboard.writeText(url);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }, []);

  if (isLoading) {
    return (
      <div className="space-y-6 animate-pulse">
        <div className="h-8 bg-muted rounded w-1/4" />
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
          <div className="lg:col-span-2"><Card><CardContent className="p-6"><div className="h-64 bg-muted rounded" /></CardContent></Card></div>
          <Card><CardContent className="p-6"><div className="h-64 bg-muted rounded" /></CardContent></Card>
        </div>
      </div>
    );
  }

  if (!asset) {
    return (
      <div className="text-center py-12">
        <Building2 className="h-12 w-12 mx-auto text-muted-foreground mb-4" />
        <h3 className="text-lg font-semibold mb-2">Asset Not Found</h3>
        <Link href="/"><Button variant="outline"><ArrowLeft className="h-4 w-4 mr-2" /> Back to Marketplace</Button></Link>
      </div>
    );
  }

  const ownershipData = ownership.map((o) => ({
    name: o.label || truncateAddress(o.address),
    value: o.percentage,
    percentage: o.percentage,
  }));

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-4">
          <Link href="/">
            <Button variant="ghost" size="icon">
              <ArrowLeft className="h-4 w-4" />
            </Button>
          </Link>
          <div>
            <div className="flex items-center gap-2">
              <h1 className="text-2xl font-bold">{asset.name}</h1>
              {asset.verified && (
                <Badge variant="secondary" className="gap-1">
                  <Shield className="h-3 w-3" /> Verified
                </Badge>
              )}
            </div>
            <p className="text-muted-foreground">{asset.code} - {asset.description}</p>
          </div>
        </div>
        <Button variant="outline" onClick={handleShare}>
          {copied ? <Check className="h-4 w-4 mr-2" /> : <Share2 className="h-4 w-4 mr-2" />}
          {copied ? "Copied!" : "Share"}
        </Button>
      </div>

      {/* Key Stats */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        {[
          { label: "Price", value: formatCurrency(asset.price), icon: DollarSign, change: asset.priceChange24h },
          { label: "Market Cap", value: formatCurrency(asset.marketCap), icon: BarChart3 },
          { label: "24h Volume", value: formatCurrency(asset.volume24h), icon: TrendingUp },
          { label: "Total Supply", value: `${parseFloat(asset.totalSupply).toLocaleString()} ${asset.code}`, icon: Users },
        ].map((stat) => (
          <Card key={stat.label}>
            <CardContent className="p-4">
              <div className="flex items-center gap-2 text-sm text-muted-foreground mb-1">
                <stat.icon className="h-4 w-4" />
                {stat.label}
              </div>
              <div className="text-lg font-bold">{stat.value}</div>
              {stat.change !== undefined && (
                <div className={`text-sm ${stat.change >= 0 ? "text-green-500" : "text-red-500"}`}>
                  {stat.change >= 0 ? "+" : ""}{stat.change}%
                </div>
              )}
            </CardContent>
          </Card>
        ))}
      </div>

      {/* Price Chart */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle>Price History</CardTitle>
              <CardDescription>{asset.code} price chart</CardDescription>
            </div>
            <div className="flex gap-1">
              {(["7d", "30d", "1y"] as const).map((range) => (
                <Button
                  key={range}
                  variant={pricePeriod === range ? "default" : "outline"}
                  size="sm"
                  onClick={() => setPricePeriod(range)}
                >
                  {range}
                </Button>
              ))}
            </div>
          </div>
        </CardHeader>
        <CardContent>
          <PerformanceChart data={priceHistory} height={260} showVolume />
        </CardContent>
      </Card>

      {/* Two-Column Layout */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Left Column: Transaction Timeline + Ownership */}
        <div className="lg:col-span-2 space-y-6">
          {/* Transaction Timeline */}
          <Card>
            <CardHeader>
              <CardTitle>Transaction History</CardTitle>
              <CardDescription>Recent on-chain activity</CardDescription>
            </CardHeader>
            <CardContent className="p-0">
              <div className="divide-y">
                {transactions.map((tx) => (
                  <div key={tx.id} className="flex items-center gap-4 p-4 hover:bg-muted/50">
                    <div className={`p-2 rounded-full shrink-0 ${
                      tx.type === "buy" ? "bg-green-500/10 text-green-500" :
                      tx.type === "sell" ? "bg-red-500/10 text-red-500" :
                      "bg-muted text-muted-foreground"
                    }`}>
                      {tx.type === "buy" ? <TrendingUp className="h-4 w-4" /> :
                       tx.type === "sell" ? <TrendingDown className="h-4 w-4" /> :
                       <Clock className="h-4 w-4" />}
                    </div>
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center justify-between">
                        <p className="text-sm font-medium capitalize">{tx.type}</p>
                        <span className={`text-sm font-medium ${tx.type === "buy" ? "text-green-500" : tx.type === "sell" ? "text-red-500" : ""}`}>
                          {tx.amount.toLocaleString()}
                        </span>
                      </div>
                      <div className="flex items-center gap-2 text-xs text-muted-foreground mt-0.5">
                        <span>{truncateAddress(tx.from)}</span>
                        <ChevronDown className="h-3 w-3 rotate-[-90deg]" />
                        <span>{truncateAddress(tx.to)}</span>
                        <span>·</span>
                        <span>{formatDate(tx.timestamp)}</span>
                        {tx.txHash && (
                          <a href={`https://stellar.expert/explorer/testnet/tx/${tx.txHash}`} target="_blank" rel="noopener noreferrer">
                            <ExternalLink className="h-3 w-3" />
                          </a>
                        )}
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>

          {/* All-Time Stats */}
          <div className="grid grid-cols-2 gap-4">
            <Card>
              <CardHeader className="pb-2">
                <CardDescription>All-Time High</CardDescription>
                <CardTitle className="text-lg text-green-500">{formatCurrency(asset.allTimeHigh)}</CardTitle>
              </CardHeader>
            </Card>
            <Card>
              <CardHeader className="pb-2">
                <CardDescription>All-Time Low</CardDescription>
                <CardTitle className="text-lg text-red-500">{formatCurrency(asset.allTimeLow)}</CardTitle>
              </CardHeader>
            </Card>
          </div>
        </div>

        {/* Right Column: Ownership + Metadata + Related */}
        <div className="space-y-6">
          {/* Ownership Distribution */}
          <Card>
            <CardHeader>
              <CardTitle>Ownership</CardTitle>
              <CardDescription>Distribution pie chart</CardDescription>
            </CardHeader>
            <CardContent>
              <AssetAllocationChart data={ownershipData} />
            </CardContent>
          </Card>

          {/* Metadata */}
          <Card>
            <CardHeader>
              <CardTitle>Metadata</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              <div className="flex items-center gap-2 text-sm">
                <MapPin className="h-4 w-4 text-muted-foreground" />
                <span className="text-muted-foreground">Location:</span>
                <span>{asset.metadata.location || "N/A"}</span>
              </div>
              <div className="flex items-center gap-2 text-sm">
                <Tag className="h-4 w-4 text-muted-foreground" />
                <span className="text-muted-foreground">Category:</span>
                <Badge variant="secondary">{asset.metadata.category || "Uncategorized"}</Badge>
              </div>
              <div className="flex items-center gap-2 text-sm">
                <Shield className="h-4 w-4 text-muted-foreground" />
                <span className="text-muted-foreground">Condition:</span>
                <span>{asset.metadata.condition || "N/A"}</span>
              </div>
              <div className="flex flex-wrap gap-1 pt-2">
                {(asset.metadata.tags || []).map((tag) => (
                  <Badge key={tag} variant="outline" className="text-xs">{tag}</Badge>
                ))}
              </div>
            </CardContent>
          </Card>

          {/* Documents */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <FileText className="h-4 w-4" /> Documents
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-2">
              {asset.documents.length === 0 ? (
                <p className="text-sm text-muted-foreground">No documents available</p>
              ) : (
                asset.documents.map((doc, i) => (
                  <a
                    key={i}
                    href={doc.url}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="flex items-center justify-between p-2 rounded-lg hover:bg-muted transition-colors"
                  >
                    <div className="text-sm">
                      <p className="font-medium">{doc.title}</p>
                      <p className="text-xs text-muted-foreground uppercase">{doc.type}</p>
                    </div>
                    <ExternalLink className="h-4 w-4 text-muted-foreground" />
                  </a>
                ))
              )}
            </CardContent>
          </Card>

          {/* Related Assets */}
          {related.length > 0 && (
            <Card>
              <CardHeader>
                <CardTitle>Related Assets</CardTitle>
              </CardHeader>
              <CardContent className="space-y-3">
                {related.map((r) => (
                  <Link key={r.id} href={`/assets/${r.id}`} className="flex items-center justify-between p-2 rounded-lg hover:bg-muted transition-colors">
                    <div>
                      <p className="text-sm font-medium">{r.name}</p>
                      <p className="text-xs text-muted-foreground">{r.symbol}</p>
                    </div>
                    <div className="text-right">
                      <p className="text-sm font-medium">{formatCurrency(r.price)}</p>
                      <p className={`text-xs ${r.change24h >= 0 ? "text-green-500" : "text-red-500"}`}>
                        {r.change24h >= 0 ? "+" : ""}{r.change24h}%
                      </p>
                    </div>
                  </Link>
                ))}
              </CardContent>
            </Card>
          )}
        </div>
      </div>
    </div>
  );
}
