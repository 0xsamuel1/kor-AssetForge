"use client";

import type { ReactNode } from "react";
import { useMemo, useState } from "react";
import { AssetPreview } from "@/components/AssetPreview";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { StellarWallet } from "@/lib/stellar";
import { cn, formatCurrency } from "@/lib/utils";
import {
  ArrowLeft,
  ArrowRight,
  Check,
  FileImage,
  FileText,
  Loader2,
  Save,
  ShieldCheck,
  Sparkles,
  Tags,
} from "lucide-react";

const DRAFT_KEY = "kor-assetforge-asset-wizard-draft";

const steps = [
  { id: "details", label: "Details", icon: FileText },
  { id: "media", label: "Media", icon: FileImage },
  { id: "pricing", label: "Pricing", icon: Tags },
  { id: "compliance", label: "Compliance", icon: ShieldCheck },
  { id: "preview", label: "Preview", icon: Sparkles },
] as const;

type StepId = (typeof steps)[number]["id"];

export interface AssetWizardData {
  assetName: string;
  tokenSymbol: string;
  category: string;
  description: string;
  location: string;
  mediaFiles: string[];
  externalMediaUrl: string;
  totalSupply: string;
  pricePerToken: string;
  royaltyPercent: string;
  saleType: string;
  valuationAmount: string;
  valuationDocument: string;
  custodianName: string;
  requiresAccreditation: boolean;
  termsAccepted: boolean;
}

export interface AssetWizardFees {
  networkFee: string;
  platformFee: number;
}

interface AssetWizardProps {
  wallet: StellarWallet;
}

const initialData: AssetWizardData = {
  assetName: "",
  tokenSymbol: "",
  category: "Real Estate",
  description: "",
  location: "",
  mediaFiles: [],
  externalMediaUrl: "",
  totalSupply: "1000",
  pricePerToken: "",
  royaltyPercent: "0",
  saleType: "Fixed Price",
  valuationAmount: "",
  valuationDocument: "",
  custodianName: "",
  requiresAccreditation: false,
  termsAccepted: false,
};

