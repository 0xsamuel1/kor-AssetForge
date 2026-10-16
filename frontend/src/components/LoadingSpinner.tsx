import { Loader2 } from "lucide-react";

import { cn } from "@/lib/utils";

const SIZE_CLASSES = {
  xs: "size-3",
  sm: "size-4",
  md: "size-6",
  lg: "size-8",
  xl: "size-12",
} as const;

export type LoadingSpinnerSize = keyof typeof SIZE_CLASSES;

interface LoadingSpinnerProps {
  size?: LoadingSpinnerSize;
  /** Announced to screen readers, and shown next to the icon when `showLabel` is set. */
  label?: string;
  showLabel?: boolean;
  className?: string;
}

/**
 * The one spinner used across the app for async operations, so every loading
 * indicator looks and announces itself the same way.
 */
export function LoadingSpinner({
  size = "md",
  label = "Loading…",
  showLabel = false,
  className,
}: LoadingSpinnerProps) {
  return (
    <span
      role="status"
      aria-live="polite"
      className={cn(
        "inline-flex items-center gap-2 text-muted-foreground",
        className,
      )}
    >
      <Loader2
        className={cn("animate-spin shrink-0", SIZE_CLASSES[size])}
        aria-hidden="true"
      />
      {showLabel ? (
        <span className="text-sm">{label}</span>
      ) : (
        <span className="sr-only">{label}</span>
      )}
    </span>
  );
}

/** Spinner sized to sit inside a button next to its label. */
export function ButtonSpinner({ className }: { className?: string }) {
  return (
    <Loader2
      className={cn("size-4 animate-spin shrink-0", className)}
      aria-hidden="true"
    />
  );
}

interface PageLoaderProps {
  label?: string;
  /** `screen` fills the viewport; `section` fills its container. */
  variant?: "screen" | "section";
  className?: string;
}

/** Centered spinner for a whole page or panel whose layout isn't known yet. */
export function PageLoader({
  label = "Loading…",
  variant = "section",
  className,
}: PageLoaderProps) {
  return (
    <div
      className={cn(
        "flex w-full items-center justify-center",
        variant === "screen" ? "min-h-screen bg-background" : "py-16",
        className,
      )}
    >
      <LoadingSpinner size="lg" label={label} showLabel />
    </div>
  );
}
