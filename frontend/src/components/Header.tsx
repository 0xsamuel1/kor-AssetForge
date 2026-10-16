'use client'

import Link from 'next/link'
import { Button } from '@/components/ui/button'
import { stellarService, StellarWallet } from '@/lib/stellar'
import { truncateAddress } from '@/lib/utils'
import { OnboardingTrigger } from '@/components/onboarding/OnboardingTrigger'
import { useState } from 'react'
import { Droplets, Home, Search, Sprout, User, FileText, Wallet, History } from 'lucide-react'

interface HeaderProps {
  wallet?: StellarWallet
  onWalletConnected?: (wallet: StellarWallet) => void
  onWalletDisconnected?: () => void
}

export function Header({ wallet, onWalletConnected, onWalletDisconnected }: HeaderProps) {
  const [isConnecting, setIsConnecting] = useState(false)

  const handleConnect = async () => {
    setIsConnecting(true)
    try {
      const w = await stellarService.connectWallet()
      onWalletConnected?.(w)
    } catch {
      // ignore
    } finally {
      setIsConnecting(false)
    }
  }

  const handleDisconnect = async () => {
    try {
      await stellarService.disconnectWallet()
      onWalletDisconnected?.()
    } catch {
      // ignore
    }
  }

  return (
    <header className="border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
      <div className="container mx-auto px-4">
        <div className="flex h-16 items-center justify-between">
          <div className="flex items-center space-x-6">
            <Link href="/" className="flex items-center space-x-2" aria-label="kor-AssetForge home">
              <div className="h-8 w-8 rounded bg-primary flex items-center justify-center" aria-hidden="true">
                <span className="text-primary-foreground font-bold text-sm">K</span>
              </div>
              <span className="font-bold text-xl">kor-AssetForge</span>
            </Link>

            <nav className="hidden md:flex items-center space-x-6" aria-label="Main navigation">
              <Link href="/" className="flex items-center space-x-1 text-sm font-medium hover:text-primary">
                <Home className="h-4 w-4" aria-hidden="true" />
                <span>Home</span>
              </Link>
              <Link href="/marketplace" className="flex items-center space-x-1 text-sm font-medium hover:text-primary">
                <Search className="h-4 w-4" aria-hidden="true" />
                <span>Marketplace</span>
              </Link>
              <Link href="/liquidity" className="flex items-center space-x-1 text-sm font-medium hover:text-primary">
                <Droplets className="h-4 w-4" />
                <span>Liquidity</span>
              </Link>
              <Link href="/staking" className="flex items-center space-x-1 text-sm font-medium hover:text-primary">
                <Sprout className="h-4 w-4" />
                <span>Staking</span>
              </Link>
              {wallet && (
                <>
                  <Link href="/dashboard" className="flex items-center space-x-1 text-sm font-medium hover:text-primary">
                    <User className="h-4 w-4" aria-hidden="true" />
                    <span>Dashboard</span>
                  </Link>
                  <Link href="/transactions" className="flex items-center space-x-1 text-sm font-medium hover:text-primary">
                    <History className="h-4 w-4" aria-hidden="true" />
                    <span>Transactions</span>
                  </Link>
                  <Link href="/kyc" className="flex items-center space-x-1 text-sm font-medium hover:text-primary">
                    <FileText className="h-4 w-4" aria-hidden="true" />
                    <span>KYC</span>
                  </Link>
                </>
              )}
            </nav>
          </div>

          <div className="flex items-center space-x-2">
            <OnboardingTrigger />
            {wallet ? (
              <div className="flex items-center space-x-2">
                <div className="hidden md:flex items-center space-x-2 px-3 py-1 bg-muted rounded-lg">
                  <Wallet className="h-4 w-4 text-muted-foreground" />
                  <span className="text-sm font-mono">
                    {truncateAddress(wallet.publicKey)}
                  </span>
                </div>
                <Button variant="outline" size="sm" onClick={handleDisconnect}>
                  Disconnect
                </Button>
              </div>
            ) : (
              <Button size="sm" onClick={handleConnect} disabled={isConnecting}>
                {isConnecting ? 'Connecting...' : 'Connect Wallet'}
              </Button>
            )}
          </div>
        </div>
      </div>
    </header>
  )
}
