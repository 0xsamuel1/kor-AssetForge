"use client"

import { useState } from "react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card"
import { FormAlert, FormErrorSummary, FormField } from "@/components/forms/FormField"
import { assetFormSchema, validateAssetCodeUnique, type AssetFormData } from "@/lib/validation-schemas"
import { submitFailedMessage, validationMessages } from "@/lib/validation/messages"
import { useFormValidation } from "@/lib/validation/useFormValidation"
import { Loader2 } from "lucide-react"
import { cn } from "@/lib/utils"

interface AssetFormProps {
  onSubmit: (data: AssetFormData) => Promise<void>
  className?: string
}

const INITIAL_VALUES: AssetFormData = {
  name: "",
  code: "",
  description: "",
  totalSupply: "",
  issuer: "",
}

const FIELD_LABELS: Record<keyof AssetFormData, string> = {
  name: "Asset name",
  code: "Asset code",
  description: "Description",
  totalSupply: "Total supply",
  issuer: "Issuer public key",
}

export function AssetForm({ onSubmit, className }: AssetFormProps) {
  const {
    values,
    errors,
    submitted,
    formError,
    setFormError,
    setValue,
    markTouched,
    setFieldError,
    validate,
    summaryRef,
  } = useFormValidation(assetFormSchema, INITIAL_VALUES)
  const [isSubmitting, setIsSubmitting] = useState(false)

  const fieldProps = (field: keyof AssetFormData) => ({
    value: values[field] ?? "",
    onBlur: () => markTouched(field),
  })

  const summary = submitted
    ? (Object.keys(errors) as (keyof AssetFormData)[]).map((field) => ({
        id: `asset-${field}`,
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
      const isUnique = await validateAssetCodeUnique(data.code)
      if (!isUnique) {
        setFieldError("code", validationMessages.unique("Asset code"))
        return
      }
      await onSubmit(data)
    } catch {
      setFormError(submitFailedMessage("create the asset"))
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <Card className={cn("w-full max-w-2xl", className)}>
      <CardHeader>
        <CardTitle>Create Asset</CardTitle>
        <CardDescription>Fill in the details to create a new asset on the Stellar network.</CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={handleSubmit} className="space-y-4" noValidate>
          <FormErrorSummary ref={summaryRef} errors={summary} />
          <FormAlert message={formError} />

          <div className="grid gap-4 sm:grid-cols-2">
            <FormField id="asset-name" label="Asset Name" error={errors.name} required>
              <Input
                {...fieldProps("name")}
                onChange={(e) => setValue("name", e.target.value)}
                placeholder="My Token"
              />
            </FormField>

            <FormField
              id="asset-code"
              label="Asset Code"
              error={errors.code}
              hint="Up to 12 letters or numbers"
              required
            >
              <Input
                {...fieldProps("code")}
                onChange={(e) => setValue("code", e.target.value.toUpperCase())}
                placeholder="TOKEN"
                maxLength={12}
              />
            </FormField>
          </div>

          <FormField id="asset-description" label="Description" error={errors.description}>
            <textarea
              {...fieldProps("description")}
              onChange={(e) => setValue("description", e.target.value)}
              placeholder="Describe your asset..."
              className="h-20 w-full min-w-0 rounded-lg border border-input bg-transparent px-2.5 py-1 text-base transition-colors outline-none placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 md:text-sm dark:bg-input/30"
              rows={3}
            />
          </FormField>

          <div className="grid gap-4 sm:grid-cols-2">
            <FormField id="asset-totalSupply" label="Total Supply" error={errors.totalSupply} required>
              <Input
                {...fieldProps("totalSupply")}
                onChange={(e) => setValue("totalSupply", e.target.value)}
                placeholder="1000000"
                inputMode="decimal"
              />
            </FormField>

            <FormField
              id="asset-issuer"
              label="Issuer Public Key"
              error={errors.issuer}
              hint="Optional — a Stellar public key starting with G"
            >
              <Input
                {...fieldProps("issuer")}
                onChange={(e) => setValue("issuer", e.target.value.trim())}
                placeholder="G..."
                spellCheck={false}
                autoComplete="off"
              />
            </FormField>
          </div>

          <Button type="submit" className="w-full" disabled={isSubmitting}>
            {isSubmitting && <Loader2 className="mr-2 h-4 w-4 animate-spin" aria-hidden="true" />}
            {isSubmitting ? "Creating Asset..." : "Create Asset"}
          </Button>
        </form>
      </CardContent>
    </Card>
  )
}
