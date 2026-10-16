"use client";

import { useEffect, useRef, useState, useCallback } from "react";
import { WebSocketClient, WSStatus, WSMessage, WSMessageHandler } from "@/lib/websocket";

interface UseWebSocketOptions {
  /** WebSocket server URL. Pass null to disable. */
  url: string | null;
  /** Auto-connect on mount (default: true) */
  autoConnect?: boolean;
  heartbeatInterval?: number;
  maxReconnectAttempts?: number;
}

interface UseWebSocketReturn {
  status: WSStatus;
  /** Send a typed message */
  send: <T>(type: string, payload: T) => void;
  /** Subscribe to a message type; returns unsubscribe fn */
  subscribe: <T>(type: string, handler: WSMessageHandler<T>) => () => void;
  /** Last received message (any type) */
  lastMessage: WSMessage | null;
  connect: () => void;
  disconnect: () => void;
}

/**
 * React hook for WebSocket connectivity.
 *
 * Usage:
 * ```tsx
 * const { status, subscribe } = useWebSocket({ url: "wss://example.com/ws" });
 *
 * useEffect(() => {
 *   return subscribe<PriceUpdate>("price_update", ({ payload }) => {
 *     setPrice(payload.price);
 *   });
 * }, [subscribe]);
 * ```
 */
export function useWebSocket({
  url,
  autoConnect = true,
  heartbeatInterval,
  maxReconnectAttempts,
}: UseWebSocketOptions): UseWebSocketReturn {
  const [status, setStatus] = useState<WSStatus>("disconnected");
  const [lastMessage, setLastMessage] = useState<WSMessage | null>(null);
  const clientRef = useRef<WebSocketClient | null>(null);

  // Initialise (or re-initialise when URL changes)
  useEffect(() => {
    if (!url) return;

    const client = new WebSocketClient({
      url,
      heartbeatInterval,
      maxReconnectAttempts,
      onStatusChange: setStatus,
    });

    // Capture all messages for lastMessage
    client.subscribe("*", (msg: WSMessage) => setLastMessage(msg));

    clientRef.current = client;

    if (autoConnect) client.connect();

    return () => {
      client.disconnect();
      clientRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [url]);

  const send = useCallback(<T,>(type: string, payload: T) => {
    clientRef.current?.send(type, payload);
  }, []);

  const subscribe = useCallback(
    <T,>(type: string, handler: WSMessageHandler<T>) =>
      clientRef.current?.subscribe(type, handler) ?? (() => {}),
    []
  );

  const connect = useCallback(() => clientRef.current?.connect(), []);
  const disconnect = useCallback(() => clientRef.current?.disconnect(), []);

  return { status, send, subscribe, lastMessage, connect, disconnect };
}

// ── Connection status badge component ────────────────────────────────────────

interface WSStatusBadgeProps {
  status: WSStatus;
  className?: string;
}

const STATUS_CONFIG: Record<WSStatus, { label: string; color: string }> = {
  connected: { label: "Live", color: "bg-emerald-500" },
  connecting: { label: "Connecting…", color: "bg-amber-400" },
  disconnected: { label: "Offline", color: "bg-gray-400" },
  error: { label: "Error", color: "bg-red-500" },
};

export function WSStatusBadge({ status, className = "" }: WSStatusBadgeProps) {
  const { label, color } = STATUS_CONFIG[status];
  return (
    <span
      className={`inline-flex items-center gap-1.5 text-xs font-medium ${className}`}
      role="status"
      aria-label={`WebSocket: ${label}`}
    >
      <span
        className={`w-2 h-2 rounded-full ${color} ${status === "connecting" ? "animate-pulse" : ""}`}
      />
      {label}
    </span>
  );
}
