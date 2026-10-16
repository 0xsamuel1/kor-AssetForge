# WebSocket architecture

The `/ws` endpoint is a process-local publish/subscribe hub. A client
connects with an access JWT, optionally supplies repeated `topics` query
parameters, and is registered as a subscriber. The hub owns registration,
unregistration, topic filtering, heartbeat delivery, and fan-out. Each
subscriber has a bounded outbound queue (128 messages); a client that cannot
keep up is disconnected rather than blocking every other client.

## Connection lifecycle

1. The server validates the access JWT, then performs the WebSocket upgrade.
2. The subscriber is registered and receives `ping` heartbeat events every 30
   seconds.
3. The client may send `{"action":"subscribe","topic":"assets"}` or
   `{"action":"unsubscribe","topic":"assets"}`.
4. A normal close removes the subscriber and closes its writer queue.
5. The access token sends an `auth.expiring` event one minute before expiry and
   closes with code `4001` at expiry. The client must refresh and reconnect.

## Delivery guarantees

Messages are delivered at-most-once. Broadcasts are queued in a bounded hub
channel and may be dropped when the hub is saturated. A slow subscriber is
disconnected when its bounded outbound queue is full. The service does not
persist messages or replay them after reconnect, so consumers should refetch
authoritative state after a reconnect.

## Rate limiting

Inbound messages use a token bucket per connection. Defaults are 20 messages
per second with a burst of 40; configure `WS_RATE_LIMIT_RPS` and
`WS_RATE_LIMIT_BURST`. A client that exceeds its bucket is closed with code
`1008`.

## Example

```ts
const socket = new WebSocketClient({
  url: "wss://api.example.com/ws",
  accessToken: () => auth.accessToken,
  refreshToken: () => auth.refreshToken,
  onTokenRefresh: refreshSession,
});
socket.connect();
```
