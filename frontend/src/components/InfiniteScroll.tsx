"use client"

import { useEffect, useRef, useCallback } from "react"
import { Loader2 } from "lucide-react"

interface InfiniteScrollProps {
  loadMore: () => Promise<void> | void
  hasMore: boolean
  isLoading: boolean
  loader?: React.ReactNode
  endMessage?: React.ReactNode
  threshold?: number
  rootMargin?: string
  className?: string
  children: React.ReactNode
}

export function InfiniteScroll({
  loadMore,
  hasMore,
  isLoading,
  loader,
  endMessage,
  threshold = 0.1,
  rootMargin = "200px",
  className,
  children,
}: InfiniteScrollProps) {
  const sentinelRef = useRef<HTMLDivElement>(null)

  const handleObserver = useCallback(
    (entries: IntersectionObserverEntry[]) => {
      const [entry] = entries
      if (entry.isIntersecting && hasMore && !isLoading) {
        loadMore()
      }
    },
    [loadMore, hasMore, isLoading],
  )

  useEffect(() => {
    const sentinel = sentinelRef.current
    if (!sentinel) return

    const observer = new IntersectionObserver(handleObserver, {
      threshold,
      rootMargin,
    })

    observer.observe(sentinel)
    return () => observer.disconnect()
  }, [handleObserver, threshold, rootMargin])

  return (
    <div className={className}>
      {children}

      <div ref={sentinelRef} className="flex justify-center py-4">
        {isLoading &&
          (loader || (
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <Loader2 className="h-4 w-4 animate-spin" />
              Loading more...
            </div>
          ))}
        {!hasMore && !isLoading && endMessage && (
          <div className="text-sm text-muted-foreground">{endMessage}</div>
        )}
      </div>
    </div>
  )
}
