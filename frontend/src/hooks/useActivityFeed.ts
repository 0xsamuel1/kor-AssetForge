"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import {
  WebSocketClient,
  getWebSocketUrl,
  type WSStatus,
} from "@/lib/websocket";

export type ActivityType =
  | "trade"
  | "listing"
  | "proposal"
  | "vote"
  | "mint"
  | "fractionalize"
  | "escrow"
  | "verification";

export interface ActivityEvent {
  id: string;
  type: ActivityType;
  title: string;
  description?: string;
  /** Wallet address that triggered the event. */
  actor?: string;
  assetId?: string;
  assetName?: string;
  amount?: number;
  /** Epoch milliseconds. */
  timestamp: number;
  txHash?: string;
}

export interface ActivityGroup {
  /** "Today", "Yesterday", or a formatted date. */
  label: string;
  events: ActivityEvent[];
}

interface ActivityPage {
  events: ActivityEvent[];
  nextCursor?: string;
  hasMore: boolean;
}

export interface UseActivityFeedOptions {
  /** Restrict the feed to these event types. Empty means "everything". */
  types?: ActivityType[];
  /** Restrict the feed to a single asset. */
  assetId?: string;
  /** Events fetched per page (default: 20). */
  pageSize?: number;
  /** Subscribe to live updates over WebSocket (default: true). */
  live?: boolean;
  /** Upper bound on retained events, so long sessions don't grow forever. */
  maxEvents?: number;
}

export interface UseActivityFeedResult {
  events: ActivityEvent[];
  /** `events` bucketed by day, ready to render as sections. */
  groups: ActivityGroup[];
  isLoading: boolean;
  isLoadingMore: boolean;
  error: string | null;
  hasMore: boolean;
  /** True while the feed is showing locally generated sample events. */
  isSample: boolean;
  liveStatus: WSStatus;
  loadMore: () => Promise<void>;
  refresh: () => Promise<void>;
}

const API_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

const DAY_MS = 24 * 60 * 60 * 1000;

/** Groups events into Today / Yesterday / explicit dates, newest first. */
export function groupActivitiesByDay(events: ActivityEvent[]): ActivityGroup[] {
  const startOfToday = new Date();
  startOfToday.setHours(0, 0, 0, 0);
  const todayMs = startOfToday.getTime();

  const groups = new Map<string, ActivityEvent[]>();

  for (const event of events) {
    let label: string;

    if (event.timestamp >= todayMs) {
      label = "Today";
    } else if (event.timestamp >= todayMs - DAY_MS) {
      label = "Yesterday";
    } else {
      label = new Intl.DateTimeFormat("en-US", {
        month: "long",
        day: "numeric",
        year:
          new Date(event.timestamp).getFullYear() === startOfToday.getFullYear()
            ? undefined
            : "numeric",
      }).format(new Date(event.timestamp));
    }

    const bucket = groups.get(label);
    if (bucket) {
      bucket.push(event);
    } else {
      groups.set(label, [event]);
    }
  }

  return Array.from(groups, ([label, groupEvents]) => ({
    label,
    events: groupEvents,
  }));
}

// ── Sample data ──────────────────────────────────────────────────────────────

const SAMPLE_TEMPLATES: {
  type: ActivityType;
  title: string;
  description: string;
}[] = [
  {
    type: "trade",
    title: "Downtown Office Tower traded",
    description: "120 DOT filled at $104.20",
  },
  {
    type: "listing",
    title: "Harbor Warehouse Fund listed",
    description: "500 HWF offered at $58.00",
  },
  {
    type: "proposal",
    title: "Proposal #18 opened",
    description: "Increase quarterly dividend payout",
  },
  {
    type: "vote",
    title: "Vote cast on proposal #17",
    description: "1,240 votes in favour",
  },
  {
    type: "mint",
    title: "Retail Plaza Token minted",
    description: "10,000 RPT issued",
  },
  {
    type: "fractionalize",
    title: "Luxury Residences fractionalised",
    description: "Split into 5,000 shares",
  },
  {
    type: "escrow",
    title: "Escrow released",
    description: "Funds delivered to the seller",
  },
  {
    type: "verification",
    title: "Industrial Park Token verified",
    description: "Documents approved by a verifier",
  },
];

/**
 * Deterministic sample events used when the activity API isn't reachable, so
 * the feed stays explorable in local development.
 */
function generateSampleActivity(
  count: number,
  before: number = Date.now(),
): ActivityEvent[] {
  return Array.from({ length: count }, (_, index) => {
    const template = SAMPLE_TEMPLATES[index % SAMPLE_TEMPLATES.length];
    // Spread events out: a few minutes apart at first, then hours.
    const offset = (index + 1) * (index < 5 ? 7 * 60_000 : 5 * 60 * 60_000);

    return {
      id: `sample-${before}-${index}`,
      type: template.type,
      title: template.title,
      description: template.description,
      actor: `GA${(index * 7919).toString(36).toUpperCase().padStart(6, "X")}…`,
      timestamp: before - offset,
    } satisfies ActivityEvent;
  });
}

