"use client"

import { useState, useEffect, useCallback, useMemo } from "react"
import { Header } from "@/components/Header"
import { TransactionFilter, TransactionFilters } from "@/components/TransactionFilter"
import { TransactionList } from "@/components/TransactionList"
import { Pagination } from "@/components/Pagination"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { assetApi, TransactionEntry } from "@/lib/asset-api"
import { cn } from "@/lib/utils"
import {
  Search,
  X,
  Download,
  FileText,
  Filter,
  Loader2,
  TrendingUp,
} from "lucide-react"

const PAGE_SIZE = 20
const DEMO_TYPES = ["buy", "sell", "transfer", "mint", "burn", "stake", "unstake", "dividend"] as const
const DEMO_STATUSES = ["completed", "completed", "completed", "pending", "failed"] as const
const DEMO_ASSETS = [
  { id: "RWA001", name: "Downtown Office Complex", symbol: "DOC" },
  { id: "RWA002", name: "Residential Tower A", symbol: "RTA" },
  { id: "RWA003", name: "Industrial Park B", symbol: "IPB" },
  { id: "RWA004", name: "Retail Plaza Fund", symbol: "RPF" },
  { id: "RWA005", name: "Green Energy Token", symbol: "GET" },
]

function generateDemoTransactions(count: number, offset: number = 0): TransactionEntry[] {
  return Array.from({ length: count }, (_, i) => {
    const idx = offset + i
    const asset = DEMO_ASSETS[idx % DEMO_ASSETS.length]
    const type = DEMO_TYPES[idx % DEMO_TYPES.length]
    const status = DEMO_STATUSES[idx % DEMO_STATUSES.length]
    const amount = Math.round((1 + (idx * 7.3) % 999) * 100) / 100
    const price = Math.round((50 + (idx * 13.7) % 950) * 100) / 100
    const daysAgo = idx * 0.8
    const timestamp = Math.floor((Date.now() - daysAgo * 86400000) / 1000)
    return {
      id: `tx-${idx + 1}`,
      type,
      from: `GABCD${String(idx).padStart(4, "0")}EFGH${String(idx * 3).padStart(4, "0")}IJKL`,
      to: `GXYZ${String(idx + 1).padStart(4, "0")}MNOP${String(idx * 2).padStart(4, "0")}QRST`,
      amount,
      price,
      fee: Math.round(amount * 0.001 * price * 100) / 100,
      assetId: asset.id,
      assetName: asset.name,
      assetSymbol: asset.symbol,
      timestamp,
      txHash: `${Array.from({ length: 64 }, (_, k) => "0123456789abcdef"[(idx * 7 + k * 13) % 16]).join("")}`,
      status,
    }
  })
}

const TOTAL_DEMO = 87

function filterDemo(txs: TransactionEntry[], filters: TransactionFilters, search: string) {
  return txs.filter((tx) => {
    if (search) {
      const q = search.toLowerCase()
      if (
        !tx.assetName?.toLowerCase().includes(q) &&
        !tx.assetSymbol?.toLowerCase().includes(q) &&
        !tx.txHash.includes(q) &&
        !tx.id.includes(q)
      ) return false
    }
    if (filters.type && tx.type !== filters.type) return false
    if (filters.status && tx.status !== filters.status) return false
    if (filters.startDate) {
      const start = new Date(filters.startDate).getTime() / 1000
      if (tx.timestamp < start) return false
    }
    if (filters.endDate) {
      const end = new Date(filters.endDate).getTime() / 1000 + 86400
      if (tx.timestamp > end) return false
    }
    const value = tx.amount * (tx.price ?? 1)
    if (filters.minAmount && value < parseFloat(filters.minAmount)) return false
    if (filters.maxAmount && value > parseFloat(filters.maxAmount)) return false
    return true
  })
}

