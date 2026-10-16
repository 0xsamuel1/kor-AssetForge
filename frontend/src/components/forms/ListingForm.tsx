"use client"

import { useState } from "react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card"
import { FormAlert, FormErrorSummary, FormField } from "@/components/forms/FormField"
import { listingFormSchema, type ListingFormData } from "@/lib/validation-schemas"
import { submitFailedMessage } from "@/lib/validation/messages"
import { useFormValidation } from "@/lib/validation/useFormValidation"
import { Loader2 } from "lucide-react"
import { cn } from "@/lib/utils"

interface AssetOption {
  id: string
  name: string
  code: string
}

interface ListingFormProps {
  assets: AssetOption[]
  onSubmit: (data: ListingFormData) => Promise<void>
  className?: string
}

const INITIAL_VALUES: ListingFormData = {
  assetId: "",
  price: "",
  amount: "",
}

const FIELD_LABELS: Record<keyof ListingFormData, string> = {
  assetId: "Asset",
  price: "Price",
  amount: "Amount",
}

export function ListingForm({ assets, onSubmit, className }: ListingFormProps) {
  const {
    values,
    errors,
    submitted,
    formError,
    setFormError,
    setValue,
    markTouched,
    validate,
    summaryRef,
  } = useFormValidation(listingFormSchema, INITIAL_VALUES)
  const [isSubmitting, setIsSubmitting] = useState(false)

  const summary = submitted
    ? (Object.keys(errors) as (keyof ListingFormData)[]).map((field) => ({
        id: `listing-${field}`,
        label: FIELD_LABELS[field],
        message: errors[field] as string,
      }))
    : []

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    const data = validate()
    if (!data) return

    setIsSubmitting(true)
    try {
      await onSubmit(data)
    } catch {
      setFormError(submitFailedMessage("create the listing"))
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <Card className={cn("w-full max-w-lg", className)}>
      <CardHeader>
        <CardTitle>Create Listing</CardTitle>
        <CardDescription>List your asset for sale on the marketplace.</CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={handleSubmit} className="space-y-4" noValidate>
          <FormErrorSummary ref={summaryRef} errors={summary} />
          <FormAlert message={formError} />

          {/* Select's root renders no DOM, so the trigger stays FormField's control. */}
          <Select
            value={values.assetId}
            onValueChange={(value) => {
              setValue("assetId", value)
              markTouched("assetId")
            }}
          >
            <FormField id="listing-assetId" label="Asset" error={errors.assetId} required>
              <SelectTrigger className="w-full" onBlur={() => markTouched("assetId")}>
                <SelectValue placeholder="Select an asset" />
              </SelectTrigger>
            </FormField>
            <SelectContent>
              {assets.map((asset) => (
                <SelectItem key={asset.id} value={asset.id}>
                  {asset.name} ({asset.code})
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          <div className="grid gap-4 sm:grid-cols-2">
            <FormField
              id="listing-price"
              label="Price (USD)"
              error={errors.price}
              hint="Up to 7 decimal places"
              required
            >
              <Input
                type="number"
                step="0.0000001"
                min="0"
                inputMode="decimal"
                value={values.price}
                onChange={(e) => setValue("price", e.target.value)}
                onBlur={() => markTouched("price")}
                placeholder="0.00"
              />
            </FormField>
            <FormField id="listing-amount" label="Amount" error={errors.amount} required>
              <Input
                type="number"
                step="any"
                min="0"
                inputMode="decimal"
                value={values.amount}
                onChange={(e) => setValue("amount", e.target.value)}
                onBlur={() => markTouched("amount")}
                placeholder="1000"
              />
            </FormField>
          </div>

          <Button type="submit" className="w-full" disabled={isSubmitting}>
            {isSubmitting && <Loader2 className="mr-2 h-4 w-4 animate-spin" aria-hidden="true" />}
            {isSubmitting ? "Creating Listing..." : "Create Listing"}
          </Button>
        </form>
      </CardContent>
    </Card>
  )
}
