"use client"

import { useCallback, ElementType } from "react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { InfiniteScroll } from "@/components/InfiniteScroll"
import { TransactionEntry } from "@/lib/asset-api"
import { cn, formatCurrency, formatDate, truncateAddress } from "@/lib/utils"
import {
  ArrowDownLeft,
  ArrowUpRight,
  ArrowLeftRight,
  Flame,
  Sparkles,
  Lock,
  Unlock,
  Gift,
  Download,
  ExternalLink,
  Loader2,
} from "lucide-react"

const TYPE_CONFIG: Record<
  TransactionEntry["type"],
  { label: string; icon: ElementType; color: string }
> = {
  buy: { label: "Buy", icon: ArrowDownLeft, color: "text-green-600 dark:text-green-400" },
  sell: { label: "Sell", icon: ArrowUpRight, color: "text-red-600 dark:text-red-400" },
  transfer: { label: "Transfer", icon: ArrowLeftRight, color: "text-blue-600 dark:text-blue-400" },
  mint: { label: "Mint", icon: Sparkles, color: "text-purple-600 dark:text-purple-400" },
  burn: { label: "Burn", icon: Flame, color: "text-orange-600 dark:text-orange-400" },
  stake: { label: "Stake", icon: Lock, color: "text-indigo-600 dark:text-indigo-400" },
  unstake: { label: "Unstake", icon: Unlock, color: "text-cyan-600 dark:text-cyan-400" },
  dividend: { label: "Dividend", icon: Gift, color: "text-emerald-600 dark:text-emerald-400" },
}

const STATUS_VARIANT: Record<
  TransactionEntry["status"],
  "default" | "secondary" | "destructive" | "outline"
> = {
  completed: "default",
  pending: "secondary",
  failed: "destructive",
}

interface TransactionListProps {
  transactions: TransactionEntry[]
  isLoading: boolean
  hasMore: boolean
  onLoadMore: () => Promise<void>
  onDownloadReceipt: (tx: TransactionEntry) => void
  downloadingId: string | null
  className?: string
}

export function TransactionList({
  transactions,
  isLoading,
  hasMore,
  onLoadMore,
  onDownloadReceipt,
  downloadingId,
  className,
}: TransactionListProps) {
  const handleLoadMore = useCallback(async () => {
    await onLoadMore()
  }, [onLoadMore])

  if (!isLoading && transactions.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-20 text-center">
        <ArrowLeftRight className="h-10 w-10 text-muted-foreground mb-3" aria-hidden="true" />
        <p className="text-muted-foreground text-sm">No transactions found</p>
        <p className="text-muted-foreground text-xs mt-1">Try adjusting your filters or date range</p>
      </div>
    )
  }

  return (
    <InfiniteScroll
      loadMore={handleLoadMore}
      hasMore={hasMore}
      isLoading={isLoading}
      endMessage="All transactions loaded"
      className={className}
    >
      {/* Table header — hidden on mobile */}
      <div
        className="hidden md:grid gap-4 px-4 py-2 text-xs font-medium text-muted-foreground border-b"
        style={{ gridTemplateColumns: "2fr 1fr 1.5fr 1fr 1fr auto" }}
        aria-hidden="true"
      >
        <span>Asset / Hash</span>
        <span>Type</span>
        <span>Date</span>
        <span className="text-right">Amount</span>
        <span className="text-center">Status</span>
        <span />
      </div>

      <ul aria-label="Transaction history">
        {transactions.map((tx) => {
          const cfg = TYPE_CONFIG[tx.type]
          const Icon = cfg.icon
          const isDownloading = downloadingId === tx.id

          return (
            <li
              key={tx.id}
              className="flex flex-col md:grid gap-3 md:gap-4 px-4 py-3 border-b last:border-b-0 hover:bg-muted/30 transition-colors"
              style={{ gridTemplateColumns: "2fr 1fr 1.5fr 1fr 1fr auto" }}
            >
              {/* Asset / Hash */}
              <div className="min-w-0">
                <p className="text-sm font-medium truncate">
                  {tx.assetName || tx.assetId}
                  {tx.assetSymbol && (
                    <span className="text-muted-foreground ml-1 text-xs">({tx.assetSymbol})</span>
                  )}
                </p>
                <a
                  href={`https://stellar.expert/explorer/public/tx/${tx.txHash}`}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-xs text-muted-foreground hover:text-primary flex items-center gap-0.5 mt-0.5"
                  aria-label={`View transaction ${tx.txHash} on Stellar Expert`}
                >
                  {truncateAddress(tx.txHash, 8, 6)}
                  <ExternalLink className="h-3 w-3 shrink-0" aria-hidden="true" />
                </a>
              </div>

              {/* Type */}
              <div className="flex items-center gap-1.5 md:self-center">
                <Icon className={cn("h-4 w-4 shrink-0", cfg.color)} aria-hidden="true" />
                <span className={cn("text-sm font-medium", cfg.color)}>{cfg.label}</span>
              </div>

              {/* Date */}
              <div className="md:self-center">
                <p className="text-sm text-muted-foreground">{formatDate(tx.timestamp)}</p>
                <p className="text-xs text-muted-foreground/60 mt-0.5 hidden md:block">
                  From: {truncateAddress(tx.from)} → {truncateAddress(tx.to)}
                </p>
              </div>

              {/* Amount */}
              <div className="md:self-center md:text-right">
                <p className="text-sm font-semibold tabular-nums">
                  {tx.price
                    ? formatCurrency(tx.amount * tx.price)
                    : `${tx.amount.toLocaleString()} units`}
                </p>
                {tx.fee !== undefined && tx.fee > 0 && (
                  <p className="text-xs text-muted-foreground mt-0.5">
                    Fee: {formatCurrency(tx.fee)}
                  </p>
                )}
              </div>

              {/* Status */}
              <div className="flex items-center md:justify-center">
                <Badge variant={STATUS_VARIANT[tx.status]} className="capitalize text-xs">
                  {tx.status}
                </Badge>
              </div>

              {/* Actions */}
              <div className="flex items-center gap-1">
                <Button
                  variant="ghost"
                  size="icon-sm"
                  onClick={() => onDownloadReceipt(tx)}
                  disabled={isDownloading}
                  aria-label={`Download receipt for transaction ${tx.id}`}
                  title="Download receipt"
                >
                  {isDownloading ? (
                    <Loader2 className="h-3.5 w-3.5 animate-spin" aria-hidden="true" />
                  ) : (
                    <Download className="h-3.5 w-3.5" aria-hidden="true" />
                  )}
                </Button>
              </div>
            </li>
          )
        })}
      </ul>

      {isLoading && transactions.length === 0 && (
        <div className="flex items-center justify-center py-12">
          <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" aria-hidden="true" />
        </div>
      )}
    </InfiniteScroll>
  )
}
