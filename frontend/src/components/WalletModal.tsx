"use client";

import { useCallback, useEffect, useState } from "react";
import {
  Check,
  Copy,
  ExternalLink,
  Loader2,
  LogOut,
  RefreshCw,
  Wallet,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Skeleton } from "@/components/ui/skeleton";
import { WalletErrorAlert } from "@/components/WalletErrorAlert";
import { toast } from "@/lib/toast";
import { cn, truncateAddress } from "@/lib/utils";
import { getWalletErrorDetails, toWalletError } from "@/lib/wallet/errors";
import {
  connectWallet,
  disconnectWallet,
  getWalletProvider,
  WALLET_PROVIDERS,
  detectWalletProviders,
  type ConnectedWallet,
  type WalletProvider,
  type WalletProviderId,
} from "@/lib/wallet-providers";

interface WalletModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Fired once a provider hands back a public key. */
  onConnected: (wallet: ConnectedWallet) => void;
  /** Fired after the wallet is disconnected from the connected view. */
  onDisconnected?: () => void;
  /** Present when a wallet is already connected — switches to the account view. */
  wallet?: ConnectedWallet;
}

/** Square brand badge — avoids shipping remote logo images. */
function ProviderBadge({ provider }: { provider: WalletProvider }) {
  return (
    <span
      className={cn(
        "flex size-9 shrink-0 items-center justify-center rounded-lg text-xs font-semibold",
        provider.badgeClassName,
      )}
      aria-hidden="true"
    >
      {provider.initials}
    </span>
  );
}

function ProviderRow({
  provider,
  isAvailable,
  isConnecting,
  isDisabled,
  onConnect,
}: {
  provider: WalletProvider;
  isAvailable: boolean;
  isConnecting: boolean;
  isDisabled: boolean;
  onConnect: () => void;
}) {
  // Web wallets (Albedo) work through a popup, so "not detected" means the
  // page hasn't loaded their script rather than "install an extension".
  const installLabel =
    provider.kind === "web" ? "Open site" : "Install";

  if (!isAvailable) {
    return (
      <a
        href={provider.installUrl}
        target="_blank"
        rel="noopener noreferrer"
        className="flex items-center gap-3 rounded-lg p-3 text-left ring-1 ring-foreground/10 transition-colors hover:bg-muted focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/50"
      >
        <ProviderBadge provider={provider} />
        <span className="flex-1 min-w-0">
          <span className="block text-sm font-medium">{provider.name}</span>
          <span className="block truncate text-xs text-muted-foreground">
            Not detected — {installLabel.toLowerCase()} to continue
          </span>
        </span>
        <span className="flex shrink-0 items-center gap-1 text-xs font-medium text-muted-foreground">
          {installLabel}
          <ExternalLink className="size-3.5" aria-hidden="true" />
        </span>
      </a>
    );
  }

  return (
    <button
      type="button"
      onClick={onConnect}
      disabled={isDisabled}
      aria-busy={isConnecting}
      className="flex items-center gap-3 rounded-lg p-3 text-left ring-1 ring-foreground/10 transition-colors hover:bg-muted focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/50 disabled:pointer-events-none disabled:opacity-60"
    >
      <ProviderBadge provider={provider} />
      <span className="flex-1 min-w-0">
        <span className="block text-sm font-medium">{provider.name}</span>
        <span className="block truncate text-xs text-muted-foreground">
          {provider.description}
        </span>
      </span>
      {isConnecting ? (
        <Loader2 className="size-4 shrink-0 animate-spin text-muted-foreground" aria-hidden="true" />
      ) : (
        <span className="flex shrink-0 items-center gap-1.5 text-xs font-medium text-emerald-600 dark:text-emerald-400">
          <span className="size-1.5 rounded-full bg-current" aria-hidden="true" />
          Detected
        </span>
      )}
    </button>
  );
}

/**
 * Multi-provider wallet picker.
 *
 * Detects which Stellar wallets are usable in the current browser, connects
 * through the chosen one, and doubles as the account sheet (address, copy,
 * disconnect) once a wallet is connected.
 */
