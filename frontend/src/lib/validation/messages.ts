/**
 * Validation error messages.
 *
 * One place for the wording of every form error, so messages read the same
 * across forms: they name the field, say what's wrong, and where possible say
 * how to fix it.
 */

import type { ZodError } from "zod";

export const validationMessages = {
  required: (field: string) => `${field} is required`,
  tooShort: (field: string, min: number) =>
    `${field} must be at least ${min} character${min === 1 ? "" : "s"}`,
  tooLong: (field: string, max: number) =>
    `${field} must be ${max} characters or fewer`,
  number: (field: string) => `${field} must be a number, e.g. 1000 or 12.5`,
  positive: (field: string) => `${field} must be greater than 0`,
  decimals: (field: string, max: number) =>
    `${field} can have at most ${max} decimal places`,
  email: () => "Enter a valid email address, e.g. name@example.com",
  phone: () => "Enter a valid phone number, e.g. +1 555 123 4567",
  date: () => "Enter a date in YYYY-MM-DD format",
  stellarPublicKey: () =>
    "Enter a valid Stellar public key — 56 characters starting with G",
  assetCode: () =>
    "Asset code can only contain letters and numbers (max 12 characters)",
  /** `thing` includes its article, e.g. "an asset". */
  select: (thing: string) => `Select ${thing}`,
  unique: (field: string) => `This ${field.toLowerCase()} is already taken`,
} as const;

/** Shown in the summary box above a form that failed validation. */
export function errorSummaryTitle(count: number): string {
  return count === 1
    ? "There is 1 problem with this form"
    : `There are ${count} problems with this form`;
}

/** Generic message for a submit that failed after validation passed. */
export const submitFailedMessage = (action: string) =>
  `Couldn't ${action}. Check your connection and try again.`;

/** Field-keyed errors, as rendered by `FormField`. */
export type FieldErrors<T> = Partial<Record<keyof T & string, string>>;

/**
 * Flattens a ZodError into one message per field (the first issue wins, which
 * is the most basic problem, e.g. "required" before "invalid format").
 */
export function zodFieldErrors<T>(error: ZodError): FieldErrors<T> {
  const result: Record<string, string> = {};
  for (const issue of error.issues) {
    const key = issue.path.join(".");
    if (key && !(key in result)) result[key] = issue.message;
  }
  return result as FieldErrors<T>;
}