export function AssetWizard({ wallet }: AssetWizardProps) {
  const [activeStep, setActiveStep] = useState<StepId>("details");
  const [data, setData] = useState<AssetWizardData>(() => loadDraftData());
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [draftSavedAt, setDraftSavedAt] = useState<string | undefined>(() =>
    hasSavedDraft() ? "Draft restored" : undefined,
  );
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [submitMessage, setSubmitMessage] = useState<string>();

  const activeIndex = steps.findIndex((step) => step.id === activeStep);
  const fees = useMemo<AssetWizardFees>(() => {
    const valuation = Number(data.valuationAmount || 0);
    return {
      networkFee: "0.0000100",
      platformFee: Math.max(10, valuation * 0.005),
    };
  }, [data.valuationAmount]);

  const updateField = <K extends keyof AssetWizardData>(field: K, value: AssetWizardData[K]) => {
    setData((current) => ({ ...current, [field]: value }));
    setErrors((current) => {
      const next = { ...current };
      delete next[field];
      return next;
    });
  };

  const validateStep = (stepId: StepId) => {
    const nextErrors: Record<string, string> = {};

    if (stepId === "details") {
      if (!data.assetName.trim()) nextErrors.assetName = "Asset name is required.";
      if (!/^[A-Z0-9]{2,12}$/.test(data.tokenSymbol.trim())) {
        nextErrors.tokenSymbol = "Use 2-12 uppercase letters or numbers.";
      }
      if (data.description.trim().length < 40) {
        nextErrors.description = "Add at least 40 characters so buyers understand the asset.";
      }
      if (!data.location.trim()) nextErrors.location = "Asset location is required.";
    }

    if (stepId === "media") {
      if (data.mediaFiles.length === 0 && !data.externalMediaUrl.trim()) {
        nextErrors.mediaFiles = "Add at least one file name or an external media URL.";
      }
      if (data.externalMediaUrl && !/^https?:\/\//i.test(data.externalMediaUrl)) {
        nextErrors.externalMediaUrl = "Use a valid http or https URL.";
      }
    }

    if (stepId === "pricing") {
      if (Number(data.totalSupply) <= 0) nextErrors.totalSupply = "Supply must be greater than zero.";
      if (Number(data.pricePerToken) <= 0) nextErrors.pricePerToken = "Token price must be greater than zero.";
      if (Number(data.royaltyPercent) < 0 || Number(data.royaltyPercent) > 25) {
        nextErrors.royaltyPercent = "Royalty must be between 0% and 25%.";
      }
    }

    if (stepId === "compliance") {
      if (Number(data.valuationAmount) <= 0) nextErrors.valuationAmount = "Valuation amount is required.";
      if (!data.custodianName.trim()) nextErrors.custodianName = "Custodian or verifier name is required.";
      if (!data.termsAccepted) nextErrors.termsAccepted = "Accept the tokenization terms to continue.";
    }

    setErrors(nextErrors);
    return Object.keys(nextErrors).length === 0;
  };

  const handleNext = () => {
    if (!validateStep(activeStep)) return;
    const nextStep = steps[activeIndex + 1];
    if (nextStep) setActiveStep(nextStep.id);
  };

  const handleBack = () => {
    const previousStep = steps[activeIndex - 1];
    if (previousStep) {
      setActiveStep(previousStep.id);
      setSubmitMessage(undefined);
    }
  };

  const saveDraft = () => {
    window.localStorage.setItem(DRAFT_KEY, JSON.stringify(data));
    setDraftSavedAt(new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }));
  };

  const handleSubmit = async () => {
    if (!validateStep("compliance")) {
      setActiveStep("compliance");
      return;
    }

    setIsSubmitting(true);
    setSubmitMessage(undefined);

    try {
      const response = await fetch(`${process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080"}/api/v1/assets/tokenize`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          ...data,
          totalSupply: Number(data.totalSupply),
          pricePerToken: Number(data.pricePerToken),
          royaltyPercent: Number(data.royaltyPercent),
          valuationAmount: Number(data.valuationAmount),
          issuer: wallet.publicKey,
        }),
      });

      if (!response.ok) throw new Error("Tokenization request failed");

      window.localStorage.removeItem(DRAFT_KEY);
      setSubmitMessage("Asset tokenization request submitted successfully.");
    } catch (error) {
      console.error("Failed to submit asset tokenization:", error);
      setSubmitMessage("Could not submit the asset yet. Your draft remains saved locally.");
      saveDraft();
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div className="space-y-6">
      <WizardProgress activeStep={activeStep} />

      <div className="flex flex-col gap-3 border-b pb-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h2 className="text-2xl font-semibold">Create tokenized asset</h2>
          <p className="text-sm text-muted-foreground">
            Complete each step to prepare asset metadata and issuance details.
          </p>
        </div>
        <div className="flex items-center gap-3">
          {draftSavedAt && <span className="text-xs text-muted-foreground">{draftSavedAt}</span>}
          <Button type="button" variant="outline" onClick={saveDraft}>
            <Save className="h-4 w-4" />
            Save Draft
          </Button>
        </div>
      </div>

      {activeStep === "details" && (
        <Card>
          <CardHeader>
            <CardTitle>Asset Details</CardTitle>
          </CardHeader>
          <CardContent className="grid gap-4 md:grid-cols-2">
            <Field label="Asset name" error={errors.assetName}>
              <Input value={data.assetName} onChange={(event) => updateField("assetName", event.target.value)} />
            </Field>
            <Field label="Token symbol" error={errors.tokenSymbol}>
              <Input
                value={data.tokenSymbol}
                maxLength={12}
                onChange={(event) => updateField("tokenSymbol", event.target.value.toUpperCase())}
              />
            </Field>
            <Field label="Category">
              <Select value={data.category} onValueChange={(value) => updateField("category", value)}>
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="Real Estate">Real Estate</SelectItem>
                  <SelectItem value="Fine Art">Fine Art</SelectItem>
                  <SelectItem value="Collectibles">Collectibles</SelectItem>
                  <SelectItem value="Commodities">Commodities</SelectItem>
                  <SelectItem value="Private Credit">Private Credit</SelectItem>
                </SelectContent>
              </Select>
            </Field>
            <Field label="Location" error={errors.location}>
              <Input value={data.location} onChange={(event) => updateField("location", event.target.value)} />
            </Field>
            <Field label="Description" error={errors.description} className="md:col-span-2">
              <textarea
                className="min-h-32 w-full rounded-lg border border-input bg-transparent px-3 py-2 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
                value={data.description}
                onChange={(event) => updateField("description", event.target.value)}
              />
            </Field>
          </CardContent>
        </Card>
      )}

      {activeStep === "media" && (
        <Card>
          <CardHeader>
            <CardTitle>Media Upload</CardTitle>
          </CardHeader>
          <CardContent className="space-y-5">
            <Field label="Upload files" error={errors.mediaFiles}>
              <Input
                type="file"
                multiple
                accept="image/*,.pdf"
                onChange={(event) =>
                  updateField(
                    "mediaFiles",
                    Array.from(event.target.files ?? []).map((file) => file.name),
                  )
                }
              />
            </Field>
            {data.mediaFiles.length > 0 && (
              <div className="grid gap-2 sm:grid-cols-2">
                {data.mediaFiles.map((file) => (
                  <div key={file} className="flex items-center gap-2 rounded-lg border px-3 py-2 text-sm">
                    <FileImage className="h-4 w-4 text-muted-foreground" />
                    <span className="truncate">{file}</span>
                  </div>
                ))}
              </div>
            )}
            <Field label="External media URL" error={errors.externalMediaUrl}>
              <Input
                value={data.externalMediaUrl}
                placeholder="https://..."
                onChange={(event) => updateField("externalMediaUrl", event.target.value)}
              />
            </Field>
          </CardContent>
        </Card>
      )}

      {activeStep === "pricing" && (
        <Card>
          <CardHeader>
            <CardTitle>Pricing and Supply</CardTitle>
          </CardHeader>
          <CardContent className="grid gap-4 md:grid-cols-2">
            <Field label="Total token supply" error={errors.totalSupply}>
              <Input type="number" min="1" value={data.totalSupply} onChange={(event) => updateField("totalSupply", event.target.value)} />
            </Field>
            <Field label="Price per token" error={errors.pricePerToken}>
              <Input type="number" min="0" step="0.01" value={data.pricePerToken} onChange={(event) => updateField("pricePerToken", event.target.value)} />
            </Field>
            <Field label="Royalty percentage" error={errors.royaltyPercent}>
              <Input type="number" min="0" max="25" value={data.royaltyPercent} onChange={(event) => updateField("royaltyPercent", event.target.value)} />
            </Field>
            <Field label="Sale type">
              <Select value={data.saleType} onValueChange={(value) => updateField("saleType", value)}>
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="Fixed Price">Fixed Price</SelectItem>
                  <SelectItem value="Auction">Auction</SelectItem>
                  <SelectItem value="Private Placement">Private Placement</SelectItem>
                </SelectContent>
              </Select>
            </Field>
            <div className="rounded-lg border bg-muted/30 p-4 md:col-span-2">
              <p className="text-sm text-muted-foreground">Estimated market cap</p>
              <p className="mt-1 text-2xl font-semibold">
                {formatCurrency(Number(data.totalSupply || 0) * Number(data.pricePerToken || 0))}
              </p>
            </div>
          </CardContent>
        </Card>
      )}

      {activeStep === "compliance" && (
        <Card>
          <CardHeader>
            <CardTitle>Compliance and Custody</CardTitle>
          </CardHeader>
          <CardContent className="grid gap-4 md:grid-cols-2">
            <Field label="Valuation amount" error={errors.valuationAmount}>
              <Input type="number" min="0" step="0.01" value={data.valuationAmount} onChange={(event) => updateField("valuationAmount", event.target.value)} />
            </Field>
            <Field label="Custodian or verifier" error={errors.custodianName}>
              <Input value={data.custodianName} onChange={(event) => updateField("custodianName", event.target.value)} />
            </Field>
            <Field label="Valuation document URL" className="md:col-span-2">
              <Input value={data.valuationDocument} placeholder="https://..." onChange={(event) => updateField("valuationDocument", event.target.value)} />
            </Field>
            <label className="flex items-start gap-3 rounded-lg border p-4 text-sm md:col-span-2">
              <input
                type="checkbox"
                className="mt-1"
                checked={data.requiresAccreditation}
                onChange={(event) => updateField("requiresAccreditation", event.target.checked)}
              />
              <span>Require accredited or approved buyers for this asset.</span>
            </label>
            <label className="flex items-start gap-3 rounded-lg border p-4 text-sm md:col-span-2">
              <input
                type="checkbox"
                className="mt-1"
                checked={data.termsAccepted}
                onChange={(event) => updateField("termsAccepted", event.target.checked)}
              />
              <span>I confirm the asset information is accurate and I have authority to tokenize it.</span>
            </label>
            {errors.termsAccepted && <p className="text-sm text-destructive md:col-span-2">{errors.termsAccepted}</p>}
          </CardContent>
        </Card>
      )}

      {activeStep === "preview" && (
        <div className="space-y-4">
          <AssetPreview data={data} fees={fees} walletAddress={wallet.publicKey} />
          {submitMessage && (
            <div className="rounded-lg border bg-muted/30 p-4 text-sm text-muted-foreground">{submitMessage}</div>
          )}
        </div>
      )}

      <div className="flex flex-col-reverse gap-3 border-t pt-4 sm:flex-row sm:items-center sm:justify-between">
        <Button type="button" variant="outline" onClick={handleBack} disabled={activeIndex === 0 || isSubmitting}>
          <ArrowLeft className="h-4 w-4" />
          Back
        </Button>
        {activeStep === "preview" ? (
          <Button type="button" onClick={handleSubmit} disabled={isSubmitting}>
            {isSubmitting ? <Loader2 className="h-4 w-4 animate-spin" /> : <Check className="h-4 w-4" />}
            Submit Asset
          </Button>
        ) : (
          <Button type="button" onClick={handleNext}>
            Continue
            <ArrowRight className="h-4 w-4" />
          </Button>
        )}
      </div>
    </div>
  );
}

