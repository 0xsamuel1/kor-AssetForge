export type WSStatus = "connecting" | "connected" | "disconnected" | "error";

/**
 * Platform WebSocket endpoint. Uses `NEXT_PUBLIC_WS_URL` when set, otherwise
 * derives it from the REST API URL (`http://host` → `ws://host/ws`).
 */
export function getWebSocketUrl(): string {
  if (process.env.NEXT_PUBLIC_WS_URL) return process.env.NEXT_PUBLIC_WS_URL;

  const apiUrl = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";
  return `${apiUrl.replace(/^http/, "ws")}/ws`;
}

export interface WSMessage<T = unknown> {
  type: string;
  payload: T;
  timestamp?: number;
  /**
   * Monotonically increasing sequence number assigned by the server hub.
   * Used by WebSocketClient to detect gaps and reorder out-of-order
   * delivery. Messages without a seq (e.g. client-originated pings) are
   * delivered immediately without ordering guarantees.
   */
  seq?: number;
}

/** Emitted on the special "__gap__" subscription when messages are lost. */
export interface WSGapEvent {
  from: number;
  to: number;
}

export type WSMessageHandler<T = unknown> = (msg: WSMessage<T>) => void;

interface WebSocketClientOptions {
  /** URL of the WebSocket server */
  url: string;
  /** Heartbeat interval in ms (default: 30 000) */
  heartbeatInterval?: number;
  /** Max reconnect attempts before giving up (default: 10) */
  maxReconnectAttempts?: number;
  /** Base reconnect delay in ms (exponential back-off, default: 1 000) */
  reconnectDelay?: number;
  /** Called when connection status changes */
  onStatusChange?: (status: WSStatus) => void;
}

/**
 * Robust WebSocket client with:
 * - Auto-reconnect with exponential back-off
 * - Heartbeat / ping-pong
 * - Outgoing message queue (flushed on reconnect)
 * - Per-message-type subscription model
 */
export class WebSocketClient {
  private ws: WebSocket | null = null;
  private status: WSStatus = "disconnected";
  private reconnectAttempts = 0;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private heartbeatTimer: ReturnType<typeof setInterval> | null = null;
  private messageQueue: string[] = [];
  private handlers: Map<string, Set<WSMessageHandler>> = new Map();

  /** Ordering state for sequenced messages (see WSMessage.seq). */
  private lastSeq: number | null = null;
  private seqBuffer: Map<number, WSMessage> = new Map();
  private seqGapTimer: ReturnType<typeof setTimeout> | null = null;
  private static readonly SEQ_GAP_TIMEOUT_MS = 500;

  private readonly url: string;
  private readonly heartbeatInterval: number;
  private readonly maxReconnectAttempts: number;
  private readonly reconnectDelay: number;
  private readonly onStatusChange?: (status: WSStatus) => void;

  constructor(options: WebSocketClientOptions) {
    this.url = options.url;
    this.heartbeatInterval = options.heartbeatInterval ?? 30_000;
    this.maxReconnectAttempts = options.maxReconnectAttempts ?? 10;
    this.reconnectDelay = options.reconnectDelay ?? 1_000;
    this.onStatusChange = options.onStatusChange;
  }

  // ── Connection lifecycle ──────────────────────────────────────────────────

  connect(): void {
    if (
      this.ws?.readyState === WebSocket.OPEN ||
      this.ws?.readyState === WebSocket.CONNECTING
    ) {
      return;
    }

    this.setStatus("connecting");

    try {
      this.ws = new WebSocket(this.url);
    } catch {
      this.handleError();
      return;
    }

    this.ws.onopen = () => {
      this.reconnectAttempts = 0;
      // A new connection resumes the server's seq stream at an unknown
      // point, so ordering state must restart rather than carry over.
      this.lastSeq = null;
      this.seqBuffer.clear();
      if (this.seqGapTimer) {
        clearTimeout(this.seqGapTimer);
        this.seqGapTimer = null;
      }
      this.setStatus("connected");
      this.startHeartbeat();
      this.flushQueue();
    };

    this.ws.onclose = () => {
      this.teardown();
      this.scheduleReconnect();
    };

    this.ws.onerror = () => {
      this.handleError();
    };

    this.ws.onmessage = (event: MessageEvent) => {
      try {
        const msg: WSMessage = JSON.parse(event.data as string);
        // Handle pong silently
        if (msg.type === "pong") return;
        this.dispatch(msg);
      } catch {
        // Non-JSON frame — ignore
      }
    };
  }

  disconnect(): void {
    this.reconnectAttempts = this.maxReconnectAttempts; // prevent auto-reconnect
    this.teardown();
  }

  // ── Public API ────────────────────────────────────────────────────────────

  /**
   * Send a typed message. Queues it if not currently connected.
   */
  send<T>(type: string, payload: T): void {
    const msg: WSMessage<T> = { type, payload, timestamp: Date.now() };
    const raw = JSON.stringify(msg);

    if (this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(raw);
    } else {
      this.messageQueue.push(raw);
    }
  }

  /**
   * Subscribe to messages of a given type.
   * Returns an unsubscribe function.
   */
  subscribe<T = unknown>(
    type: string,
    handler: WSMessageHandler<T>
  ): () => void {
    if (!this.handlers.has(type)) {
      this.handlers.set(type, new Set());
    }
    this.handlers.get(type)!.add(handler as WSMessageHandler);

    return () => {
      this.handlers.get(type)?.delete(handler as WSMessageHandler);
    };
  }

