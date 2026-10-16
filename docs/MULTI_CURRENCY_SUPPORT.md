# Multi-Currency Support

kor-AssetForge supports both fiat and cryptocurrency denominations across asset listings, payments, and portfolio valuations.

---

## Supported Currencies

| Code | Name              | Type   |
|------|-------------------|--------|
| USD  | US Dollar         | Fiat   |
| EUR  | Euro              | Fiat   |
| GBP  | British Pound     | Fiat   |
| XLM  | Stellar Lumens    | Crypto |
| BTC  | Bitcoin           | Crypto |
| ETH  | Ethereum          | Crypto |

All API endpoints that accept a `currency` parameter expect one of the codes above. Unsupported codes will return a `400 Bad Request`.

---

## Exchange Rate Source

Exchange rates are fetched from the [Open Exchange Rates API](https://open.er-api.com) (`open.er-api.com/v6/latest`).

- **Base currency:** USD (all rates expressed relative to USD)
- **Update frequency:** Rates are refreshed every **15 minutes** and cached in Redis
- **Fallback:** If the upstream API is unreachable and the cache is cold, the service falls back to hardcoded snapshot rates. This prevents total failure but may introduce slight inaccuracy until connectivity is restored
- **Pivot conversion:** Conversions between any two non-USD currencies use USD as the intermediate pivot (`amount / fromRate * toRate`)

### Configure the Rate Source

Set the following environment variable to point to any compatible exchange-rate API:

```
EXCHANGE_RATE_API_URL=https://open.er-api.com/v6/latest
```

If left unset, the service defaults to the hardcoded fallback rates only.

---

## Conversion Methodology

Conversions follow a two-step cross-rate calculation:

```
convertedAmount = (amount / rateFrom) * rateTo
```

Where `rateFrom` and `rateTo` are both expressed relative to USD.

**Example:** Convert 100 EUR to GBP

```
rateEUR = 0.92   (1 USD = 0.92 EUR)
rateGBP = 0.79   (1 USD = 0.79 GBP)

100 EUR → USD: 100 / 0.92 = 108.70 USD
108.70 USD → GBP: 108.70 * 0.79 = 85.87 GBP
```

---

## Fallback Rates

When the live API is unavailable these static rates are used:

| Currency | Rate (per USD) |
|----------|---------------|
| USD      | 1.000000      |
| EUR      | 0.920000      |
| GBP      | 0.790000      |
| XLM      | 8.930000      |
| BTC      | 0.000016      |
| ETH      | 0.000310      |

> These rates are approximations and will diverge from real market prices. Rely on the live API for production accuracy.

---

## API Examples with Currency Parameters

### Create a Payment in EUR

```http
POST /api/v1/payments
Authorization: Bearer <token>
Content-Type: application/json

{
  "asset_id": 42,
  "amount_fiat": 15000,
  "currency": "EUR",
  "gateway": "stripe"
}
```

**Response:**

```json
{
  "id": 101,
  "gateway": "stripe",
  "checkout_url": "https://checkout.stripe.com/pay/pi_stripe_...",
  "currency": "EUR",
  "amount_fiat": 15000,
  "status": "pending"
}
```

### Get Asset Price in GBP

```http
GET /api/v1/assets/42?currency=GBP
Authorization: Bearer <token>
```

The API converts the stored USD price to the requested currency using the live exchange rate before returning the response.

### Oracle Price Feed (XLM)

```http
GET /api/v1/oracle/price?asset_code=MYASSET&currency=XLM
```

**Response:**

```json
{
  "asset_code": "MYASSET",
  "price": "4.23",
  "currency": "XLM",
  "timestamp": "2026-10-16T10:00:00Z"
}
```

---

## Caching Behavior

| Layer   | TTL        | Notes                                        |
|---------|------------|----------------------------------------------|
| Redis   | 15 minutes | Shared across all API instances              |
| Fallback| N/A        | In-memory, served only when Redis + API fail |

Cache keys follow the pattern `kor:fx:rates`.

---

## Error Handling

| Scenario                       | Behavior                                      |
|--------------------------------|-----------------------------------------------|
| Unsupported currency code      | `400 Bad Request` with descriptive message    |
| Live API unreachable           | Fallback rates returned, no error surfaced    |
| Redis unavailable              | API fetch attempted directly                  |
| API + Redis both unavailable   | Fallback rates returned transparently         |

---

## Adding a New Currency

1. Append the ISO 4217 / ticker code to `SupportedCurrencies` in `backend/services/currency_service.go`
2. Add a fallback rate entry in `fallbackRates()`
3. Update this document and the table above
4. Verify the upstream exchange-rate API includes the new code

---

## Related Files

| File                                      | Purpose                                   |
|-------------------------------------------|-------------------------------------------|
| `backend/services/currency_service.go`   | FX rate fetching, caching, and conversion |
| `backend/services/payment_gateway.go`    | Fiat payment processing                   |
| `backend/handlers/payment.go`            | Payment HTTP handlers                     |
| `docs/API.md`                             | Full API reference                        |
