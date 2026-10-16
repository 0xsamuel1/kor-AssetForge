# kor-AssetForge API Client SDK

This document describes how to integrate with the kor-AssetForge API using the official SDKs and REST endpoints.

## Overview

The kor-AssetForge API provides RESTful endpoints for asset management, marketplace operations, and compliance workflows. SDKs are available for JavaScript/TypeScript, Go, and Python.

## Base URL

```
https://api.assetforge.io/api/v1
```

For development/testing:
```
http://localhost:8080/api/v1
```

## Authentication

All API requests require Bearer token authentication via the `Authorization` header.

```
Authorization: Bearer YOUR_API_KEY
```

### Obtaining an API Key

1. Log in to your kor-AssetForge dashboard
2. Navigate to **Settings > API Keys**
3. Click **Create New Key**
4. Copy the key (shown only once)
5. Securely store it in your environment variables

```bash
export ASSETFORGE_API_KEY="your_api_key_here"
```

## JavaScript/TypeScript SDK

### Installation

```bash
npm install @assetforge/sdk
# or
yarn add @assetforge/sdk
```

### Initialization

```typescript
import { AssetForgeClient } from '@assetforge/sdk';

const client = new AssetForgeClient({
  apiKey: process.env.ASSETFORGE_API_KEY,
  baseUrl: 'https://api.assetforge.io', // optional
});
```

### Asset Management

#### Create Asset

```typescript
const asset = await client.assets.create({
  name: 'Premium Real Estate Token',
  type: 'real_estate',
  totalSupply: '1000000',
  fractionalUnits: 100,
  underlyingAsset: {
    location: 'New York, NY',
    valuationDate: '2026-10-15',
    estimatedValue: '5000000',
  },
});

console.log(asset.id); // ast_123abc
```

#### Get Asset

```typescript
const asset = await client.assets.get('ast_123abc');
console.log(asset.name); // 'Premium Real Estate Token'
console.log(asset.currentPrice); // current market price
```

#### List Assets

```typescript
const { assets, pagination } = await client.assets.list({
  limit: 20,
  offset: 0,
  filter: {
    type: 'real_estate',
    minPrice: 100,
    maxPrice: 10000,
  },
});

for (const asset of assets) {
  console.log(asset.id, asset.name, asset.currentPrice);
}
```

### Marketplace Operations

#### List Asset for Sale

```typescript
const listing = await client.marketplace.list({
  assetId: 'ast_123abc',
  quantity: 500,
  pricePerUnit: 100,
  description: 'Selling partial ownership',
});

console.log(listing.id); // lst_456def
```

#### Execute Trade

```typescript
const trade = await client.marketplace.trade({
  listingId: 'lst_456def',
  quantity: 250,
});

console.log(trade.id); // txn_789ghi
console.log(trade.status); // 'pending' | 'confirmed' | 'failed'
```

#### Get Trade Status

```typescript
const trade = await client.marketplace.getTrade('txn_789ghi');
console.log(trade.status);
console.log(trade.stellarTransactionHash);
```

### Portfolio Management

#### Get User Portfolio

```typescript
const portfolio = await client.portfolio.get();

console.log(portfolio.totalValue);
console.log(portfolio.holdings);
// [{
//   assetId: 'ast_123abc',
//   quantity: 500,
//   currentValue: 50000,
//   percentageChange: 12.5
// }]
```

#### Get Portfolio Analytics

```typescript
const analytics = await client.portfolio.getAnalytics({
  period: '30d', // '7d', '30d', '90d', '1y', 'all'
});

console.log(analytics.totalReturn);
console.log(analytics.volatility);
console.log(analytics.topPerformer);
```

## Go SDK

### Installation

```bash
go get github.com/parkerwinner/kor-assetforge-go-sdk
```

### Initialization

```go
package main

import (
  "github.com/parkerwinner/kor-assetforge-go-sdk"
)

func main() {
  client := assetforge.NewClient(
    assetforge.WithAPIKey(os.Getenv("ASSETFORGE_API_KEY")),
  )
  
  // Use client...
}
```

### Asset Management

```go
// Create asset
asset, err := client.Assets.Create(ctx, &assetforge.CreateAssetRequest{
  Name: "Premium Real Estate Token",
  Type: "real_estate",
  TotalSupply: "1000000",
  FractionalUnits: 100,
})

// Get asset
asset, err := client.Assets.Get(ctx, "ast_123abc")

// List assets
assets, err := client.Assets.List(ctx, &assetforge.ListAssetsOptions{
  Limit: 20,
  Type: "real_estate",
})
```

### Marketplace Operations

