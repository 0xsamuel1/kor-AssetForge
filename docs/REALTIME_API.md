# Realtime API

## Connect

`GET /ws?token=<access-jwt>&topics=assets&topics=transactions`

Browser WebSocket clients cannot set an `Authorization` header, so the token
is supplied as the `token` query parameter. Non-browser clients may use the
`Authorization: Bearer <token>` header. The token must be an unexpired JWT
with `type: access` and a numeric `user_id`.

## Messages

Server events use this envelope:

```json
{
  "type": "asset.created",
  "payload": {"id": 42},
  "timestamp": "2026-10-16T12:00:00Z"
}
```

Supported control messages are:

```json
{"action":"subscribe","topic":"assets"}
{"action":"unsubscribe","topic":"assets"}
```

The server sends `ping` events as application heartbeats. It also sends
`auth.expiring` one minute before the JWT expires. Events are at-most-once:
there is no server-side replay queue, and clients should resync after
reconnecting.

## Reconnection and authentication

On close code `4001`, refresh the session through `POST /api/v1/auth/refresh`
with `{"refresh_token":"..."}` and reconnect with the returned
`access_token`. A refresh failure is an authentication failure; stop retrying
and require the user to sign in again. The frontend `WebSocketClient` performs
this flow automatically when `refreshToken` is supplied, or applications can
provide `onTokenRefresh`.

## Limits and close codes

Each connection allows 20 inbound messages per second with a burst of 40 by
default. Set `WS_RATE_LIMIT_RPS` and `WS_RATE_LIMIT_BURST` to tune it. Code
`1008` means the message rate limit was exceeded; code `4001` means the access
token expired.