  getStatus(): WSStatus {
    return this.status;
  }

  // ── Private helpers ───────────────────────────────────────────────────────

  private setStatus(status: WSStatus): void {
    this.status = status;
    this.onStatusChange?.(status);
  }

  private teardown(): void {
    this.stopHeartbeat();
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    if (
      this.ws &&
      this.ws.readyState !== WebSocket.CLOSED &&
      this.ws.readyState !== WebSocket.CLOSING
    ) {
      this.ws.close();
    }
    this.ws = null;
    this.setStatus("disconnected");
  }

  private handleError(): void {
    this.teardown();
    this.setStatus("error");
    this.scheduleReconnect();
  }

  private scheduleReconnect(): void {
    if (this.reconnectAttempts >= this.maxReconnectAttempts) return;

    const baseDelay =
      this.reconnectDelay * Math.pow(2, Math.min(this.reconnectAttempts, 6));
    // Full jitter: spreads reconnect attempts from many clients disconnected
    // by the same event (e.g. a server restart) instead of all retrying in
    // lockstep and causing a connection storm.
    const delay = baseDelay / 2 + Math.random() * (baseDelay / 2);
    this.reconnectAttempts += 1;

    this.reconnectTimer = setTimeout(() => {
      this.connect();
    }, delay);
  }

  private startHeartbeat(): void {
    this.heartbeatTimer = setInterval(() => {
      if (this.ws?.readyState === WebSocket.OPEN) {
        this.ws.send(JSON.stringify({ type: "ping", timestamp: Date.now() }));
      }
    }, this.heartbeatInterval);
  }

  private stopHeartbeat(): void {
    if (this.heartbeatTimer) {
      clearInterval(this.heartbeatTimer);
      this.heartbeatTimer = null;
    }
  }

  private flushQueue(): void {
    while (this.messageQueue.length > 0 && this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(this.messageQueue.shift()!);
    }
  }

  private dispatch(msg: WSMessage): void {
    if (typeof msg.seq !== "number") {
      // Unsequenced messages (e.g. client-originated echoes) have no
      // ordering guarantee to enforce — deliver immediately.
      this.deliverMessage(msg);
      return;
    }
    this.handleSequencedMessage(msg);
  }

  /**
   * Buffers and reorders messages by WSMessage.seq, delivering them to
   * handlers strictly in order. Out-of-order arrivals are held until the
   * gap is filled or SEQ_GAP_TIMEOUT_MS elapses, at which point delivery
   * resumes from the next available message and a "__gap__" event fires.
   */
  private handleSequencedMessage(msg: WSMessage): void {
    const seq = msg.seq as number;

    if (this.lastSeq === null) {
      // First sequenced message on this connection establishes the baseline.
      this.lastSeq = seq;
      this.deliverMessage(msg);
      this.drainSeqBuffer();
      return;
    }

    if (seq <= this.lastSeq) {
      return; // duplicate or stale retransmission — ignore
    }

    if (seq === this.lastSeq + 1) {
      this.lastSeq = seq;
      this.deliverMessage(msg);
      this.drainSeqBuffer();
      return;
    }

    // Out-of-order / gap: hold until the missing message(s) arrive.
    this.seqBuffer.set(seq, msg);
    this.scheduleGapRecovery();
  }

  private drainSeqBuffer(): void {
    while (this.lastSeq !== null && this.seqBuffer.has(this.lastSeq + 1)) {
      const next = this.seqBuffer.get(this.lastSeq + 1)!;
      this.seqBuffer.delete(this.lastSeq + 1);
      this.lastSeq += 1;
      this.deliverMessage(next);
    }
    if (this.seqBuffer.size === 0 && this.seqGapTimer) {
      clearTimeout(this.seqGapTimer);
      this.seqGapTimer = null;
    }
  }

  private scheduleGapRecovery(): void {
    if (this.seqGapTimer) return;
    this.seqGapTimer = setTimeout(() => {
      this.seqGapTimer = null;
      if (this.seqBuffer.size === 0 || this.lastSeq === null) return;

      // The missing message(s) never arrived in time. Skip ahead to the
      // lowest buffered seq so delivery can resume, and flag the gap.
      const resumeSeq = Math.min(...this.seqBuffer.keys());
      const from = this.lastSeq;
      this.lastSeq = resumeSeq - 1;
      this.dispatchGapEvent(from, resumeSeq);
      this.drainSeqBuffer();
    }, WebSocketClient.SEQ_GAP_TIMEOUT_MS);
  }

  private dispatchGapEvent(from: number, to: number): void {
    const gapEvent: WSMessage<WSGapEvent> = {
      type: "__gap__",
      payload: { from, to },
    };
    this.handlers.get("__gap__")?.forEach((h) => h(gapEvent));
  }

  private deliverMessage(msg: WSMessage): void {
    const typeHandlers = this.handlers.get(msg.type);
    typeHandlers?.forEach((h) => h(msg));

    // Also dispatch to wildcard "*" listeners
    this.handlers.get("*")?.forEach((h) => h(msg));
  }
}
