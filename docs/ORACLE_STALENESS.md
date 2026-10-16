# Oracle Staleness Handling

Implements #344 — "Oracle Price Feed Lacks Staleness Check".

A price feed that silently stops updating is a direct arbitrage primitive: buy
the asset cheaply while the contract still believes it is expensive (or the
reverse) and the position is closed the moment the feed wakes up. This document
describes the maximum-age policy applied on-chain and in the backend.

## Maximum age: 5 minutes

`Oracle::DEFAULT_STALENESS_THRESHOLD` (300s) is the maximum age accepted for a
price. It is deliberately short relative to the market it prices but long enough
to survive a missed heartbeat or a slow relayer.

| Layer | Enforcement |
| --- | --- |
| Contract | `Oracle::get_fresh_price`, `Oracle::is_price_stale`, `Oracle::get_price_with_fallback`, stale filtering inside `aggregate`, timestamp validation in `submit_band_price` |
| Backend | `OraclePriceService.FetchPriceStrict`, `PriceFeed.IsStale`, `DefaultMaxPriceAge` |
| Monitoring | `monitoring/alerts/oracle_staleness.yml` |

The threshold can be tuned per asset (`set_staleness_threshold`) or globally
(`set_default_staleness_threshold`), but a value of `0` is always rejected — it
would mark every price stale, including one submitted in the current ledger.

## On-chain behaviour

* `is_price_stale(asset_id)` — true when `now - last_update > threshold`. An
  asset that has never reported is always stale.
* `get_price_age(asset_id)` — age in seconds, `u64::MAX` when never reported.
* `get_fresh_price(asset_id)` — the price or a panic. **Every transaction that
  settles value must use this entry point.**
* `get_price_with_fallback(asset_id)` — falls back to the secondary oracle when
  the primary feed is stale. It panics if no fallback is configured, or if the
  fallback price is itself stale, so there is no silent degradation path.
* `submit_band_price` — rejects submissions whose reported timestamp is older
  than `max_price_age` or in the future, so a relayer cannot pin a dead price.
* `aggregate` — discards submissions older than `max_price_age` before computing
  the median, and panics when nothing in the round is fresh.

Every staleness rejection emits a `stale_price` event; a fallback resolution also
emits `fallback_price_used`, so both are visible to indexers and keepers.

## Fallback chain

1. Primary authorized oracles aggregate into `AggregatedPrice`.
2. When that price exceeds the staleness threshold, `get_price_with_fallback`
   reads the price most recently submitted by the asset's `FallbackSource`.
3. The fallback source is set by `set_fallback_source` and must itself be an
   authorized oracle; only it may call `submit_fallback_price`.
4. If the fallback price is also stale the call fails — value never settles
   against a stale number.

## Backend behaviour

`FetchPrice` walks the provider order (`PRICE_ORACLE_PROVIDER` first, then the
remaining providers) and returns the first response that is:

* successfully parsed,
* strictly positive, and
* no older than the configured maximum age.

`FetchPriceStrict` additionally guarantees that the caller never receives a
stale price — it is what the HTTP handlers use. When every provider fails it
returns an error wrapping `ErrStalePrice` and sets `oracle_stale_feed{symbol}=1`.

Configuration:

| Variable | Meaning | Default |
| --- | --- | --- |
| `PRICE_ORACLE_PROVIDER` | Primary provider | `chainlink` |
| `ORACLE_MAX_PRICE_AGE` | Maximum accepted price age (`300s` or `300`) | `300s` |
| `CUSTOM_PRICE_ORACLE_URL` | Endpoint used by the `custom` provider | unset |
| `ORACLE_CHAINLINK_URL` | Override the Chainlink-style endpoint | unset |
| `ORACLE_BAND_URL` | Override the Band Protocol endpoint | unset |

## Monitoring

| Metric | Meaning |
| --- | --- |
| `oracle_requests_total{provider,outcome}` | Fetch attempts by provider and outcome |
| `oracle_price_age_seconds{symbol}` | Age of the last accepted price |
| `oracle_stale_feed{symbol}` | `1` while a symbol has no usable price |

Alerts:

* **OracleFeedStale** (`oracle_stale_feed == 1`, 1m) — critical, pricing for the
  symbol is failing.
* **OraclePriceAgeHigh** (`oracle_price_age_seconds > 240`, 2m) — warning, the
  feed is about to go stale.
* **OracleAllProvidersStale** (any `outcome="stale"` traffic, 1m) — critical,
  fallbacks are exhausted network-wide.
* **OracleProviderErrorRateHigh** (>50% non-stale failures for a provider,
  5m) — warning, traffic is being shifted to fallbacks.

## Operational response

1. `OracleFeedStale` fires → check the relayer/keeper for that symbol.
2. If the primary is down, confirm a fallback source is configured
   (`get_fallback_source`); otherwise re-register one with
   `set_fallback_source` pointing at an authorized oracle.
3. If no fresh price can be sourced, listings, purchases and valuations for
   that asset are intentionally blocked. Do not raise the threshold to clear
   the alert — that re-opens the arbitrage window.