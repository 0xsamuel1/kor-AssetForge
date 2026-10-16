"use client";

import { ExternalLink, RotateCw, TriangleAlert, X } from "lucide-react";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import {
  getWalletErrorDetails,
  type WalletProviderId,
} from "@/lib/wallet/errors";

interface WalletErrorAlertProps {
  error: unknown;
  /** Used for wording when `error` isn't a WalletError. */
  providerId?: WalletProviderId;
  /** Shown as an install link for `not-installed` errors. */
  installUrl?: string;
  onRetry?: () => void;
  onDismiss?: () => void;
  isRetrying?: boolean;
  className?: string;
}

/**
 * Explains a wallet failure: what happened, and numbered steps to fix it,
 * with retry / install actions where they apply.
 */
export function WalletErrorAlert({
  error,
  providerId,
  installUrl,
  onRetry,
  onDismiss,
  isRetrying = false,
  className,
}: WalletErrorAlertProps) {
  const details = getWalletErrorDetails(error, providerId);

  return (
    <div
      role="alert"
      data-error-code={details.code}
      className={cn(
        "rounded-lg bg-destructive/10 p-3 text-sm text-destructive",
        className,
      )}
    >
      <div className="flex items-start gap-2">
        <TriangleAlert className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
        <div className="min-w-0 flex-1 space-y-2">
          <div>
            <p className="font-medium">{details.title}</p>
            <p className="text-destructive/90">{details.message}</p>
          </div>

          {details.recovery.length > 0 && (
            <div>
              <p className="text-xs font-medium uppercase tracking-wide text-destructive/80">
                How to fix
              </p>
              <ol className="mt-1 list-decimal space-y-0.5 pl-4 text-xs text-foreground/80">
                {details.recovery.map((step) => (
                  <li key={step}>{step}</li>
                ))}
              </ol>
            </div>
          )}

          {(onRetry || (installUrl && details.code === "not-installed")) && (
            <div className="flex flex-wrap gap-2 pt-1">
              {installUrl && details.code === "not-installed" && (
                <Button asChild size="xs" variant="outline">
                  <a href={installUrl} target="_blank" rel="noopener noreferrer">
                    Install wallet
                    <ExternalLink className="size-3" aria-hidden="true" />
                  </a>
                </Button>
              )}
              {onRetry && details.retryable && (
                <Button
                  size="xs"
                  variant="outline"
                  onClick={onRetry}
                  disabled={isRetrying}
                  aria-busy={isRetrying}
                >
                  <RotateCw
                    className={cn("size-3", isRetrying && "animate-spin")}
                    aria-hidden="true"
                  />
                  Try again
                </Button>
              )}
            </div>
          )}
        </div>
        {onDismiss && (
          <button
            type="button"
            onClick={onDismiss}
            aria-label="Dismiss error"
            className="rounded p-0.5 text-destructive/70 hover:text-destructive focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
          >
            <X className="size-3.5" aria-hidden="true" />
          </button>
        )}
      </div>
    </div>
  );
}
