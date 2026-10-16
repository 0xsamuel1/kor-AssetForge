import {
  Skeleton,
  SkeletonCard,
  SkeletonChart,
  SkeletonList,
  SkeletonMetric,
  SkeletonTable,
} from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";

/**
 * Skeleton screens for content that is still loading. They are built from the
 * primitives in `ui/skeleton` and shaped like the real layouts, so the page
 * doesn't jump when data arrives.
 *
 * Every screen is a `role="status"` region with an accessible label and
 * `aria-busy`, so assistive tech hears "Loading …" instead of empty boxes.
 */

interface SkeletonRegionProps {
  label: string;
  className?: string;
  children: React.ReactNode;
}

function SkeletonRegion({ label, className, children }: SkeletonRegionProps) {
  return (
    <div
      role="status"
      aria-busy="true"
      aria-label={label}
      className={className}
    >
      {children}
      <span className="sr-only">{label}</span>
    </div>
  );
}

/** Grid of card placeholders — asset grids, bundles, pools. */
export function CardGridSkeleton({
  count = 6,
  label = "Loading content",
  className,
}: {
  count?: number;
  label?: string;
  className?: string;
}) {
  return (
    <SkeletonRegion
      label={label}
      className={cn("grid gap-6 md:grid-cols-2 lg:grid-cols-3", className)}
    >
      {Array.from({ length: count }).map((_, i) => (
        <SkeletonCard key={i} />
      ))}
    </SkeletonRegion>
  );
}

/** Stacked rows — proposals, escrows, activity. */
export function ListSkeleton({
  rows = 4,
  label = "Loading list",
  className,
}: {
  rows?: number;
  label?: string;
  className?: string;
}) {
  return (
    <SkeletonRegion label={label} className={className}>
      <div className="space-y-3">
        {Array.from({ length: rows }).map((_, i) => (
          <div
            key={i}
            className="rounded-xl ring-1 ring-foreground/10 bg-card p-4 space-y-3"
          >
            <div className="flex items-center justify-between gap-4">
              <Skeleton className="h-4 w-1/2" />
              <Skeleton className="h-5 w-16 rounded-full" />
            </div>
            <Skeleton className="h-3 w-4/5" />
            <Skeleton className="h-3 w-1/3" />
          </div>
        ))}
      </div>
    </SkeletonRegion>
  );
}

/** Table placeholder — transactions, order history. */
export function TableSkeleton({
  rows = 5,
  cols = 4,
  label = "Loading table",
  className,
}: {
  rows?: number;
  cols?: number;
  label?: string;
  className?: string;
}) {
  return (
    <SkeletonRegion label={label} className={className}>
      <SkeletonTable rows={rows} cols={cols} />
    </SkeletonRegion>
  );
}

/** Row of KPI tiles followed by a chart — analytics and dashboards. */
export function DashboardSkeleton({
  label = "Loading dashboard",
  className,
}: {
  label?: string;
  className?: string;
}) {
  return (
    <SkeletonRegion label={label} className={cn("space-y-6", className)}>
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {Array.from({ length: 4 }).map((_, i) => (
          <SkeletonMetric key={i} />
        ))}
      </div>
      <SkeletonChart />
      <SkeletonList rows={3} />
    </SkeletonRegion>
  );
}

/** Two-column detail view — single asset or trading pair. */
export function DetailSkeleton({
  label = "Loading details",
  className,
}: {
  label?: string;
  className?: string;
}) {
  return (
    <SkeletonRegion label={label} className={cn("space-y-6", className)}>
      <div className="space-y-2">
        <Skeleton className="h-8 w-1/3" />
        <Skeleton className="h-4 w-1/2" />
      </div>
      <div className="grid gap-6 lg:grid-cols-3">
        <div className="space-y-6 lg:col-span-2">
          <SkeletonChart />
          <SkeletonCard />
        </div>
        <div className="space-y-6">
          <SkeletonCard />
          <SkeletonCard />
        </div>
      </div>
    </SkeletonRegion>
  );
}

/** Form placeholder — label + input pairs and a submit button. */
export function FormSkeleton({
  fields = 4,
  label = "Loading form",
  className,
}: {
  fields?: number;
  label?: string;
  className?: string;
}) {
  return (
    <SkeletonRegion label={label} className={cn("space-y-5", className)}>
      {Array.from({ length: fields }).map((_, i) => (
        <div key={i} className="space-y-2">
          <Skeleton className="h-3 w-24" />
          <Skeleton className="h-9 w-full" />
        </div>
      ))}
      <Skeleton className="h-9 w-32" />
    </SkeletonRegion>
  );
}

/**
 * Generic route-level skeleton: header bar, page title and a content grid.
 * Used by `loading.tsx` files while a route segment streams in.
 */
export function PageSkeleton({
  label = "Loading page",
  children,
}: {
  label?: string;
  children?: React.ReactNode;
}) {
  return (
    <div className="min-h-screen bg-background">
      <div className="border-b">
        <div className="container mx-auto flex h-16 items-center justify-between px-4">
          <Skeleton className="h-6 w-36" />
          <div className="hidden gap-4 md:flex">
            {Array.from({ length: 4 }).map((_, i) => (
              <Skeleton key={i} className="h-4 w-16" />
            ))}
          </div>
          <Skeleton className="h-9 w-32" />
        </div>
      </div>
      <main className="container mx-auto px-4 py-8">
        <div className="mb-8 space-y-2">
          <Skeleton className="h-8 w-64" />
          <Skeleton className="h-4 w-96 max-w-full" />
        </div>
        {children ?? <CardGridSkeleton label={label} />}
      </main>
    </div>
  );
}
