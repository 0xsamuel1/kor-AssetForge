/**
 * Headless toast store.
 *
 * The store lives outside React so any module (API clients, event handlers,
 * non-component code) can raise a toast with a plain function call:
 *
 * ```ts
 * import { toast } from "@/lib/toast"
 *
 * toast.success("Proposal created")
 * toast.error("Vote failed", { description: err.message })
 * toast.info("New block", { action: { label: "View", onClick: openBlock } })
 * ```
 *
 * Rendering lives in `@/components/ui/Toast` — the container subscribes to this
 * store and owns the auto-dismiss timers (so they can pause on hover).
 */

export type ToastVariant =
  | "default"
  | "success"
  | "error"
  | "warning"
  | "info"
  | "loading";

export interface ToastAction {
  label: string;
  /** Invoked on click. The toast is dismissed afterwards unless this returns false. */
  onClick: () => void | boolean;
}

export interface ToastOptions {
  /** Supply to update an existing toast instead of pushing a new one. */
  id?: string;
  /** Secondary line rendered under the title. */
  description?: string;
  variant?: ToastVariant;
  /** Auto-dismiss delay in ms. `0` or `Infinity` keeps the toast until dismissed. */
  duration?: number;
  action?: ToastAction;
  /** Show the close button (default: true). */
  dismissible?: boolean;
  /** Called when the toast leaves the queue, for any reason. */
  onDismiss?: (id: string) => void;
}

export interface ToastRecord extends Omit<ToastOptions, "id" | "duration"> {
  id: string;
  title: string;
  variant: ToastVariant;
  duration: number;
  dismissible: boolean;
  /** Bumped on every update so the renderer knows to re-arm its timer. */
  updatedAt: number;
}

/** Per-variant defaults — errors linger, loading never auto-dismisses. */
const DEFAULT_DURATION: Record<ToastVariant, number> = {
  default: 4_000,
  success: 4_000,
  info: 5_000,
  warning: 6_000,
  error: 8_000,
  loading: Infinity,
};

/** Frozen empty snapshot so server renders stay referentially stable. */
const EMPTY: readonly ToastRecord[] = Object.freeze([]);

let toasts: readonly ToastRecord[] = EMPTY;
let counter = 0;

const listeners = new Set<() => void>();

function notify(): void {
  for (const listener of listeners) listener();
}

/** Subscribe to queue changes. Returns an unsubscribe function. */
export function subscribeToToasts(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

/** Current queue — a new array reference on every change (useSyncExternalStore safe). */
export function getToasts(): readonly ToastRecord[] {
  return toasts;
}

/** Server snapshot: toasts are client-only, so the queue is always empty on the server. */
export function getServerToasts(): readonly ToastRecord[] {
  return EMPTY;
}

function normalizeDuration(
  duration: number | undefined,
  variant: ToastVariant,
): number {
  if (duration === undefined) return DEFAULT_DURATION[variant];
  // `0` is the ergonomic way to ask for a sticky toast.
  return duration <= 0 ? Infinity : duration;
}

/**
 * Push a toast (or update one when `options.id` matches a queued toast).
 * Returns the toast id so callers can update or dismiss it later.
 */
function push(
  title: string,
  variant: ToastVariant,
  options: ToastOptions = {},
): string {
  const id = options.id ?? `toast-${++counter}`;
  const existing = toasts.find((t) => t.id === id);

  const record: ToastRecord = {
    id,
    title,
    description: options.description,
    variant,
    duration: normalizeDuration(options.duration, variant),
    dismissible: options.dismissible ?? true,
    action: options.action,
    onDismiss: options.onDismiss ?? existing?.onDismiss,
    updatedAt: Date.now(),
  };

  toasts = existing
    ? toasts.map((t) => (t.id === id ? record : t))
    : [...toasts, record];

  notify();
  return id;
}

/** Remove a single toast. No-op when the id is unknown. */
export function dismissToast(id: string): void {
  const target = toasts.find((t) => t.id === id);
  if (!target) return;

  toasts = toasts.filter((t) => t.id !== id);
  notify();
  target.onDismiss?.(id);
}

/** Remove every queued toast. */
export function dismissAllToasts(): void {
  const dismissed = toasts;
  toasts = EMPTY;
  notify();
  for (const t of dismissed) t.onDismiss?.(t.id);
}

type ToastFn = (title: string, options?: ToastOptions) => string;

interface ToastApi extends ToastFn {
  success: ToastFn;
  error: ToastFn;
  warning: ToastFn;
  info: ToastFn;
  loading: ToastFn;
  dismiss: (id: string) => void;
  dismissAll: () => void;
  /**
   * Bind a toast to a promise: shows a loading toast, then swaps it for the
   * success or error message when the promise settles.
   *
   * ```ts
   * await toast.promise(submitOrder(), {
   *   loading: "Submitting order…",
   *   success: "Order submitted",
   *   error: (err) => err.message,
   * })
   * ```
   */
  promise: <T>(
    promise: Promise<T>,
    messages: {
      loading: string;
      success: string | ((value: T) => string);
      error: string | ((error: unknown) => string);
    },
    options?: ToastOptions,
  ) => Promise<T>;
}

const toastFn: ToastFn = (title, options) =>
  push(title, options?.variant ?? "default", options);

export const toast: ToastApi = Object.assign(toastFn, {
  success: (title: string, options?: ToastOptions) =>
    push(title, "success", options),
  error: (title: string, options?: ToastOptions) =>
    push(title, "error", options),
  warning: (title: string, options?: ToastOptions) =>
    push(title, "warning", options),
  info: (title: string, options?: ToastOptions) => push(title, "info", options),
  loading: (title: string, options?: ToastOptions) =>
    push(title, "loading", options),
  dismiss: dismissToast,
  dismissAll: dismissAllToasts,
  promise: async <T,>(
    promise: Promise<T>,
    messages: {
      loading: string;
      success: string | ((value: T) => string);
      error: string | ((error: unknown) => string);
    },
    options?: ToastOptions,
  ): Promise<T> => {
    const id = push(messages.loading, "loading", { ...options, duration: 0 });

    try {
      const value = await promise;
      push(
        typeof messages.success === "function"
          ? messages.success(value)
          : messages.success,
        "success",
        { ...options, id, duration: options?.duration },
      );
      return value;
    } catch (error) {
      push(
        typeof messages.error === "function"
          ? messages.error(error)
          : messages.error,
        "error",
        { ...options, id, duration: options?.duration },
      );
      throw error;
    }
  },
});
