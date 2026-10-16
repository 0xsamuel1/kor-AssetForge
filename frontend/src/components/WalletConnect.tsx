'use client'

import { useCallback, useEffect, useState } from 'react'
import { Wallet } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { WalletErrorAlert } from '@/components/WalletErrorAlert'
import { WalletModal } from '@/components/WalletModal'
import { StellarWallet } from '@/lib/stellar'
import { toast } from '@/lib/toast'
import { truncateAddress } from '@/lib/utils'
import { getWalletErrorDetails, type WalletError } from '@/lib/wallet/errors'
import { diagnoseFreighter } from '@/lib/wallet/freighter'
import {
  getStoredProviderId,
  getWalletProvider,
  restoreWalletConnection,
  type ConnectedWallet,
  type WalletProviderId,
} from '@/lib/wallet-providers'

interface WalletConnectProps {
  onWalletConnected: (wallet: StellarWallet) => void
  onWalletDisconnected: () => void
  wallet?: StellarWallet
  /** `card` (default) matches the original layout; `button` is a compact trigger. */
  variant?: 'card' | 'button'
  /** Silently restore the previous session on mount (default: true). */
  autoReconnect?: boolean
  className?: string
}

/**
 * Wallet entry point. Opens the multi-provider {@link WalletModal} instead of
 * talking to a single wallet, and restores the previous session on mount.
 *
 * The props are unchanged from the original Freighter-only version, so existing
 * pages keep working as-is.
 */
export function WalletConnect({
  onWalletConnected,
  onWalletDisconnected,
  wallet,
  variant = 'card',
  autoReconnect = true,
  className,
}: WalletConnectProps) {
  const [isModalOpen, setIsModalOpen] = useState(false)
  const [providerId, setProviderId] = useState<WalletProviderId | null>(null)
  // Starts true when a restore attempt is about to run, so the button never
  // flashes "Connect Wallet" before the previous session comes back.
  const [isRestoring, setIsRestoring] = useState(
    () => autoReconnect && !wallet?.connected,
  )
  // Why the previous session couldn't be restored (wallet locked, wrong
  // network), so the user knows what to fix before reconnecting.
  const [restoreError, setRestoreError] = useState<WalletError | null>(null)

  const handleConnected = useCallback(
    (connected: ConnectedWallet) => {
      setRestoreError(null)
      setProviderId(connected.providerId)
      onWalletConnected(connected)
    },
    [onWalletConnected],
  )

  // One-click return: if the user connected before and the wallet is still
  // available, reconnect without showing the modal. Also recovers which
  // provider was used, since localStorage is unavailable during SSR.
  useEffect(() => {
    if (!autoReconnect || wallet?.connected) return

    let cancelled = false

    restoreWalletConnection()
      .then(async (restored) => {
        if (cancelled) return
        if (restored) {
          handleConnected(restored)
          return
        }

        const storedId = getStoredProviderId()
        setProviderId(storedId)
        if (storedId !== 'freighter') return

        const problem = await diagnoseFreighter()
        if (cancelled || !problem) return
        setRestoreError(problem)
        if (variant === 'button') {
          // The compact trigger has no room for the full alert.
          const details = getWalletErrorDetails(problem)
          toast.warning(details.title, { description: details.recovery[0] })
        }
      })
      .catch(() => {
        // Restoring is best-effort; the user can still connect manually.
      })
      .finally(() => {
        if (!cancelled) setIsRestoring(false)
      })

    return () => {
      cancelled = true
    }
    // Runs once on mount; reconnecting on every prop change would re-prompt
    // the user.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const handleDisconnected = useCallback(() => {
    setProviderId(null)
    onWalletDisconnected()
  }, [onWalletDisconnected])

  const activeWallet: ConnectedWallet | undefined = wallet?.connected
    ? {
        publicKey: wallet.publicKey,
        connected: true,
        providerId: providerId ?? 'freighter',
        providerName: getWalletProvider(providerId ?? 'freighter')?.name ?? 'Wallet',
      }
    : undefined

  const modal = (
    <WalletModal
      open={isModalOpen}
      onOpenChange={setIsModalOpen}
      onConnected={handleConnected}
      onDisconnected={handleDisconnected}
      wallet={activeWallet}
    />
  )

  if (variant === 'button') {
    return (
      <>
        <Button
          onClick={() => setIsModalOpen(true)}
          variant={activeWallet ? 'outline' : 'default'}
          className={className}
          disabled={isRestoring && !activeWallet}
          aria-busy={isRestoring}
        >
          <Wallet className="h-4 w-4" aria-hidden="true" />
          {activeWallet
            ? truncateAddress(activeWallet.publicKey)
            : isRestoring
              ? 'Reconnecting…'
              : 'Connect Wallet'}
        </Button>
        {modal}
      </>
    )
  }

  if (activeWallet) {
    return (
      <>
        <Card className={className ?? 'w-full max-w-md'}>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Wallet className="h-5 w-5" aria-hidden="true" />
              Wallet Connected
            </CardTitle>
            <CardDescription>
              Connected with {activeWallet.providerName} and ready to sign
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="p-3 bg-muted rounded-lg">
              <p className="text-sm font-mono">
                {truncateAddress(activeWallet.publicKey)}
              </p>
            </div>
            <Button
              onClick={() => setIsModalOpen(true)}
              variant="outline"
              className="w-full"
            >
              Manage wallet
            </Button>
          </CardContent>
        </Card>
        {modal}
      </>
    )
  }

  return (
    <>
      <Card className={className ?? 'w-full max-w-md'}>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Wallet className="h-5 w-5" aria-hidden="true" />
            Connect Wallet
          </CardTitle>
          <CardDescription>
            Connect a Stellar wallet — Freighter, Albedo, LOBSTR, xBull or Rabet
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {restoreError && (
            <WalletErrorAlert
              error={restoreError}
              onRetry={() => {
                setRestoreError(null)
                setIsModalOpen(true)
              }}
              onDismiss={() => setRestoreError(null)}
            />
          )}
          <Button
            onClick={() => setIsModalOpen(true)}
            className="w-full"
            disabled={isRestoring}
            aria-busy={isRestoring}
          >
            <Wallet className="h-4 w-4" aria-hidden="true" />
            {isRestoring ? 'Reconnecting…' : 'Connect Wallet'}
          </Button>
        </CardContent>
      </Card>
      {modal}
    </>
  )
}
