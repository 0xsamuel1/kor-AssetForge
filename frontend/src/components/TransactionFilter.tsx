"use client"

import { useState, useEffect } from "react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Badge } from "@/components/ui/badge"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { cn } from "@/lib/utils"
import { SlidersHorizontal, RotateCcw, ChevronDown, ChevronUp, X } from "lucide-react"

export interface TransactionFilters {
  search: string
  type: string
  status: string
  startDate: string
  endDate: string
  minAmount: string
  maxAmount: string
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

const TRANSACTION_TYPES = [
  { value: "buy", label: "Buy" },
  { value: "sell", label: "Sell" },
  { value: "transfer", label: "Transfer" },
  { value: "mint", label: "Mint" },
  { value: "burn", label: "Burn" },
  { value: "stake", label: "Stake" },
  { value: "unstake", label: "Unstake" },
  { value: "dividend", label: "Dividend" },
]

const STATUSES = [
  { value: "completed", label: "Completed" },
  { value: "pending", label: "Pending" },
  { value: "failed", label: "Failed" },
]

interface TransactionFilterProps {
  filters: TransactionFilters
  onFiltersChange: (filters: TransactionFilters) => void
  className?: string
}

export function TransactionFilter({ filters, onFiltersChange, className }: TransactionFilterProps) {
  const [expanded, setExpanded] = useState<Record<string, boolean>>({ date: true, type: true })
  const [localMin, setLocalMin] = useState(filters.minAmount)
  const [localMax, setLocalMax] = useState(filters.maxAmount)

  useEffect(() => {
    setLocalMin(filters.minAmount)
    setLocalMax(filters.maxAmount)
  }, [filters.minAmount, filters.maxAmount])

  const set = (key: keyof TransactionFilters, value: string) => {
    onFiltersChange({ ...filters, [key]: value })
  }

  const applyAmount = () => {
    onFiltersChange({ ...filters, minAmount: localMin, maxAmount: localMax })
  }

  const clearAll = () => {
    onFiltersChange({ ...DEFAULT_FILTERS })
    setLocalMin("")
    setLocalMax("")
  }

  const removeFilter = (key: keyof TransactionFilters) => {
    const next = { ...filters, [key]: "" }
    if (key === "minAmount" || key === "maxAmount") {
      next.minAmount = ""
      next.maxAmount = ""
      setLocalMin("")
      setLocalMax("")
    }
    onFiltersChange(next)
  }

  const activeChips: { key: keyof TransactionFilters; label: string }[] = []
  if (filters.type) activeChips.push({ key: "type", label: `Type: ${filters.type}` })
  if (filters.status) activeChips.push({ key: "status", label: `Status: ${filters.status}` })
  if (filters.startDate || filters.endDate) {
    const from = filters.startDate || "…"
    const to = filters.endDate || "…"
    activeChips.push({ key: "startDate", label: `Date: ${from} – ${to}` })
  }
  if (filters.minAmount || filters.maxAmount) {
    const lo = filters.minAmount || "0"
    const hi = filters.maxAmount || "∞"
    activeChips.push({ key: "minAmount", label: `Amount: $${lo} – $${hi}` })
  }

  const toggle = (key: string) => setExpanded((e) => ({ ...e, [key]: !e[key] }))

  return (
    <div className={cn("space-y-4", className)}>
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <SlidersHorizontal className="h-4 w-4" aria-hidden="true" />
          <span className="font-semibold text-sm">Filters</span>
          {activeChips.length > 0 && (
            <Badge variant="secondary" className="text-xs">{activeChips.length}</Badge>
          )}
        </div>
        {activeChips.length > 0 && (
          <Button variant="ghost" size="sm" onClick={clearAll} className="h-7 text-xs">
            <RotateCcw className="h-3 w-3 mr-1" aria-hidden="true" />
            Clear
          </Button>
        )}
      </div>

      {activeChips.length > 0 && (
        <div className="flex flex-wrap gap-1">
          {activeChips.map((chip) => (
            <Badge key={chip.key} variant="secondary" className="gap-1 pr-1">
              <span className="text-xs">{chip.label}</span>
              <button
                type="button"
                aria-label={`Remove ${chip.label} filter`}
                onClick={() => removeFilter(chip.key)}
                className="ml-1 hover:bg-muted rounded-full p-0.5"
              >
                <X className="h-3 w-3" aria-hidden="true" />
              </button>
            </Badge>
          ))}
        </div>
      )}

      {/* Transaction Type */}
      <div>
        <button
          type="button"
          aria-expanded={!!expanded.type}
          className="flex items-center justify-between w-full text-sm font-medium mb-2"
          onClick={() => toggle("type")}
        >
          Transaction Type
          {expanded.type ? <ChevronUp className="h-4 w-4" aria-hidden="true" /> : <ChevronDown className="h-4 w-4" aria-hidden="true" />}
        </button>
        {expanded.type && (
          <Select value={filters.type} onValueChange={(v) => set("type", v === "_all" ? "" : v)}>
            <SelectTrigger className="h-8 text-sm">
              <SelectValue placeholder="All types" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="_all">All types</SelectItem>
              {TRANSACTION_TYPES.map((t) => (
                <SelectItem key={t.value} value={t.value}>{t.label}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
      </div>

      {/* Status */}
      <div>
        <button
          type="button"
          aria-expanded={!!expanded.status}
          className="flex items-center justify-between w-full text-sm font-medium mb-2"
          onClick={() => toggle("status")}
        >
          Status
          {expanded.status ? <ChevronUp className="h-4 w-4" aria-hidden="true" /> : <ChevronDown className="h-4 w-4" aria-hidden="true" />}
        </button>
        {expanded.status && (
          <div className="space-y-1 pl-1">
            {STATUSES.map((s) => (
              <button
                key={s.value}
                type="button"
                aria-pressed={filters.status === s.value}
                onClick={() => set("status", filters.status === s.value ? "" : s.value)}
                className={cn(
                  "flex items-center w-full text-sm px-2 py-1.5 rounded-md transition-colors",
                  filters.status === s.value
                    ? "bg-primary/10 text-primary font-medium"
                    : "hover:bg-muted text-muted-foreground"
                )}
              >
                {s.label}
              </button>
            ))}
          </div>
        )}
      </div>

      {/* Date Range */}
      <div>
        <button
          type="button"
          aria-expanded={!!expanded.date}
          className="flex items-center justify-between w-full text-sm font-medium mb-2"
          onClick={() => toggle("date")}
        >
          Date Range
          {expanded.date ? <ChevronUp className="h-4 w-4" aria-hidden="true" /> : <ChevronDown className="h-4 w-4" aria-hidden="true" />}
        </button>
        {expanded.date && (
          <div className="space-y-2 pl-1">
            <div>
              <label className="text-xs text-muted-foreground mb-1 block" htmlFor="filter-start-date">From</label>
              <Input
                id="filter-start-date"
                type="date"
                value={filters.startDate}
                onChange={(e) => set("startDate", e.target.value)}
                className="h-8 text-sm"
              />
            </div>
            <div>
              <label className="text-xs text-muted-foreground mb-1 block" htmlFor="filter-end-date">To</label>
              <Input
                id="filter-end-date"
                type="date"
                value={filters.endDate}
                onChange={(e) => set("endDate", e.target.value)}
                className="h-8 text-sm"
              />
            </div>
          </div>
        )}
      </div>

      {/* Amount Range */}
      <div>
        <button
          type="button"
          aria-expanded={!!expanded.amount}
          className="flex items-center justify-between w-full text-sm font-medium mb-2"
          onClick={() => toggle("amount")}
        >
          Amount Range
          {expanded.amount ? <ChevronUp className="h-4 w-4" aria-hidden="true" /> : <ChevronDown className="h-4 w-4" aria-hidden="true" />}
        </button>
        {expanded.amount && (
          <div className="space-y-2 pl-1">
            <div className="flex gap-2">
              <Input
                type="number"
                placeholder="Min"
                aria-label="Minimum amount"
                value={localMin}
                min={0}
                onChange={(e) => setLocalMin(e.target.value)}
                className="h-8 text-sm"
              />
              <Input
                type="number"
                placeholder="Max"
                aria-label="Maximum amount"
                value={localMax}
                min={0}
                onChange={(e) => setLocalMax(e.target.value)}
                className="h-8 text-sm"
              />
            </div>
            <Button size="sm" className="w-full h-7 text-xs" onClick={applyAmount}>
              Apply
            </Button>
          </div>
        )}
      </div>
    </div>
  )
}