// ── Hook ─────────────────────────────────────────────────────────────────────

/**
 * Loads the platform activity feed with cursor pagination, live WebSocket
 * updates and day grouping.
 *
 * ```tsx
 * const { groups, hasMore, loadMore } = useActivityFeed({ types: ["trade"] })
 * ```
 */
export function useActivityFeed({
  types,
  assetId,
  pageSize = 20,
  live = true,
  maxEvents = 200,
}: UseActivityFeedOptions = {}): UseActivityFeedResult {
  const [events, setEvents] = useState<ActivityEvent[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [isLoadingMore, setIsLoadingMore] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [hasMore, setHasMore] = useState(true);
  const [isSample, setIsSample] = useState(false);
  const [liveStatus, setLiveStatus] = useState<WSStatus>("disconnected");

  const cursorRef = useRef<string | undefined>(undefined);

  // Serialised so the fetch callbacks don't change identity on every render.
  const typesKey = types?.join(",") ?? "";

  const fetchPage = useCallback(
    async (cursor?: string): Promise<ActivityPage> => {
      const query = new URLSearchParams({ limit: String(pageSize) });
      if (cursor) query.set("cursor", cursor);
      if (typesKey) query.set("types", typesKey);
      if (assetId) query.set("asset_id", assetId);

      const res = await fetch(`${API_URL}/api/v1/activity?${query}`);
      if (!res.ok) throw new Error(`Activity feed unavailable (${res.status})`);

      const data = (await res.json()) as Partial<ActivityPage>;
      return {
        events: data.events ?? [],
        nextCursor: data.nextCursor,
        hasMore: data.hasMore ?? Boolean(data.nextCursor),
      };
    },
    [pageSize, typesKey, assetId],
  );

  const refresh = useCallback(async () => {
    try {
      const page = await fetchPage();
      cursorRef.current = page.nextCursor;
      setEvents(page.events);
      setHasMore(page.hasMore);
      setIsSample(false);
      setError(null);
    } catch {
      // Fall back to sample events rather than an empty screen, and say so.
      cursorRef.current = undefined;
      setEvents(generateSampleActivity(pageSize));
      setHasMore(true);
      setIsSample(true);
      setError(null);
    } finally {
      setIsLoading(false);
    }
  }, [fetchPage, pageSize]);

  const loadMore = useCallback(async () => {
    if (isLoadingMore || !hasMore) return;

    setIsLoadingMore(true);
    try {
      if (isSample) {
        // Keep paginating the sample timeline backwards in time.
        const oldest = events[events.length - 1]?.timestamp ?? Date.now();
        const combined = [
          ...events,
          ...generateSampleActivity(pageSize, oldest),
        ].slice(0, maxEvents);

        setEvents(combined);
        setHasMore(combined.length < maxEvents);
        return;
      }

      const page = await fetchPage(cursorRef.current);
      cursorRef.current = page.nextCursor;
      setEvents((current) => {
        const seen = new Set(current.map((event) => event.id));
        const merged = [
          ...current,
          ...page.events.filter((event) => !seen.has(event.id)),
        ];
        return merged.slice(0, maxEvents);
      });
      setHasMore(page.hasMore);
    } catch (err) {
      setError(
        err instanceof Error ? err.message : "Could not load more activity.",
      );
      setHasMore(false);
    } finally {
      setIsLoadingMore(false);
    }
  }, [events, fetchPage, hasMore, isLoadingMore, isSample, maxEvents, pageSize]);

  // Initial load, and a reload whenever the filters change. Queued as a task so
  // the effect body performs no synchronous state updates.
  useEffect(() => {
    const timer = window.setTimeout(refresh, 0);
    return () => window.clearTimeout(timer);
  }, [refresh]);

  // Live updates: new events are prepended, de-duplicated and filtered with the
  // same rules as the REST query.
  useEffect(() => {
    if (!live) return;

    const activeTypes = typesKey ? typesKey.split(",") : [];
    const client = new WebSocketClient({
      url: getWebSocketUrl(),
      onStatusChange: setLiveStatus,
    });

    const unsubscribe = client.subscribe<ActivityEvent>(
      "activity",
      ({ payload }) => {
        if (!payload?.id) return;
        if (activeTypes.length > 0 && !activeTypes.includes(payload.type)) return;
        if (assetId && payload.assetId !== assetId) return;

        setEvents((current) =>
          current.some((event) => event.id === payload.id)
            ? current
            : [payload, ...current].slice(0, maxEvents),
        );
      },
    );

    client.connect();
    client.send("subscribe", { channel: "activity", types: activeTypes, assetId });

    return () => {
      unsubscribe();
      client.disconnect();
    };
  }, [live, typesKey, assetId, maxEvents]);

  const groups = useMemo(() => groupActivitiesByDay(events), [events]);

  return {
    events,
    groups,
    isLoading,
    isLoadingMore,
    error,
    hasMore,
    isSample,
    liveStatus,
    loadMore,
    refresh,
  };
}