function generateReceiptContent(tx: TransactionEntry): string {
  const lines = [
    "TRANSACTION RECEIPT",
    "===================",
    `ID:          ${tx.id}`,
    `Type:        ${tx.type.toUpperCase()}`,
    `Status:      ${tx.status.toUpperCase()}`,
    `Asset:       ${tx.assetName ?? tx.assetId}${tx.assetSymbol ? ` (${tx.assetSymbol})` : ""}`,
    `Amount:      ${tx.amount.toLocaleString()} units`,
    tx.price ? `Price:       $${tx.price.toFixed(2)} / unit` : "",
    tx.price ? `Total Value: $${(tx.amount * tx.price).toFixed(2)}` : "",
    tx.fee ? `Fee:         $${tx.fee.toFixed(2)}` : "",
    `From:        ${tx.from}`,
    `To:          ${tx.to}`,
    `Tx Hash:     ${tx.txHash}`,
    `Date:        ${new Date(tx.timestamp * 1000).toISOString()}`,
    "",
    "kor-AssetForge — Stellar Real World Asset Platform",
  ].filter(Boolean)
  return lines.join("\n")
}

function exportToCsv(transactions: TransactionEntry[], filename: string) {
  const headers = ["ID", "Type", "Status", "Asset", "Symbol", "Amount", "Price (USD)", "Total Value", "Fee", "From", "To", "Tx Hash", "Date"]
  const rows = transactions.map((tx) => [
    tx.id,
    tx.type,
    tx.status,
    tx.assetName ?? tx.assetId,
    tx.assetSymbol ?? "",
    tx.amount,
    tx.price ?? "",
    tx.price ? (tx.amount * tx.price).toFixed(2) : "",
    tx.fee ?? "",
    tx.from,
    tx.to,
    tx.txHash,
    new Date(tx.timestamp * 1000).toISOString(),
  ])
  const csv = [headers, ...rows]
    .map((row) => row.map((cell) => `"${String(cell).replace(/"/g, '""')}"`).join(","))
    .join("\n")
  const blob = new Blob([csv], { type: "text/csv;charset=utf-8;" })
  triggerDownload(blob, filename)
}

