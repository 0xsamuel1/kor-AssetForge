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

    const delay =
      this.reconnectDelay * Math.pow(2, Math.min(this.reconnectAttempts, 6));
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
    const typeHandlers = this.handlers.get(msg.type);
    typeHandlers?.forEach((h) => h(msg));

    // Also dispatch to wildcard "*" listeners
    this.handlers.get("*")?.forEach((h) => h(msg));
  }
}
