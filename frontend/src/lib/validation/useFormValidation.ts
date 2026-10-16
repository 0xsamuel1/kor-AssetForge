"use client"

import { useCallback, useMemo, useRef, useState } from "react"
import type { ZodType, ZodTypeDef } from "zod"

import { zodFieldErrors, type FieldErrors } from "./messages"

type Values = Record<string, unknown>

/**
 * Real-time validation for a zod-backed form.
 *
 * - A field shows its error once it has been blurred (or the form submitted),
 *   so users aren't scolded while typing a value for the first time.
 * - After that, the error updates on every change, so it disappears the
 *   moment the input becomes valid.
 * - A failed submit reveals every error and focuses the error summary.
 */
export function useFormValidation<T extends Values, Out = T>(
  schema: ZodType<Out, ZodTypeDef, T>,
  initialValues: T,
) {
  const [values, setValues] = useState<T>(initialValues)
  const [touched, setTouched] = useState<Partial<Record<keyof T, boolean>>>({})
  const [submitted, setSubmitted] = useState(false)
  /** Errors set from outside the schema, e.g. "already taken" from the API. */
  const [externalErrors, setExternalErrors] = useState<FieldErrors<T>>({})
  const [formError, setFormError] = useState<string | undefined>()
  const summaryRef = useRef<HTMLDivElement>(null)

  // Validating the whole object is cheap for forms this size and keeps
  // cross-field rules (refinements) correct.
  const schemaErrors = useMemo<FieldErrors<T>>(() => {
    const result = schema.safeParse(values)
    return result.success ? {} : zodFieldErrors<T>(result.error)
  }, [schema, values])

  const errors = useMemo(() => {
    const visible: FieldErrors<T> = {}
    for (const key of Object.keys(values) as (keyof T & string)[]) {
      const message = externalErrors[key] ?? schemaErrors[key]
      if (message && (submitted || touched[key])) visible[key] = message
    }
    return visible
  }, [values, externalErrors, schemaErrors, submitted, touched])

  const setValue = useCallback(<K extends keyof T>(field: K, value: T[K]) => {
    setValues((prev) => ({ ...prev, [field]: value }))
    setExternalErrors((prev) => {
      if (!(field in prev)) return prev
      const next = { ...prev }
      delete next[field as keyof T & string]
      return next
    })
    setFormError(undefined)
  }, [])

  const markTouched = useCallback((field: keyof T) => {
    setTouched((prev) => (prev[field] ? prev : { ...prev, [field]: true }))
  }, [])

  const setFieldError = useCallback((field: keyof T & string, message: string) => {
    setExternalErrors((prev) => ({ ...prev, [field]: message }))
  }, [])

  /** Validates everything; returns parsed data, or `null` and focuses the summary. */
  const validate = useCallback((): Out | null => {
    setSubmitted(true)
    setFormError(undefined)
    const result = schema.safeParse(values)
    if (result.success && Object.keys(externalErrors).length === 0) {
      return result.data
    }
    // Wait for the summary to render before moving focus to it.
    requestAnimationFrame(() => summaryRef.current?.focus())
    return null
  }, [schema, values, externalErrors])

  const reset = useCallback(() => {
    setValues(initialValues)
    setTouched({})
    setSubmitted(false)
    setExternalErrors({})
    setFormError(undefined)
  }, [initialValues])

  return {
    values,
    errors,
    /** True once the user has tried to submit — gates the error summary. */
    submitted,
    formError,
    setFormError,
    setValue,
    markTouched,
    setFieldError,
    validate,
    reset,
    summaryRef,
  }
}