function exportToPdf(transactions: TransactionEntry[]) {
  const win = window.open("", "_blank")
  if (!win) return
  const rows = transactions
    .map(
      (tx) => `
      <tr>
        <td>${tx.id}</td>
        <td>${tx.type}</td>
        <td>${tx.assetName ?? tx.assetId}</td>
        <td>${tx.amount.toLocaleString()}</td>
        <td>${tx.price ? `$${(tx.amount * tx.price).toFixed(2)}` : "—"}</td>
        <td class="status-${tx.status}">${tx.status}</td>
        <td>${new Date(tx.timestamp * 1000).toLocaleDateString()}</td>
      </tr>`
    )
    .join("")
  win.document.write(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <title>Transaction History — kor-AssetForge</title>
  <style>
    body { font-family: system-ui, sans-serif; font-size: 12px; margin: 24px; color: #111; }
    h1 { font-size: 18px; margin-bottom: 4px; }
    p.sub { color: #666; margin-bottom: 16px; }
    table { width: 100%; border-collapse: collapse; }
    th { background: #f4f4f5; text-align: left; padding: 6px 8px; font-size: 11px; border-bottom: 2px solid #e4e4e7; }
    td { padding: 6px 8px; border-bottom: 1px solid #e4e4e7; vertical-align: top; }
    .status-completed { color: #16a34a; font-weight: 600; }
    .status-pending { color: #d97706; font-weight: 600; }
    .status-failed { color: #dc2626; font-weight: 600; }
    @media print { button { display: none; } }
  </style>
</head>
<body>
  <h1>Transaction History</h1>
  <p class="sub">Generated ${new Date().toLocaleString()} — kor-AssetForge</p>
  <table>
    <thead>
      <tr>
        <th>ID</th><th>Type</th><th>Asset</th><th>Amount</th><th>Value</th><th>Status</th><th>Date</th>
      </tr>
    </thead>
    <tbody>${rows}</tbody>
  </table>
  <script>window.onload = () => { window.print(); }<\/script>
</body>
</html>`)
  win.document.close()
}

function triggerDownload(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob)
  const a = document.createElement("a")
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
}

const DEFAULT_FILTERS: TransactionFilters = {
  search: "",
  type: "",
  status: "",
  startDate: "",
  endDate: "",
  minAmount: "",
  maxAmount: "",
}

export default function TransactionsPage() {
  const [filters, setFilters] = useState<TransactionFilters>(DEFAULT_FILTERS)
  const [search, setSearch] = useState("")
  const [searchInput, setSearchInput] = useState("")
  const [sidebarOpen, setSidebarOpen] = useState(false)

  const [allDemo] = useState(() => generateDemoTransactions(TOTAL_DEMO))
  const [transactions, setTransactions] = useState<TransactionEntry[]>([])
  const [total, setTotal] = useState(0)
  const [hasMore, setHasMore] = useState(false)
  const [isLoading, setIsLoading] = useState(false)
  const [page, setPage] = useState(1)

  // infinite scroll state
  const [infiniteItems, setInfiniteItems] = useState<TransactionEntry[]>([])
  const [infiniteCursor, setInfiniteCursor] = useState(0)
  const [infiniteHasMore, setInfiniteHasMore] = useState(true)
  const [infiniteLoading, setInfiniteLoading] = useState(false)
  const [useInfinite] = useState(false) // toggle: true = infinite scroll, false = pagination

  const [downloadingId, setDownloadingId] = useState<string | null>(null)
  const [exportingCsv, setExportingCsv] = useState(false)

  const filteredAll = useMemo(
    () => filterDemo(allDemo, filters, search),
    [allDemo, filters, search]
  )

  useEffect(() => {
    setPage(1)
  }, [filters, search])

  useEffect(() => {
    const start = (page - 1) * PAGE_SIZE
    const slice = filteredAll.slice(start, start + PAGE_SIZE)
    setTransactions(slice)
    setTotal(filteredAll.length)
    setHasMore(start + PAGE_SIZE < filteredAll.length)
  }, [page, filteredAll])

  // Infinite scroll bootstrap
  useEffect(() => {
    if (!useInfinite) return
    const slice = filteredAll.slice(0, PAGE_SIZE)
    setInfiniteItems(slice)
    setInfiniteCursor(slice.length)
    setInfiniteHasMore(slice.length < filteredAll.length)
  }, [filteredAll, useInfinite])

  const loadMore = useCallback(async () => {
    if (infiniteLoading || !infiniteHasMore) return
    setInfiniteLoading(true)
    await new Promise((r) => setTimeout(r, 400))
    const next = filteredAll.slice(infiniteCursor, infiniteCursor + PAGE_SIZE)
    setInfiniteItems((prev) => [...prev, ...next])
    setInfiniteCursor((c) => c + next.length)
    setInfiniteHasMore(infiniteCursor + next.length < filteredAll.length)
    setInfiniteLoading(false)
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [infiniteLoading, infiniteHasMore, infiniteCursor, filteredAll])

  const handleSearch = (e: { preventDefault: () => void }) => {
    e.preventDefault()
    setSearch(searchInput)
  }

  const handleDownloadReceipt = useCallback(async (tx: TransactionEntry) => {
    setDownloadingId(tx.id)
    try {
      // Try real API first; fall back to generated receipt
      let blob: Blob
      try {
        blob = await assetApi.getTransactionReceipt(tx.id)
      } catch {
        const content = generateReceiptContent(tx)
        blob = new Blob([content], { type: "text/plain;charset=utf-8" })
      }
      triggerDownload(blob, `receipt-${tx.id}.txt`)
    } finally {
      setDownloadingId(null)
    }
  }, [])

  const handleExportCsv = async () => {
    setExportingCsv(true)
    try {
      // Try real API first; fall back to client-side generation
      try {
        const blob = await assetApi.exportTransactionsCsv(
          undefined,
          filters.startDate || undefined,
          filters.endDate || undefined,
        )
        triggerDownload(blob, "transactions.csv")
      } catch {
        exportToCsv(filteredAll, "transactions.csv")
      }
    } finally {
      setExportingCsv(false)
    }
  }

  const handleExportPdf = () => {
    exportToPdf(filteredAll)
  }

  const displayedTxs = useInfinite ? infiniteItems : transactions
  const totalPages = Math.ceil(total / PAGE_SIZE)

  return (
    <div className="min-h-screen bg-background">
      <Header />

      <main className="container mx-auto px-4 py-8">
        {/* Page Header */}
        <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 mb-8">
          <div>
            <h1 className="text-3xl font-bold mb-1 flex items-center gap-2">
              <TrendingUp className="h-7 w-7" aria-hidden="true" />
              Transaction History
            </h1>
            <p className="text-muted-foreground text-sm">
              {total.toLocaleString()} transaction{total !== 1 ? "s" : ""} found
            </p>
          </div>

          <div className="flex items-center gap-2 flex-wrap">
            <Button
              variant="outline"
              size="sm"
              onClick={handleExportCsv}
              disabled={exportingCsv || filteredAll.length === 0}
              aria-label="Export transactions to CSV"
            >
              {exportingCsv ? (
                <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />
              ) : (
                <Download className="h-4 w-4" aria-hidden="true" />
              )}
              Export CSV
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={handleExportPdf}
              disabled={filteredAll.length === 0}
              aria-label="Export transactions to PDF"
            >
              <FileText className="h-4 w-4" aria-hidden="true" />
              Export PDF
            </Button>
            <Button
              variant="outline"
              size="sm"
              className="md:hidden"
              onClick={() => setSidebarOpen((o) => !o)}
              aria-expanded={sidebarOpen}
              aria-label="Toggle filters"
            >
              <Filter className="h-4 w-4" aria-hidden="true" />
              Filters
            </Button>
          </div>
        </div>

        {/* Search Bar */}
        <form onSubmit={handleSearch} className="mb-6" role="search">
          <div className="relative max-w-xl">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" aria-hidden="true" />
            <Input
              type="text"
              placeholder="Search by asset name, symbol, or tx hash…"
              aria-label="Search transactions"
              value={searchInput}
              onChange={(e) => setSearchInput(e.target.value)}
              className="pl-9 pr-20"
            />
            {searchInput && (
              <Button
                type="button"
                variant="ghost"
                size="icon-xs"
                className="absolute right-14 top-1/2 -translate-y-1/2"
                onClick={() => { setSearchInput(""); setSearch("") }}
                aria-label="Clear search"
              >
                <X className="h-3.5 w-3.5" aria-hidden="true" />
              </Button>
            )}
            <Button
              type="submit"
              size="sm"
              className="absolute right-1.5 top-1/2 -translate-y-1/2 h-6"
            >
              Search
            </Button>
          </div>
        </form>

        <div className="flex gap-6">
          {/* Sidebar Filters */}
          <aside
            className={cn(
              "w-56 shrink-0",
              "hidden md:block",
              sidebarOpen && "!block fixed inset-0 z-40 bg-background/80 backdrop-blur md:relative md:bg-transparent md:backdrop-blur-none"
            )}
            aria-label="Transaction filters"
          >
            <div className={cn(
              "md:static md:bg-transparent md:shadow-none",
              sidebarOpen && "fixed left-0 top-0 h-full w-72 bg-background shadow-xl p-6 z-50 overflow-y-auto"
            )}>
              {sidebarOpen && (
                <div className="flex items-center justify-between mb-4 md:hidden">
                  <span className="font-semibold">Filters</span>
                  <Button variant="ghost" size="icon-sm" onClick={() => setSidebarOpen(false)} aria-label="Close filters">
                    <X className="h-4 w-4" aria-hidden="true" />
                  </Button>
                </div>
              )}
              <TransactionFilter
                filters={filters}
                onFiltersChange={(f) => { setFilters(f); setSidebarOpen(false) }}
              />
            </div>
          </aside>

          {/* Transaction List */}
          <div className="flex-1 min-w-0">
            <div className="border rounded-lg overflow-hidden">
              <TransactionList
                transactions={displayedTxs}
                isLoading={isLoading || infiniteLoading}
                hasMore={useInfinite ? infiniteHasMore : false}
                onLoadMore={loadMore}
                onDownloadReceipt={handleDownloadReceipt}
                downloadingId={downloadingId}
              />
            </div>

            {/* Pagination — only shown when not using infinite scroll */}
            {!useInfinite && totalPages > 1 && (
              <div className="mt-4">
                <Pagination
                  currentPage={page}
                  totalPages={totalPages}
                  totalItems={total}
                  pageSize={PAGE_SIZE}
                  onPageChange={setPage}
                  showTotalCount
                  showPageSizeSelector={false}
                />
              </div>
            )}
          </div>
        </div>
      </main>

      {/* Mobile filter overlay backdrop */}
      {sidebarOpen && (
        <div
          className="fixed inset-0 z-30 bg-black/40 md:hidden"
          onClick={() => setSidebarOpen(false)}
          aria-hidden="true"
        />
      )}
    </div>
  )
}
