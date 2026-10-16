"use client";

import * as React from "react";
import Link from "next/link";
import {
  ArrowLeftRight,
  BadgeCheck,
  Coins,
  FileText,
  Lock,
  PieChart,
  RefreshCw,
  Tag,
  TriangleAlert,
  Vote,
} from "lucide-react";

import { InfiniteScroll } from "@/components/InfiniteScroll";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { SkeletonList } from "@/components/ui/skeleton";
import {
  useActivityFeed,
  type ActivityEvent,
  type ActivityType,
} from "@/hooks/useActivityFeed";
import { cn, formatAmount, truncateAddress } from "@/lib/utils";

interface ActivityFeedProps {
  /** Limit the feed to one asset. */
  assetId?: string;
  /** Show the event-type filter row (default: true). */
  showFilters?: boolean;
  /** Subscribe to live WebSocket updates (default: true). */
  live?: boolean;
  pageSize?: number;
  className?: string;
}

const EVENT_META: Record<
  ActivityType,
  { label: string; icon: React.ElementType; className: string }
> = {
  trade: {
    label: "Trades",
    icon: ArrowLeftRight,
    className: "bg-emerald-500/15 text-emerald-600 dark:text-emerald-400",
  },
  listing: {
    label: "Listings",
    icon: Tag,
    className: "bg-sky-500/15 text-sky-600 dark:text-sky-400",
  },
  proposal: {
    label: "Proposals",
    icon: FileText,
    className: "bg-violet-500/15 text-violet-600 dark:text-violet-400",
  },
  vote: {
    label: "Votes",
    icon: Vote,
    className: "bg-amber-500/15 text-amber-600 dark:text-amber-400",
  },
  mint: {
    label: "Mints",
    icon: Coins,
    className: "bg-indigo-500/15 text-indigo-600 dark:text-indigo-400",
  },
  fractionalize: {
    label: "Fractionalisations",
    icon: PieChart,
    className: "bg-teal-500/15 text-teal-600 dark:text-teal-400",
  },
  escrow: {
    label: "Escrow",
    icon: Lock,
    className: "bg-rose-500/15 text-rose-600 dark:text-rose-400",
  },
  verification: {
    label: "Verifications",
    icon: BadgeCheck,
    className: "bg-cyan-500/15 text-cyan-600 dark:text-cyan-400",
  },
};

const FILTERABLE_TYPES = Object.keys(EVENT_META) as ActivityType[];

/** Compact relative time ("just now", "12m", "3h", "5d"). */
function formatRelativeTime(timestamp: number): string {
  const seconds = Math.max(0, Math.round((Date.now() - timestamp) / 1000));

  if (seconds < 45) return "just now";
  if (seconds < 3600) return `${Math.round(seconds / 60)}m ago`;
  if (seconds < 86_400) return `${Math.round(seconds / 3600)}h ago`;
  return `${Math.round(seconds / 86_400)}d ago`;
}

function ActivityRow({ event }: { event: ActivityEvent }) {
  const meta = EVENT_META[event.type];
  const Icon = meta?.icon ?? FileText;

  const row = (
    <div className="flex items-start gap-3 rounded-lg p-2 transition-colors hover:bg-muted/60">
      <span
        className={cn(
          "flex size-8 shrink-0 items-center justify-center rounded-lg",
          meta?.className,
        )}
        aria-hidden="true"
      >
        <Icon className="size-4" />
      </span>

      <div className="min-w-0 flex-1">
        <p className="text-sm font-medium leading-snug">{event.title}</p>
        {event.description && (
          <p className="truncate text-xs text-muted-foreground">
            {event.description}
          </p>
        )}
        {event.actor && (
          <p className="mt-0.5 font-mono text-[0.7rem] text-muted-foreground">
            {truncateAddress(event.actor)}
          </p>
        )}
      </div>

      <div className="shrink-0 text-right">
        <time
          dateTime={new Date(event.timestamp).toISOString()}
          className="text-xs text-muted-foreground"
        >
          {formatRelativeTime(event.timestamp)}
        </time>
        {event.amount !== undefined && (
          <p className="font-mono text-xs">{formatAmount(event.amount, 2)}</p>
        )}
      </div>
    </div>
  );

  // Events tied to an asset link straight to its detail page.
  return event.assetId ? (
    <Link
      href={`/assets/${event.assetId}`}
      className="block rounded-lg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
    >
      {row}
    </Link>
  ) : (
    row
  );
}

/**
 * Platform activity feed: trades, listings, proposals and other events grouped
 * by day, filterable by type, with infinite scroll and live WebSocket updates.
 */
