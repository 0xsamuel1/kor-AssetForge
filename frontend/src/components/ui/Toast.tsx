"use client";

import * as React from "react";
import { createPortal } from "react-dom";
import {
  CheckCircle2,
  Info,
  Loader2,
  TriangleAlert,
  X,
  XCircle,
} from "lucide-react";

import {
  dismissToast,
  getServerToasts,
  getToasts,
  subscribeToToasts,
  type ToastRecord,
  type ToastVariant,
} from "@/lib/toast";
import { cn } from "@/lib/utils";

export type ToastPosition =
  | "top-left"
  | "top-center"
  | "top-right"
  | "bottom-left"
  | "bottom-center"
  | "bottom-right";

interface ToastContainerProps {
  /** Where the stack is anchored (default: bottom-right). */
  position?: ToastPosition;
  /** How many toasts render at once — the rest stay queued (default: 3). */
  maxVisible?: number;
}

const POSITION_CLASSES: Record<ToastPosition, string> = {
  "top-left": "top-0 left-0 items-start",
  "top-center": "top-0 left-1/2 -translate-x-1/2 items-center",
  "top-right": "top-0 right-0 items-end",
  "bottom-left": "bottom-0 left-0 items-start",
  "bottom-center": "bottom-0 left-1/2 -translate-x-1/2 items-center",
  "bottom-right": "bottom-0 right-0 items-end",
};

const VARIANT_ICON: Record<ToastVariant, React.ElementType> = {
  default: Info,
  success: CheckCircle2,
  error: XCircle,
  warning: TriangleAlert,
  info: Info,
  loading: Loader2,
};

/** Accent colours per variant. Neutral variants inherit the popover palette. */
const VARIANT_ICON_CLASSES: Record<ToastVariant, string> = {
  default: "text-muted-foreground",
  success: "text-emerald-600 dark:text-emerald-400",
  error: "text-destructive",
  warning: "text-amber-600 dark:text-amber-400",
  info: "text-sky-600 dark:text-sky-400",
  loading: "text-muted-foreground animate-spin",
};

const VARIANT_RING_CLASSES: Record<ToastVariant, string> = {
  default: "ring-foreground/10",
  success: "ring-emerald-500/30",
  error: "ring-destructive/30",
  warning: "ring-amber-500/30",
  info: "ring-sky-500/30",
  loading: "ring-foreground/10",
};

/** Exit animation length — kept in sync with the leaving classes below. */
const EXIT_DURATION = 160;

// Hydration-safe "are we on the client?" check: the server snapshot is false,
// the client snapshot is true, and the value never changes afterwards.
const neverChanges = () => () => {};
const onClient = () => true;
const onServer = () => false;

function ToastItem({
  toast,
  position,
}: {
  toast: ToastRecord;
  position: ToastPosition;
}) {
  const [isLeaving, setIsLeaving] = React.useState(false);
  const [isPaused, setIsPaused] = React.useState(false);

  const Icon = VARIANT_ICON[toast.variant];
  const isTop = position.startsWith("top");

  const close = React.useCallback(() => {
    setIsLeaving(true);
    window.setTimeout(() => dismissToast(toast.id), EXIT_DURATION);
  }, [toast.id]);

  // Auto-dismiss. Re-armed whenever the toast is updated (e.g. loading →
  // success) and suspended while the pointer or keyboard focus is on the stack.
  React.useEffect(() => {
    if (isPaused || isLeaving) return;
    if (!Number.isFinite(toast.duration)) return;

    const timer = window.setTimeout(close, toast.duration);
    return () => window.clearTimeout(timer);
  }, [close, isPaused, isLeaving, toast.duration, toast.updatedAt]);

  const handleAction = () => {
    // An action handler may return `false` to keep the toast on screen.
    const keepOpen = toast.action?.onClick() === false;
    if (!keepOpen) close();
  };

  return (
    <div
      // Errors interrupt the screen reader; everything else is announced politely.
      role={toast.variant === "error" ? "alert" : "status"}
      aria-atomic="true"
      data-variant={toast.variant}
      data-state={isLeaving ? "closed" : "open"}
      onMouseEnter={() => setIsPaused(true)}
      onMouseLeave={() => setIsPaused(false)}
      onFocusCapture={() => setIsPaused(true)}
      onBlurCapture={() => setIsPaused(false)}
      className={cn(
        "pointer-events-auto flex w-full items-start gap-3 rounded-xl bg-popover p-3.5 text-sm text-popover-foreground shadow-lg ring-1",
        VARIANT_RING_CLASSES[toast.variant],
        "motion-safe:transition-all motion-safe:duration-150 motion-safe:ease-out",
        isLeaving
          ? "motion-safe:scale-95 motion-safe:opacity-0"
          : "motion-safe:animate-in motion-safe:fade-in-0",
        !isLeaving &&
          (isTop
            ? "motion-safe:slide-in-from-top-2"
            : "motion-safe:slide-in-from-bottom-2"),
      )}
    >
      <Icon
        className={cn("mt-0.5 size-4 shrink-0", VARIANT_ICON_CLASSES[toast.variant])}
        aria-hidden="true"
      />

      <div className="flex-1 space-y-1">
        <p className="font-medium leading-snug break-words">{toast.title}</p>
        {toast.description && (
          <p className="text-xs leading-relaxed text-muted-foreground break-words">
            {toast.description}
          </p>
        )}
        {toast.action && (
          <button
            type="button"
            onClick={handleAction}
            className="mt-1 rounded-md text-xs font-medium text-primary underline-offset-4 transition-colors hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
          >
            {toast.action.label}
          </button>
        )}
      </div>

      {toast.dismissible && (
        <button
          type="button"
          onClick={close}
          aria-label="Dismiss notification"
          className="-mr-1 -mt-1 rounded-md p-1 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
        >
          <X className="size-3.5" aria-hidden="true" />
        </button>
      )}
    </div>
  );
}

/**
 * Renders the toast queue in a portal. Mount once, near the end of the root
 * layout — everything else talks to it through `toast()` from `@/lib/toast`.
 */
export function ToastContainer({
  position = "bottom-right",
  maxVisible = 3,
}: ToastContainerProps) {
  const toasts = React.useSyncExternalStore(
    subscribeToToasts,
    getToasts,
    getServerToasts,
  );

  // Portals need a DOM target, so wait for hydration before rendering.
  const isMounted = React.useSyncExternalStore(
    neverChanges,
    onClient,
    onServer,
  );

  if (!isMounted) return null;

  const visible = toasts.slice(0, maxVisible);
  const queued = toasts.length - visible.length;
  const isTop = position.startsWith("top");

  return createPortal(
    <div
      // The wrapper is click-through; individual toasts opt back in.
      // Announcements come from each toast's role (status/alert), so the
      // wrapper deliberately has no aria-live of its own — that would make
      // screen readers read every toast twice.
      className={cn(
        "pointer-events-none fixed z-[100] flex w-full max-w-full gap-2 p-4 sm:max-w-sm",
        // Column direction keeps the newest toast closest to the screen edge
        // and the overflow counter furthest from it.
        isTop ? "flex-col-reverse" : "flex-col",
        POSITION_CLASSES[position],
      )}
    >
      {queued > 0 && (
        <p className="px-1 text-xs text-muted-foreground">
          +{queued} more notification{queued > 1 ? "s" : ""}
        </p>
      )}

      {visible.map((toastRecord) => (
        <ToastItem
          key={toastRecord.id}
          toast={toastRecord}
          position={position}
        />
      ))}
    </div>,
    document.body,
  );
}
