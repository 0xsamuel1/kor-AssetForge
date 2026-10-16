"use client"

import { useState } from "react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card"
import { FormAlert, FormErrorSummary, FormField } from "@/components/forms/FormField"
import { kycSchema, type KYCFormData } from "@/lib/validation-schemas"
import { submitFailedMessage } from "@/lib/validation/messages"
import { useFormValidation } from "@/lib/validation/useFormValidation"
import { Loader2 } from "lucide-react"
import { cn } from "@/lib/utils"

interface KYCFormProps {
  onSubmit: (data: KYCFormData) => Promise<void>
  className?: string
}

const INITIAL_VALUES: KYCFormData = {
  firstName: "",
  lastName: "",
  email: "",
  phone: "",
  country: "",
  address: "",
  dateOfBirth: "",
  idType: "passport",
  idNumber: "",
}

const FIELD_LABELS: Record<keyof KYCFormData, string> = {
  firstName: "First name",
  lastName: "Last name",
  email: "Email",
  phone: "Phone",
  country: "Country",
  address: "Address",
  dateOfBirth: "Date of birth",
  idType: "ID type",
  idNumber: "ID number",
}

type TextField = Exclude<keyof KYCFormData, "idType">

export function KYCForm({ onSubmit, className }: KYCFormProps) {
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
  } = useFormValidation(kycSchema, INITIAL_VALUES)
  const [isSubmitting, setIsSubmitting] = useState(false)

  const textProps = (field: TextField) => ({
    value: values[field],
    onChange: (e: React.ChangeEvent<HTMLInputElement>) => setValue(field, e.target.value),
    onBlur: () => markTouched(field),
  })

  const summary = submitted
    ? (Object.keys(errors) as (keyof KYCFormData)[]).map((field) => ({
        id: `kyc-${field}`,
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
      setFormError(submitFailedMessage("submit your KYC details"))
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <Card className={cn("w-full max-w-2xl", className)}>
      <CardHeader>
        <CardTitle>KYC Verification</CardTitle>
        <CardDescription>Please provide your identity information for verification.</CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={handleSubmit} className="space-y-4" noValidate>
          <FormErrorSummary ref={summaryRef} errors={summary} />
          <FormAlert message={formError} />

          <div className="grid gap-4 sm:grid-cols-2">
            <FormField id="kyc-firstName" label="First Name" error={errors.firstName} required>
              <Input {...textProps("firstName")} autoComplete="given-name" />
            </FormField>
            <FormField id="kyc-lastName" label="Last Name" error={errors.lastName} required>
              <Input {...textProps("lastName")} autoComplete="family-name" />
            </FormField>
          </div>

          <div className="grid gap-4 sm:grid-cols-2">
            <FormField id="kyc-email" label="Email" error={errors.email} required>
              <Input {...textProps("email")} type="email" autoComplete="email" />
            </FormField>
            <FormField
              id="kyc-phone"
              label="Phone"
              error={errors.phone}
              hint="Include your country code"
              required
            >
              <Input {...textProps("phone")} type="tel" autoComplete="tel" placeholder="+1234567890" />
            </FormField>
          </div>

          <div className="grid gap-4 sm:grid-cols-2">
            <FormField id="kyc-country" label="Country" error={errors.country} required>
              <Input {...textProps("country")} autoComplete="country-name" />
            </FormField>
            <FormField id="kyc-dateOfBirth" label="Date of Birth" error={errors.dateOfBirth} required>
              <Input {...textProps("dateOfBirth")} type="date" autoComplete="bday" />
            </FormField>
          </div>

          <FormField id="kyc-address" label="Address" error={errors.address} required>
            <Input {...textProps("address")} autoComplete="street-address" />
          </FormField>

          <div className="grid gap-4 sm:grid-cols-2">
            {/* Select's root renders no DOM, so the trigger stays FormField's control. */}
            <Select
              value={values.idType}
              onValueChange={(value) => {
                setValue("idType", value as KYCFormData["idType"])
                markTouched("idType")
              }}
            >
              <FormField id="kyc-idType" label="ID Type" error={errors.idType} required>
                <SelectTrigger className="w-full">
                  <SelectValue placeholder="Select ID type" />
                </SelectTrigger>
              </FormField>
              <SelectContent>
                <SelectItem value="passport">Passport</SelectItem>
                <SelectItem value="drivers_license">Driver&apos;s License</SelectItem>
                <SelectItem value="national_id">National ID</SelectItem>
                <SelectItem value="other">Other</SelectItem>
              </SelectContent>
            </Select>
            <FormField id="kyc-idNumber" label="ID Number" error={errors.idNumber} required>
              <Input {...textProps("idNumber")} autoComplete="off" />
            </FormField>
          </div>

          <Button type="submit" className="w-full" disabled={isSubmitting}>
            {isSubmitting && <Loader2 className="mr-2 h-4 w-4 animate-spin" aria-hidden="true" />}
            {isSubmitting ? "Submitting..." : "Submit KYC"}
          </Button>
        </form>
      </CardContent>
    </Card>
  )
}