export function WalletModal({
  open,
  onOpenChange,
  onConnected,
  onDisconnected,
  wallet,
}: WalletModalProps) {
  const [availability, setAvailability] = useState<Record<
    WalletProviderId,
    boolean
  > | null>(null);
  const [connectingId, setConnectingId] = useState<WalletProviderId | null>(null);
  const [error, setError] = useState<{
    providerId: WalletProviderId;
    error: unknown;
  } | null>(null);
  const [hasCopied, setHasCopied] = useState(false);

  /** Manual re-scan from the footer button. */
  const detect = useCallback(async () => {
    setAvailability(null);
    setAvailability(await detectWalletProviders());
  }, []);

  // Re-probe every time the modal opens: extensions can be installed or
  // unlocked while the tab stays open. Previous results stay on screen until
  // the new ones land, so re-opening doesn't flash skeletons.
  useEffect(() => {
    if (!open || wallet) return;

    let cancelled = false;
    detectWalletProviders().then((result) => {
      if (cancelled) return;
      setAvailability(result);
      setError(null);
    });

    return () => {
      cancelled = true;
    };
  }, [open, wallet]);

  const handleConnect = async (providerId: WalletProviderId) => {
    setConnectingId(providerId);
    setError(null);

    try {
      const connected = await connectWallet(providerId);
      onConnected(connected);
      onOpenChange(false);
      toast.success(`${connected.providerName} connected`, {
        description: truncateAddress(connected.publicKey),
      });
    } catch (err) {
      const walletError = toWalletError(providerId, err, "connect");
      const details = getWalletErrorDetails(walletError);
      setError({ providerId, error: walletError });
      toast.error(details.title, { description: details.message });
    } finally {
      setConnectingId(null);
    }
  };

  const handleDisconnect = async () => {
    await disconnectWallet(wallet?.providerId ?? null);
    onDisconnected?.();
    onOpenChange(false);
    toast.info("Wallet disconnected");
  };

  const handleCopy = async () => {
    if (!wallet) return;

    try {
      await navigator.clipboard.writeText(wallet.publicKey);
      setHasCopied(true);
      window.setTimeout(() => setHasCopied(false), 2_000);
    } catch {
      toast.error("Could not copy the address");
    }
  };

  const detectedCount = availability
    ? Object.values(availability).filter(Boolean).length
    : 0;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        {wallet ? (
          // ── Connected account view ──────────────────────────────────────
          <>
            <DialogHeader>
              <DialogTitle className="flex items-center gap-2">
                <Wallet className="size-4" aria-hidden="true" />
                {wallet.providerName} connected
              </DialogTitle>
              <DialogDescription>
                Your Stellar account is linked to kor-AssetForge.
              </DialogDescription>
            </DialogHeader>

            <div className="flex items-center gap-2 rounded-lg bg-muted p-3">
              <p className="flex-1 truncate font-mono text-sm" title={wallet.publicKey}>
                {truncateAddress(wallet.publicKey, 10, 6)}
              </p>
              <Button
                variant="ghost"
                size="icon-sm"
                onClick={handleCopy}
                aria-label="Copy wallet address"
              >
                {hasCopied ? (
                  <Check className="size-3.5 text-emerald-600 dark:text-emerald-400" aria-hidden="true" />
                ) : (
                  <Copy className="size-3.5" aria-hidden="true" />
                )}
              </Button>
            </div>

            <Button variant="outline" className="w-full" onClick={handleDisconnect}>
              <LogOut className="size-4" aria-hidden="true" />
              Disconnect
            </Button>
          </>
        ) : (
          // ── Provider picker ─────────────────────────────────────────────
          <>
            <DialogHeader>
              <DialogTitle className="flex items-center gap-2">
                <Wallet className="size-4" aria-hidden="true" />
                Connect a wallet
              </DialogTitle>
              <DialogDescription>
                Choose a Stellar wallet to sign in. Your keys never leave the
                wallet.
              </DialogDescription>
            </DialogHeader>

            {error && (
              <WalletErrorAlert
                error={error.error}
                providerId={error.providerId}
                installUrl={getWalletProvider(error.providerId)?.installUrl}
                onRetry={() => handleConnect(error.providerId)}
                onDismiss={() => setError(null)}
                isRetrying={connectingId === error.providerId}
              />
            )}

            <div className="flex flex-col gap-2">
              {availability === null
                ? // Loading: one skeleton row per provider.
                  WALLET_PROVIDERS.map((provider) => (
                    <div
                      key={provider.id}
                      className="flex items-center gap-3 rounded-lg p-3 ring-1 ring-foreground/10"
                    >
                      <Skeleton className="size-9 rounded-lg" />
                      <div className="flex-1 space-y-2">
                        <Skeleton className="h-3 w-24" />
                        <Skeleton className="h-3 w-40" />
                      </div>
                    </div>
                  ))
                : WALLET_PROVIDERS.map((provider) => (
                    <ProviderRow
                      key={provider.id}
                      provider={provider}
                      isAvailable={availability[provider.id]}
                      isConnecting={connectingId === provider.id}
                      isDisabled={connectingId !== null}
                      onConnect={() => handleConnect(provider.id)}
                    />
                  ))}
            </div>

            <div className="flex items-center justify-between gap-2 text-xs text-muted-foreground">
              <span>
                {availability === null
                  ? "Detecting wallets…"
                  : `${detectedCount} of ${WALLET_PROVIDERS.length} wallets detected`}
              </span>
              <Button
                variant="ghost"
                size="xs"
                onClick={detect}
                disabled={availability === null || connectingId !== null}
              >
                <RefreshCw className="size-3" aria-hidden="true" />
                Re-scan
              </Button>
            </div>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
