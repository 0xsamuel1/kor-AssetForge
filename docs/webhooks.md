# Webhook Documentation

This document describes the webhook system for kor-AssetForge, including payload structures, validation requirements, and security measures.

## Overview

Webhooks allow your application to receive real-time notifications about events occurring in the kor-AssetForge platform. When an event is triggered, a POST request is sent to your registered webhook endpoint with the relevant event data.

## Webhook Events

### Asset Events

#### `asset.created`
Triggered when a new asset is tokenized and added to the registry.

**Payload:**
```json
{
  "event": "asset.created",
  "timestamp": "2026-10-16T12:00:00Z",
  "data": {
    "assetId": "ast_123abc",
    "assetName": "Premium Real Estate Token",
    "issuer": "addr_issuer_123",
    "totalSupply": "1000000",
    "fractionalUnits": "100",
    "underlyingAssetType": "real_estate",
    "metadata": {
      "location": "New York, NY",
      "valuationDate": "2026-10-15",
      "estimatedValue": "5000000"
    }
  }
}
```

#### `asset.updated`
Triggered when asset details or metadata are modified.

**Payload:**
```json
{
  "event": "asset.updated",
  "timestamp": "2026-10-16T12:00:00Z",
  "data": {
    "assetId": "ast_123abc",
    "previousData": { /* ... */ },
    "updatedData": { /* ... */ }
  }
}
```

### Marketplace Events

#### `trade.completed`
Triggered when an asset trade is successfully executed on-chain.

**Payload:**
```json
{
  "event": "trade.completed",
  "timestamp": "2026-10-16T12:00:00Z",
  "data": {
    "transactionId": "txn_456def",
    "buyer": "addr_buyer_123",
    "seller": "addr_seller_456",
    "assetId": "ast_123abc",
    "quantity": "500",
    "pricePerUnit": "100",
    "totalPrice": "50000",
    "contractId": "cont_789ghi",
    "stellarTransactionHash": "hash_xyz123"
  }
}
```

#### `transfer.pending_approval`
Triggered when a fractional transfer requires approval due to compliance restrictions.

**Payload:**
```json
{
  "event": "transfer.pending_approval",
  "timestamp": "2026-10-16T12:00:00Z",
  "data": {
    "transferId": "trf_999jkl",
    "from": "addr_from_123",
    "to": "addr_to_456",
    "assetId": "ast_123abc",
    "quantity": "250",
    "reason": "whitelist_verification_required",
    "approvalDeadline": "2026-10-23T12:00:00Z"
  }
}
```

### Compliance Events

#### `compliance.kyc_verified`
Triggered when a user's KYC verification is completed successfully.

**Payload:**
```json
{
  "event": "compliance.kyc_verified",
  "timestamp": "2026-10-16T12:00:00Z",
  "data": {
    "userId": "user_123",
    "verificationLevel": "accredited_investor",
    "documentType": "passport",
    "verificationExpiry": "2027-10-16T00:00:00Z"
  }
}
```

## Payload Validation

All webhook payloads include a signature header for verification purposes.

### Signature Verification

Each webhook request includes an `X-Webhook-Signature` header containing an HMAC-SHA256 signature of the request body.

**Steps to verify:**

1. Retrieve your webhook secret from the dashboard
2. Get the raw request body (as received, no parsing)
3. Generate HMAC-SHA256 signature: `signature = HMAC-SHA256(webhook_secret, request_body)`
4. Compare with the header value

**Example (Node.js):**
```javascript
const crypto = require('crypto');

function verifyWebhookSignature(body, signature, secret) {
  const computed = crypto
    .createHmac('sha256', secret)
    .update(JSON.stringify(body))
    .digest('hex');
  
  return crypto.timingSafeEqual(
    Buffer.from(signature),
    Buffer.from(computed)
  );
}
```