export function ActivityFeed({
  assetId,
  showFilters = true,
  live = true,
  pageSize = 20,
  className,
}: ActivityFeedProps) {
  const [selectedTypes, setSelectedTypes] = React.useState<ActivityType[]>([]);

  const {
    groups,
    events,
    isLoading,
    isLoadingMore,
    error,
    hasMore,
    isSample,
    liveStatus,
    loadMore,
    refresh,
  } = useActivityFeed({ types: selectedTypes, assetId, pageSize, live });

  const toggleType = (type: ActivityType) => {
    setSelectedTypes((current) =>
      current.includes(type)
        ? current.filter((value) => value !== type)
        : [...current, type],
    );
  };

  const isEmpty = !isLoading && events.length === 0;

  return (
    <Card className={className}>
      <CardHeader className="border-b">
        <CardTitle className="flex items-center gap-2">
          Activity
          {live && (
            <span
              role="status"
              className="inline-flex items-center gap-1.5 text-xs font-normal text-muted-foreground"
            >
              <span
                aria-hidden="true"
                className={cn(
                  "size-1.5 rounded-full",
                  liveStatus === "connected"
                    ? "bg-emerald-500"
                    : liveStatus === "connecting"
                      ? "animate-pulse bg-amber-400"
                      : "bg-muted-foreground/50",
                )}
              />
              {liveStatus === "connected" ? "Live" : "Offline"}
            </span>
          )}
        </CardTitle>
        <CardDescription>
          {assetId
            ? "Recent events for this asset"
            : "What's happening across the platform"}
        </CardDescription>
      </CardHeader>

      <CardContent className="space-y-4">
        {showFilters && (
          <div
            role="group"
            aria-label="Filter activity by type"
            className="flex flex-wrap gap-1.5"
          >
            <button
              type="button"
              onClick={() => setSelectedTypes([])}
              aria-pressed={selectedTypes.length === 0}
              className={cn(
                "rounded-full px-2.5 py-1 text-xs font-medium ring-1 transition-colors focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/50",
                selectedTypes.length === 0
                  ? "bg-primary text-primary-foreground ring-transparent"
                  : "ring-foreground/10 hover:bg-muted",
              )}
            >
              All
            </button>

            {FILTERABLE_TYPES.map((type) => (
              <button
                key={type}
                type="button"
                onClick={() => toggleType(type)}
                aria-pressed={selectedTypes.includes(type)}
                className={cn(
                  "rounded-full px-2.5 py-1 text-xs font-medium ring-1 transition-colors focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/50",
                  selectedTypes.includes(type)
                    ? "bg-primary text-primary-foreground ring-transparent"
                    : "ring-foreground/10 hover:bg-muted",
                )}
              >
                {EVENT_META[type].label}
              </button>
            ))}
          </div>
        )}

        {isSample && !isLoading && (
          <p className="flex items-start gap-2 rounded-lg bg-amber-500/10 p-2.5 text-xs text-amber-700 dark:text-amber-400">
            <TriangleAlert className="mt-0.5 size-3.5 shrink-0" aria-hidden="true" />
            The activity API is unavailable — showing sample events.
          </p>
        )}

        {error && (
          <div
            role="alert"
            className="flex items-center justify-between gap-3 rounded-lg bg-destructive/10 p-2.5 text-xs text-destructive"
          >
            <span>{error}</span>
            <Button variant="ghost" size="xs" onClick={refresh}>
              <RefreshCw className="size-3" aria-hidden="true" />
              Retry
            </Button>
          </div>
        )}

        {isLoading && <SkeletonList rows={5} />}

        {isEmpty && (
          <div className="py-10 text-center">
            <p className="text-sm text-muted-foreground">
              No activity to show
              {selectedTypes.length > 0 ? " for these filters" : " yet"}.
            </p>
            {selectedTypes.length > 0 && (
              <Button
                variant="outline"
                size="sm"
                className="mt-3"
                onClick={() => setSelectedTypes([])}
              >
                Clear filters
              </Button>
            )}
          </div>
        )}

        {!isLoading && !isEmpty && (
          <InfiniteScroll
            loadMore={loadMore}
            hasMore={hasMore}
            isLoading={isLoadingMore}
            endMessage="You're all caught up."
          >
            <div className="space-y-4">
              {groups.map((group) => (
                <section key={group.label}>
                  <h3 className="sticky top-0 z-10 bg-card/95 py-1 text-xs font-medium text-muted-foreground backdrop-blur">
                    {group.label}
                  </h3>
                  <div className="space-y-0.5">
                    {group.events.map((event) => (
                      <ActivityRow key={event.id} event={event} />
                    ))}
                  </div>
                </section>
              ))}
            </div>
          </InfiniteScroll>
        )}
      </CardContent>
    </Card>
  );
}
