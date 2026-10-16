"use client";

import type { ComponentType } from "react";
import type { AssetWizardData, AssetWizardFees } from "@/components/AssetWizard";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { formatCurrency, truncateAddress } from "@/lib/utils";
import { CheckCircle2, FileText, ImageIcon, ShieldCheck, Wallet } from "lucide-react";

interface AssetPreviewProps {
  data: AssetWizardData;
  fees: AssetWizardFees;
  walletAddress: string;
}

export function AssetPreview({ data, fees, walletAddress }: AssetPreviewProps) {
  const totalSupply = Number(data.totalSupply || 0);
  const pricePerToken = Number(data.pricePerToken || 0);
  const marketCap = totalSupply * pricePerToken;
  const mediaCount = data.mediaFiles.length + (data.externalMediaUrl ? 1 : 0);

  return (
    <div className="grid gap-4 lg:grid-cols-[1.2fr_0.8fr]">
      <Card>
        <CardHeader>
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <CardTitle className="text-xl">{data.assetName || "Untitled asset"}</CardTitle>
              <p className="mt-1 text-sm text-muted-foreground">{data.category}</p>
            </div>
            <Badge variant="secondary">{data.tokenSymbol || "TOKEN"}</Badge>
          </div>
        </CardHeader>
        <CardContent className="space-y-5">
          <div className="rounded-lg border bg-muted/30 p-4">
            <p className="text-sm leading-6 text-muted-foreground">
              {data.description || "No asset description provided yet."}
            </p>
          </div>

          <div className="grid gap-3 sm:grid-cols-2">
            <PreviewMetric label="Total supply" value={totalSupply.toLocaleString()} />
            <PreviewMetric label="Token price" value={formatCurrency(pricePerToken || 0)} />
            <PreviewMetric label="Estimated market cap" value={formatCurrency(marketCap || 0)} />
            <PreviewMetric label="Royalty" value={`${data.royaltyPercent || "0"}%`} />
          </div>

          <div className="grid gap-3 sm:grid-cols-3">
            <PreviewStatus icon={ImageIcon} label={`${mediaCount} media item${mediaCount === 1 ? "" : "s"}`} />
            <PreviewStatus icon={ShieldCheck} label={data.requiresAccreditation ? "Restricted sale" : "Open sale"} />
            <PreviewStatus icon={FileText} label={data.valuationDocument ? "Valuation linked" : "Valuation pending"} />
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Wallet className="h-4 w-4" />
            Transaction Preview
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-3 text-sm">
            <PreviewRow label="Issuer" value={truncateAddress(walletAddress)} />
            <PreviewRow label="Operation" value="Create asset token" />
            <PreviewRow label="Network" value="Stellar Testnet" />
            <PreviewRow label="Minted supply" value={`${totalSupply.toLocaleString()} ${data.tokenSymbol || "TOKEN"}`} />
            <PreviewRow label="Listing price" value={formatCurrency(pricePerToken || 0)} />
            <PreviewRow label="Network fee" value={`${fees.networkFee} XLM`} />
            <PreviewRow label="Platform fee" value={formatCurrency(fees.platformFee)} />
          </div>

          <div className="rounded-lg border border-primary/20 bg-primary/5 p-4 text-sm">
            <div className="mb-2 flex items-center gap-2 font-medium">
              <CheckCircle2 className="h-4 w-4" />
              Ready to submit
            </div>
            <p className="leading-6 text-muted-foreground">
              Submitting will create tokenization metadata, attach the uploaded media references, and prepare the Stellar
              issuance transaction for wallet approval.
            </p>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

function PreviewMetric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg border p-3">
      <p className="text-xs uppercase text-muted-foreground">{label}</p>
      <p className="mt-1 text-base font-semibold">{value}</p>
    </div>
  );
}

function PreviewStatus({
  icon: Icon,
  label,
}: {
  icon: ComponentType<{ className?: string }>;
  label: string;
}) {
  return (
    <div className="flex min-h-14 items-center gap-2 rounded-lg bg-muted/40 px-3 text-sm font-medium">
      <Icon className="h-4 w-4 text-muted-foreground" />
      <span>{label}</span>
    </div>
  );
}

function PreviewRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-4 border-b pb-2 last:border-0 last:pb-0">
      <span className="text-muted-foreground">{label}</span>
      <span className="text-right font-medium">{value}</span>
    </div>
  );
}