function hasSavedDraft() {
  return typeof window !== "undefined" && Boolean(window.localStorage.getItem(DRAFT_KEY));
}

function loadDraftData() {
  if (typeof window === "undefined") return initialData;

  const draft = window.localStorage.getItem(DRAFT_KEY);
  if (!draft) return initialData;

  try {
    const parsed = JSON.parse(draft) as Partial<AssetWizardData>;
    return { ...initialData, ...parsed, mediaFiles: parsed.mediaFiles ?? [] };
  } catch {
    window.localStorage.removeItem(DRAFT_KEY);
    return initialData;
  }
}

function WizardProgress({ activeStep }: { activeStep: StepId }) {
  const activeIndex = steps.findIndex((step) => step.id === activeStep);

  return (
    <div className="grid gap-2 sm:grid-cols-5">
      {steps.map((step, index) => {
        const Icon = step.icon;
        const isComplete = index < activeIndex;
        const isActive = step.id === activeStep;

        return (
          <div
            key={step.id}
            className={cn(
              "flex min-h-16 items-center gap-3 rounded-lg border px-3 py-2 text-sm",
              isActive && "border-primary bg-primary/5",
              isComplete && "bg-muted/50",
            )}
          >
            <span
              className={cn(
                "flex h-8 w-8 shrink-0 items-center justify-center rounded-full border",
                (isActive || isComplete) && "border-primary bg-primary text-primary-foreground",
              )}
            >
              {isComplete ? <Check className="h-4 w-4" /> : <Icon className="h-4 w-4" />}
            </span>
            <span className="font-medium">{step.label}</span>
          </div>
        );
      })}
    </div>
  );
}

function Field({
  label,
  error,
  className,
  children,
}: {
  label: string;
  error?: string;
  className?: string;
  children: ReactNode;
}) {
  return (
    <div className={cn("space-y-2", className)}>
      <Label>{label}</Label>
      {children}
      {error && <p className="text-sm text-destructive">{error}</p>}
    </div>
  );
}
