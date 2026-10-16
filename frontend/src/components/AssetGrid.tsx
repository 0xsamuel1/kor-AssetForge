'use client'

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { AssetInfo, StellarWallet } from '@/lib/stellar'
import { formatAmount, truncateAddress } from '@/lib/utils'
import { Building2, TrendingUp, Eye } from 'lucide-react'
import Link from 'next/link'
import { CardGridSkeleton } from '@/components/LoadingSkeleton'

interface AssetGridProps {
  assets: AssetInfo[]
  isLoading: boolean
  wallet?: StellarWallet
}

export function AssetGrid({ assets, isLoading, wallet }: AssetGridProps) {
  if (isLoading) {
    return <CardGridSkeleton count={6} label="Loading assets" />
  }

  if (assets.length === 0) {
    return (
      <div className="text-center py-12">
        <Building2 className="h-12 w-12 mx-auto text-muted-foreground mb-4" aria-hidden="true" />
        <h3 className="text-lg font-semibold mb-2">No Assets Available</h3>
        <p className="text-muted-foreground">
          {wallet 
            ? "There are no assets available in the marketplace yet."
            : "Connect your wallet to see available assets."
          }
        </p>
      </div>
    )
  }

  return (
    <ul className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6 list-none p-0" aria-label="Marketplace assets">
      {assets.map((asset) => (
        <li key={asset.id}>
        <Card className="hover:shadow-lg transition-shadow">
          <CardHeader>
            <div className="flex items-start justify-between">
              <div className="flex items-center space-x-2">
                <div className="h-10 w-10 rounded-lg bg-primary/10 flex items-center justify-center" aria-hidden="true">
                  <Building2 className="h-5 w-5 text-primary" />
                </div>
                <div>
                  <CardTitle className="text-lg">{asset.name}</CardTitle>
                  <CardDescription className="text-sm">
                    {asset.code}
                  </CardDescription>
                </div>
              </div>
              <div className="flex items-center text-sm text-muted-foreground">
                <TrendingUp className="h-4 w-4 mr-1" aria-hidden="true" />
                Active
              </div>
            </div>
          </CardHeader>
          
          <CardContent className="space-y-4">
            {asset.description && (
              <p className="text-sm text-muted-foreground line-clamp-2">
                {asset.description}
              </p>
            )}
            
            <div className="space-y-2">
              <div className="flex justify-between text-sm">
                <span className="text-muted-foreground">Total Supply</span>
                <span className="font-medium">
                  {formatAmount(asset.total_supply)} {asset.code}
                </span>
              </div>
              
              <div className="flex justify-between text-sm">
                <span className="text-muted-foreground">Issuer</span>
                <span className="font-mono text-xs">
                  {truncateAddress(asset.issuer)}
                </span>
              </div>
            </div>
            
            <div className="flex space-x-2 pt-2">
              <Link href={`/assets/${asset.id}`} className="flex-1">
                <Button variant="outline" className="w-full">
                  <Eye className="h-4 w-4 mr-2" aria-hidden="true" />
                  View Details
                </Button>
              </Link>
              
              {wallet && (
                <Link href={`/marketplace?asset=${asset.id}`} className="flex-1">
                  <Button className="w-full">
                    Trade
                  </Button>
                </Link>
              )}
            </div>
          </CardContent>
        </Card>
        </li>
      ))}
    </ul>
  )
}
