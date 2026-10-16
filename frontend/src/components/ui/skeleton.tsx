import { cn } from "@/lib/utils"

function Skeleton({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="skeleton"
      className={cn("animate-pulse rounded-md bg-muted", className)}
      {...props}
    />
  )
}

// Card skeleton — mimics Card layout with header + body rows
function SkeletonCard({ className }: { className?: string }) {
  return (
    <div
      data-slot="skeleton-card"
      className={cn(
        "rounded-xl ring-1 ring-foreground/10 bg-card p-4 space-y-3",
        className
      )}
    >
      <Skeleton className="h-4 w-2/3" />
      <Skeleton className="h-3 w-1/2" />
      <div className="space-y-2 pt-1">
        <Skeleton className="h-3 w-full" />
        <Skeleton className="h-3 w-4/5" />
      </div>
    </div>
  )
}

// Table skeleton — header row + N body rows
function SkeletonTable({
  rows = 5,
  cols = 4,
  className,
}: {
  rows?: number
  cols?: number
  className?: string
}) {
  return (
    <div className={cn("space-y-2", className)}>
      {/* header */}
      <div className="flex gap-4 px-4 py-2">
        {Array.from({ length: cols }).map((_, i) => (
          <Skeleton key={i} className="h-3 flex-1" />
        ))}
      </div>
      {/* rows */}
      {Array.from({ length: rows }).map((_, r) => (
        <div key={r} className="flex gap-4 px-4 py-3 border-t border-border/40">
          {Array.from({ length: cols }).map((_, c) => (
            <Skeleton
              key={c}
              className={cn("h-4 flex-1", c === 0 && "flex-[2]")}
            />
          ))}
        </div>
      ))}
    </div>
  )
}

// List skeleton — N stacked rows, each with avatar + two text lines
function SkeletonList({
  rows = 4,
  className,
}: {
  rows?: number
  className?: string
}) {
  return (
    <div className={cn("space-y-3", className)}>
      {Array.from({ length: rows }).map((_, i) => (
        <div key={i} className="flex items-center gap-3">
          <Skeleton className="h-10 w-10 rounded-full shrink-0" />
          <div className="flex-1 space-y-2">
            <Skeleton className="h-3 w-3/4" />
            <Skeleton className="h-3 w-1/2" />
          </div>
        </div>
      ))}
    </div>
  )
}

// Metrics card skeleton — icon placeholder + big number + label
function SkeletonMetric({ className }: { className?: string }) {
  return (
    <div
      className={cn(
        "rounded-xl ring-1 ring-foreground/10 bg-card p-4 space-y-3",
        className
      )}
    >
      <div className="flex justify-between items-start">
        <Skeleton className="h-3 w-24" />
        <Skeleton className="h-10 w-10 rounded-lg" />
      </div>
      <Skeleton className="h-7 w-28" />
      <Skeleton className="h-3 w-16" />
    </div>
  )
}

// Chart area skeleton — shimmer block with fixed height
function SkeletonChart({
  height = 200,
  className,
}: {
  height?: number
  className?: string
}) {
  return (
    <Skeleton
      className={cn("w-full rounded-lg", className)}
      style={{ height }}
    />
  )
}

export {
  Skeleton,
  SkeletonCard,
  SkeletonTable,
  SkeletonList,
  SkeletonMetric,
  SkeletonChart,
}