```go
// List for sale
listing, err := client.Marketplace.List(ctx, &assetforge.ListRequest{
  AssetID: "ast_123abc",
  Quantity: "500",
  PricePerUnit: "100",
})

// Execute trade
trade, err := client.Marketplace.Trade(ctx, &assetforge.TradeRequest{
  ListingID: "lst_456def",
  Quantity: "250",
})

// Get trade status
trade, err := client.Marketplace.GetTrade(ctx, "txn_789ghi")
```

## Python SDK

### Installation

```bash
pip install assetforge-sdk
```

### Initialization

```python
from assetforge import AssetForgeClient

client = AssetForgeClient(
    api_key=os.getenv("ASSETFORGE_API_KEY")
)
```

### Asset Management

```python
# Create asset
asset = client.assets.create(
    name="Premium Real Estate Token",
    asset_type="real_estate",
    total_supply="1000000",
    fractional_units=100,
    underlying_asset={
        "location": "New York, NY",
        "valuation_date": "2026-10-15",
        "estimated_value": "5000000"
    }
)

# Get asset
asset = client.assets.get("ast_123abc")

# List assets
assets = client.assets.list(
    limit=20,
    asset_type="real_estate",
    min_price=100,
    max_price=10000
)
```

### Marketplace Operations

```python
# List for sale
listing = client.marketplace.list(
    asset_id="ast_123abc",
    quantity=500,
    price_per_unit=100
)

# Execute trade
trade = client.marketplace.trade(
    listing_id="lst_456def",
    quantity=250
)

# Get trade status
trade = client.marketplace.get_trade("txn_789ghi")
print(trade.status)  # 'pending', 'confirmed', 'failed'
print(trade.stellar_tx_hash)
```

## REST API Endpoints

### Assets

```
GET    /assets              - List all assets
POST   /assets              - Create new asset
GET    /assets/:id          - Get asset details
PUT    /assets/:id          - Update asset
DELETE /assets/:id          - Retire asset
```

### Marketplace

```
GET    /marketplace/listings        - List active listings
POST   /marketplace/list            - Create new listing
GET    /marketplace/listings/:id    - Get listing details
POST   /marketplace/trade           - Execute trade
GET    /marketplace/trades/:id      - Get trade status
```

### Portfolio

```
GET    /portfolio           - Get user portfolio
GET    /portfolio/analytics - Get portfolio analytics
GET    /portfolio/holdings  - Get detailed holdings
```

### Compliance

```
GET    /compliance/status         - Get compliance status
POST   /compliance/kyc/verify     - Initiate KYC verification
GET    /compliance/kyc/:id        - Get verification status
```

## Error Handling

### Common Status Codes

- `200 OK` - Successful request
- `201 Created` - Resource created
- `400 Bad Request` - Invalid parameters
- `401 Unauthorized` - Missing or invalid API key
- `403 Forbidden` - Insufficient permissions
- `404 Not Found` - Resource not found
- `429 Too Many Requests` - Rate limit exceeded
- `500 Internal Server Error` - Server error

### Error Response Format

```json
{
  "error": {
    "code": "INSUFFICIENT_BALANCE",
    "message": "Wallet has insufficient balance for this transaction",
    "details": {
      "required": "500",
      "available": "250"
    }
  }
}
```

### Handling Errors (JavaScript)

```typescript
try {
  const trade = await client.marketplace.trade({
    listingId: 'lst_456def',
    quantity: 1000,
  });
} catch (error) {
  if (error.code === 'INSUFFICIENT_BALANCE') {
    console.log(`Need ${error.details.required}, have ${error.details.available}`);
  } else {
    console.error('Unknown error:', error.message);
  }
}
```

## Rate Limiting

API requests are rate-limited to **100 requests per minute** per API key.

Response headers indicate rate limit status:

```
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 87
X-RateLimit-Reset: 1697462400
```

## Pagination

List endpoints support cursor-based pagination:

```typescript
const response = await client.assets.list({
  limit: 20,
  cursor: 'eyJpZCI6IGFzdF8xMjNhYmMifQ=='
});

if (response.pagination.hasMore) {
  const nextPage = await client.assets.list({
    limit: 20,
    cursor: response.pagination.nextCursor
  });
}
```

## Webhooks

For real-time event notifications, configure webhooks in your dashboard. See [Webhook Documentation](./webhooks.md) for details.

## Support & Resources

- API Status: https://status.assetforge.io
- Documentation: https://docs.assetforge.io
- GitHub Examples: https://github.com/parkerwinner/kor-assetforge-examples
- Support Email: support@assetforge.io
- Slack Community: [Join our Slack](https://slack.assetforge.io)