**Example (Go):**
```go
import (
  "crypto/hmac"
  "crypto/sha256"
  "encoding/hex"
  "encoding/json"
)

func VerifyWebhookSignature(body []byte, signature, secret string) bool {
  h := hmac.New(sha256.New, []byte(secret))
  h.Write(body)
  computed := hex.EncodeToString(h.Sum(nil))
  return hmac.Equal([]byte(signature), []byte(computed))
}
```

## Payload Schema Validation

All payloads conform to the following base schema:

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": ["event", "timestamp", "data"],
  "properties": {
    "event": {
      "type": "string",
      "description": "The type of event"
    },
    "timestamp": {
      "type": "string",
      "format": "date-time",
      "description": "ISO 8601 timestamp when the event occurred"
    },
    "data": {
      "type": "object",
      "description": "Event-specific data payload"
    }
  }
}
```

## Validation Rules

### Required Fields
- `event`: Must be one of the documented event types
- `timestamp`: Must be a valid ISO 8601 datetime string
- `data`: Must be an object containing event-specific fields

### Data Type Validation

Each event type enforces specific data types:

**asset.created:**
- `assetId`: string, format: `ast_[alphanumeric]`
- `totalSupply`: string (numeric value)
- `fractionalUnits`: number (positive integer)
- `issuer`: string, format: Stellar address

**trade.completed:**
- `quantity`: string (numeric value)
- `pricePerUnit`: string (numeric value)
- `buyer`, `seller`: strings, format: Stellar addresses
- `stellarTransactionHash`: string, format: hex

## Retry Policy

If your webhook endpoint returns a status code other than 2xx, we will retry with exponential backoff:

- Attempt 1: Immediately
- Attempt 2: 5 minutes later
- Attempt 3: 30 minutes later
- Attempt 4: 2 hours later
- Attempt 5: 24 hours later

**Total retry window: 26.5 hours**

After the final retry, the webhook delivery is marked as failed and logged in your webhook event history.

## Best Practices

1. **Validate Signatures**: Always verify the signature before processing the payload
2. **Idempotency**: Implement idempotent handlers to safely process duplicate webhooks
3. **Quick Responses**: Return a 2xx status code immediately, process asynchronously
4. **Logging**: Log all webhook events for debugging and audit purposes
5. **Error Handling**: Don't throw exceptions during webhook processing; log and continue
6. **Rate Limiting**: Design your endpoint to handle burst traffic

## Webhook Management

### Registering a Webhook

Navigate to your dashboard's **Settings > Webhooks** and add a new endpoint:

1. Enter your webhook URL
2. Select which events to subscribe to
3. Copy your webhook secret for signature verification
4. Test the endpoint with a sample payload

### Monitoring Webhook Delivery

Track webhook delivery status in the dashboard:

- View delivery history for each event
- See response status codes and timing
- Retry failed deliveries manually
- Monitor for patterns in failures

## Examples

### Python Integration

```python
import hmac
import hashlib
import json
from flask import Flask, request

app = Flask(__name__)
WEBHOOK_SECRET = "your_webhook_secret"

def verify_signature(body, signature):
    computed = hmac.new(
        WEBHOOK_SECRET.encode(),
        body.encode(),
        hashlib.sha256
    ).hexdigest()
    return hmac.compare_digest(signature, computed)

@app.route('/webhooks/assetforge', methods=['POST'])
def handle_webhook():
    signature = request.headers.get('X-Webhook-Signature')
    if not verify_signature(request.get_data(as_text=True), signature):
        return {'error': 'Invalid signature'}, 401
    
    payload = request.json
    event_type = payload.get('event')
    event_data = payload.get('data')
    
    if event_type == 'trade.completed':
        # Handle trade completion
        process_trade(event_data)
    elif event_type == 'asset.created':
        # Handle new asset
        handle_new_asset(event_data)
    
    return {'status': 'received'}, 200
```

## Support

For webhook integration support, please:

- Check the [API Documentation](./API.md)
- Review webhook logs in the dashboard
- Contact support@assetforge.io

