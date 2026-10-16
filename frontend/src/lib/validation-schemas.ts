import { z } from "zod"
import { validationMessages as m } from "@/lib/validation/messages"

const stellarPublicKeyRegex = /^G[A-HJ-NP-Z0-9]{55}$/

export const walletSchema = z.object({
  publicKey: z.string().regex(stellarPublicKeyRegex, m.stellarPublicKey()),
})

export const assetFormSchema = z.object({
  name: z.string().trim().min(1, m.required("Asset name")).max(100, m.tooLong("Asset name", 100)),
  code: z
    .string()
    .trim()
    .min(1, m.required("Asset code"))
    .max(12, m.tooLong("Asset code", 12))
    .regex(/^[A-Za-z0-9]+$/, m.assetCode()),
  description: z.string().max(1000, m.tooLong("Description", 1000)).optional(),
  totalSupply: z.string().trim().min(1, m.required("Total supply")).regex(/^\d+(\.\d+)?$/, m.number("Total supply")),
  // Optional: an empty input means "not provided", not an invalid key.
  issuer: z.string().regex(stellarPublicKeyRegex, m.stellarPublicKey()).optional().or(z.literal("")),
})

export const kycSchema = z.object({
  firstName: z.string().trim().min(1, m.required("First name")).max(100, m.tooLong("First name", 100)),
  lastName: z.string().trim().min(1, m.required("Last name")).max(100, m.tooLong("Last name", 100)),
  email: z.string().trim().min(1, m.required("Email")).email(m.email()),
  phone: z.string().trim().min(1, m.required("Phone number")).regex(/^\+?[\d\s\-()]{7,20}$/, m.phone()),
  country: z.string().trim().min(1, m.required("Country")),
  address: z.string().trim().min(1, m.required("Address")).max(500, m.tooLong("Address", 500)),
  dateOfBirth: z.string().min(1, m.required("Date of birth")).regex(/^\d{4}-\d{2}-\d{2}$/, m.date()),
  idType: z.enum(["passport", "drivers_license", "national_id", "other"], {
    errorMap: () => ({ message: m.select("an ID type") }),
  }),
  idNumber: z.string().trim().min(1, m.required("ID number")).max(100, m.tooLong("ID number", 100)),
})

export const listingFormSchema = z.object({
  assetId: z.string().min(1, m.select("an asset")),
  price: z
    .string()
    .min(1, m.required("Price"))
    .regex(/^\d+(\.\d+)?$/, m.number("Price"))
    .regex(/^\d+(\.\d{1,7})?$/, m.decimals("Price", 7))
    .refine((v) => Number(v) > 0, m.positive("Price")),
  amount: z
    .string()
    .min(1, m.required("Amount"))
    .regex(/^\d+(\.\d+)?$/, m.number("Amount"))
    .refine((v) => Number(v) > 0, m.positive("Amount")),
})

export const searchSchema = z.object({
  query: z.string().max(200).optional(),
  category: z.string().optional(),
  minPrice: z.string().regex(/^\d+(\.\d+)?$/, "Invalid minimum price").optional().or(z.literal("")),
  maxPrice: z.string().regex(/^\d+(\.\d+)?$/, "Invalid maximum price").optional().or(z.literal("")),
  location: z.string().optional(),
  sortBy: z.enum(["price_asc", "price_desc", "name_asc", "name_desc", "newest", "oldest"]).optional(),
})

export const contactFormSchema = z.object({
  name: z.string().min(1, "Name is required").max(100),
  email: z.string().email("Invalid email address"),
  subject: z.string().min(1, "Subject is required").max(200),
  message: z.string().min(1, "Message is required").max(5000),
})

export const settingsFormSchema = z.object({
  displayName: z.string().min(1, "Display name is required").max(100),
  email: z.string().email("Invalid email address"),
  notifications: z.boolean().optional(),
  twoFactor: z.boolean().optional(),
  theme: z.enum(["light", "dark", "system"]),
})

export type AssetFormData = z.infer<typeof assetFormSchema>
export type KYCFormData = z.infer<typeof kycSchema>
export type ListingFormData = z.infer<typeof listingFormSchema>
export type SearchFormData = z.infer<typeof searchSchema>
export type ContactFormData = z.infer<typeof contactFormSchema>
export type SettingsFormData = z.infer<typeof settingsFormSchema>

export async function validateUniqueField(
  field: string,
  value: string,
): Promise<boolean> {
  try {
    const backendUrl = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080"
    const res = await fetch(`${backendUrl}/api/v1/validate/${field}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ value }),
    })
    if (!res.ok) return true
    const data = await res.json()
    return data.unique !== false
  } catch {
    return true
  }
}

export async function validateEmailUnique(
  email: string,
): Promise<boolean> {
  return validateUniqueField("email", email)
}

export async function validateAssetCodeUnique(
  code: string,
): Promise<boolean> {
  return validateUniqueField("asset_code", code)
}
