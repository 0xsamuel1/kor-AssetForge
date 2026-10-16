"use client"

import { cloneElement, isValidElement, type ReactElement, type ReactNode, type Ref } from "react"
import { AlertCircle } from "lucide-react"

import { Label } from "@/components/ui/label"
import { cn } from "@/lib/utils"
import { errorSummaryTitle } from "@/lib/validation/messages"

type ControlProps = {
  id?: string
  "aria-invalid"?: boolean
  "aria-describedby"?: string
  "aria-required"?: boolean
  "aria-errormessage"?: string
}

interface FormFieldProps {
  /** Becomes the control's `id`, and the prefix for its hint/error ids. */
  id: string
  label: ReactNode
  /** Validation message; when set the field renders in its error state. */
  error?: string
  /** Helper text shown under the control while there is no error. */
  hint?: ReactNode
  required?: boolean
  className?: string
  /** A single form control (Input, textarea, SelectTrigger, …). */
  children: ReactElement<ControlProps>
}

export const fieldErrorId = (id: string) => `${id}-error`
const fieldHintId = (id: string) => `${id}-hint`

/**
 * Label + control + inline error, wired up for assistive tech: the control
 * gets `aria-invalid`, `aria-required` and `aria-describedby` pointing at the
 * hint and error, and the error sits in a polite live region so it is read
 * out as it appears without interrupting typing.
 */
export function FormField({
  id,
  label,
  error,
  hint,
  required = false,
  className,
  children,
}: FormFieldProps) {
  const errorId = fieldErrorId(id)
  const hintId = fieldHintId(id)
  const describedBy = [hint ? hintId : null, error ? errorId : null]
    .filter(Boolean)
    .join(" ")

  const control = isValidElement(children)
    ? cloneElement(children, {
        id,
        "aria-invalid": error ? true : undefined,
        "aria-required": required || undefined,
        "aria-describedby": describedBy || undefined,
        "aria-errormessage": error ? errorId : undefined,
      })
    : children

  return (
    <div
      className={cn("form-field space-y-2", className)}
      data-invalid={error ? "true" : undefined}
    >
      <Label htmlFor={id} className={cn(error && "text-destructive")}>
        {label}
        {required && (
          <span className="form-field-required" aria-hidden="true">
            *
          </span>
        )}
      </Label>
      {control}
      {hint && !error && (
        <p id={hintId} className="text-xs text-muted-foreground">
          {hint}
        </p>
      )}
      {/* Always mounted so screen readers notice when a message appears. */}
      <div aria-live="polite" aria-atomic="true">
        {error && (
          <p id={errorId} className="form-field-error">
            <AlertCircle className="size-3.5 shrink-0" aria-hidden="true" />
            <span>{error}</span>
          </p>
        )}
      </div>
    </div>
  )
}

export interface SummaryError {
  /** Field id to jump to. */
  id: string
  label: string
  message: string
}

interface FormErrorSummaryProps {
  errors: SummaryError[]
  /** Receives focus after a failed submit (see `useFormValidation`). */
  ref?: Ref<HTMLDivElement>
  className?: string
}

/**
 * Form-level list of every problem, shown after a submit attempt. Each entry
 * links to its field so keyboard and screen-reader users can jump straight
 * to it.
 */
export function FormErrorSummary({ errors, ref, className }: FormErrorSummaryProps) {
  if (errors.length === 0) return null

  const focusField = (event: React.MouseEvent<HTMLAnchorElement>, id: string) => {
    const target = document.getElementById(id)
    if (!target) return
    event.preventDefault()
    target.scrollIntoView({ block: "center", behavior: "smooth" })
    target.focus({ preventScroll: true })
  }

  return (
    <div
      ref={ref}
      role="alert"
      tabIndex={-1}
      aria-labelledby="form-error-summary-title"
      className={cn("form-error-summary", className)}
    >
      <p id="form-error-summary-title" className="flex items-center gap-2 font-medium">
        <AlertCircle className="size-4 shrink-0" aria-hidden="true" />
        {errorSummaryTitle(errors.length)}
      </p>
      <ul className="mt-2 space-y-1 pl-6 text-sm">
        {errors.map((error) => (
          <li key={error.id} className="list-disc">
            <a
              href={`#${error.id}`}
              onClick={(event) => focusField(event, error.id)}
              className="underline underline-offset-2 hover:no-underline"
            >
              <span className="font-medium">{error.label}:</span> {error.message}
            </a>
          </li>
        ))}
      </ul>
    </div>
  )
}

/** Form-level message for failures that aren't tied to one field. */
export function FormAlert({ message, className }: { message?: string; className?: string }) {
  if (!message) return null
  return (
    <div role="alert" className={cn("form-alert", className)}>
      <AlertCircle className="size-4 shrink-0" aria-hidden="true" />
      <span>{message}</span>
    </div>
  )
}
