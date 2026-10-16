"use client";

import { useEffect, useMemo, useState } from "react";

type Batch = {
  id: number;
  status: string;
  progress_percent: number;
  completed_count: number;
  failed_count: number;
  total_operations: number;
  estimated_seconds_remaining?: number;
};

type BatchProgressProps = {
  batchId: number;
  apiBase?: string;
  token?: string;
};

export function BatchProgress({ batchId, apiBase = "", token = "" }: BatchProgressProps) {
  const [batch, setBatch] = useState<Batch>();
  const [error, setError] = useState<string>();
  const endpoint = useMemo(() => `${apiBase}/api/v1/batch/${batchId}`, [apiBase, batchId]);

  useEffect(() => {
    let cancelled = false;
    const load = async () => {
      try {
        const response = await fetch(endpoint, { headers: token ? { Authorization: `Bearer ${token}` } : undefined });
        if (!response.ok) throw new Error("Unable to load batch progress");
        const value = (await response.json()) as Batch;
        if (!cancelled) {
          setBatch(value);
          setError(undefined);
        }
      } catch (cause) {
        if (!cancelled) setError(cause instanceof Error ? cause.message : "Unable to load batch progress");
      }
    };
    void load();
    const interval = window.setInterval(load, 2000);

    let socket: WebSocket | undefined;
    if (typeof window !== "undefined" && token) {
      const wsBase = endpoint.replace(/^http/, "ws").replace(/\/api\/v1\/batch\/\d+$/, "/ws");
      socket = new WebSocket(`${wsBase}?token=${encodeURIComponent(token)}&topics=transactions`);
      socket.onmessage = (event) => {
        try {
          const message = JSON.parse(event.data) as { type?: string; payload?: Batch & { batch_id?: number } };
          if (message.type === "batch.progress" && message.payload?.batch_id === batchId) {
            setBatch((current) => ({ ...current, ...message.payload } as Batch));
          }
        } catch {
          // Polling remains the source of truth when a socket message is malformed.
        }
      };
    }

    return () => {
      cancelled = true;
      window.clearInterval(interval);
      socket?.close();
    };
  }, [batchId, endpoint, token]);

  if (error) return <p role="alert">{error}</p>;
  if (!batch) return <p aria-live="polite">Loading batch progress...</p>;

  const progress = Math.min(100, Math.max(0, batch.progress_percent));
  return (
    <section aria-label={`Batch ${batch.id} progress`} className="space-y-2">
      <div className="flex justify-between text-sm">
        <span>{batch.status}</span>
        <span>{Math.round(progress)}%</span>
      </div>
      <progress className="h-2 w-full" max={100} value={progress} aria-valuetext={`${Math.round(progress)} percent complete`} />
      <p className="text-sm text-muted-foreground">
        {batch.completed_count} completed, {batch.failed_count} failed of {batch.total_operations}
        {batch.estimated_seconds_remaining ? ` · about ${batch.estimated_seconds_remaining}s remaining` : ""}
      </p>
    </section>
  );
}
