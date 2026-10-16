# API Sandbox Environment

The kor-AssetForge sandbox provides a fully functional copy of the API that developers can use to test integrations without touching production data or triggering real payments, emails, or blockchain transactions.

---

## Quick Start

### 1. Start the Sandbox Server

```bash
APP_ENV=sandbox \
DATABASE_URL=postgresql://postgres:password@localhost:5432/assetforge_sandbox?sslmode=disable \
STELLAR_NETWORK=testnet \
go run ./backend
```

### 2. Seed Test Data

```bash
./scripts/seed_sandbox.sh
```

This creates:
- 1 sandbox admin account
- 3 sandbox user accounts
- 3 test assets (real estate, art, commodity)

### 3. Open the API Playground

Navigate to [http://localhost:8080/swagger/index.html](http://localhost:8080/swagger/index.html) to explore and call every endpoint interactively.

---

## Environment Setup

| Variable                  | Sandbox Value                                               | Description                                |
|---------------------------|-------------------------------------------------------------|--------------------------------------------|
| `APP_ENV`                 | `sandbox`                                                   | **Required.** Enables sandbox mode.        |
| `DATABASE_URL`            | `...assetforge_sandbox...`                                  | Use a dedicated database.                  |
| `STELLAR_NETWORK`         | `testnet`                                                   | Always testnet in sandbox.                 |
| `STELLAR_HORIZON_URL`     | `https://horizon-testnet.stellar.org`                       | Testnet Horizon.                           |
| `SANDBOX_ADMIN_EMAIL`     | `sandbox@kor-assetforge.dev`                                | Pre-seeded admin email.                    |
| `SANDBOX_ADMIN_PASSWORD`  | `SandboxPass123!`                                           | Pre-seeded admin password.                 |
| `SANDBOX_RESET_ENABLED`   | `true`                                                      | Allow `/sandbox/reset` endpoint.           |
| `REDIS_URL`               | `localhost:6379`                                            | Can share Redis, or use a dedicated DB.    |

See `backend/.env.example` for a full list of available environment variables.

---

## Sandbox Behavior

When `APP_ENV=sandbox` is set, the following changes apply automatically:

| Feature                   | Normal behavior             | Sandbox behavior                           |
|---------------------------|-----------------------------|--------------------------------------------|
| Payment processing        | Real Stripe / PayPal charge | Simulated — no real money moves            |
| Email delivery            | Emails sent via SMTP        | Emails logged to stdout, not delivered     |
| Stellar transactions      | Mainnet blockchain writes   | Testnet only                               |
| KYC provider calls        | Real KYC API                | Mock provider returns approved             |
| Response headers          | Normal headers              | `X-Sandbox-Mode: true` on every response  |

### Identifying Sandbox Responses

Every sandbox response includes:

```
X-Sandbox-Mode: true
X-Sandbox-Warning: This is a sandbox environment. No real transactions will occur.
```

---

## Separate Database

**Use a dedicated database for sandbox.** This ensures sandbox test data never mixes with production records and allows safe full-reset operations.

```bash
# Create the sandbox database
createdb assetforge_sandbox

# Run migrations
DATABASE_URL=postgresql://postgres:password@localhost:5432/assetforge_sandbox?sslmode=disable \
  go run ./backend/cmd/migrate
```

---

## Seeding Test Data

The seed script `scripts/seed_sandbox.sh` is idempotent and can be re-run safely.

```bash
# Default configuration
./scripts/seed_sandbox.sh

# Custom API URL
API_BASE_URL=http://staging.example.com ./scripts/seed_sandbox.sh
```

### What Gets Seeded

| Resource             | Details                                                      |
|----------------------|--------------------------------------------------------------|
| Admin user           | `sandbox@kor-assetforge.dev` / `SandboxPass123!`            |
| Regular users        | `sandbox-user{1,2,3}@kor-assetforge.dev`                    |
| Real estate asset    | "Sandbox Office Building" — $1,000,000 / 1,000 tokens       |
| Art asset            | "Sandbox Art Collection" — $250,000 / 2,500 tokens          |
| Commodity asset      | "Sandbox Gold Reserve" — $500,000 / 500 tokens              |

---

## Reset Mechanism

To wipe all sandbox data and re-seed from scratch:

```bash
# 1. Drop and recreate the database
dropdb assetforge_sandbox
createdb assetforge_sandbox

# 2. Re-run migrations
DATABASE_URL=postgresql://postgres:password@localhost:5432/assetforge_sandbox?sslmode=disable \
  go run ./backend/cmd/migrate

# 3. Re-seed
./scripts/seed_sandbox.sh
```

---

## Swagger / API Playground

The embedded Swagger UI is available at `/swagger/index.html` and lets you:

- Browse every available endpoint with descriptions and schemas
- Authenticate via the **Authorize** button (paste a Bearer token)
- Send live requests against the sandbox and inspect responses

To get a token for the Swagger UI:

```bash
curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"sandbox@kor-assetforge.dev","password":"SandboxPass123!"}' \
  | jq -r '.token'
```

Paste the token into the Swagger **Authorize** dialog as `Bearer <token>`.

---

## Test Payments

In sandbox mode, payments go through the same handler code but external gateway calls are suppressed. Use these test card / PayPal credentials:

### Stripe Test Cards

| Card Number          | Scenario             |
|----------------------|----------------------|
| `4242 4242 4242 4242` | Payment succeeds    |
| `4000 0000 0000 0002` | Card declined       |
| `4000 0025 0000 3155` | 3DS authentication  |

Use any future expiry date and any 3-digit CVC.

### PayPal Sandbox

Use the PayPal developer sandbox credentials from your [PayPal Developer Dashboard](https://developer.paypal.com/dashboard/).

---

## Related Files

| File                                      | Purpose                                              |
|-------------------------------------------|------------------------------------------------------|
| `backend/config/environment.go`          | `IsSandbox()`, `CurrentEnvironment()` helpers        |
| `backend/middleware/sandbox.go`          | Sandbox response headers and context flag            |
| `scripts/seed_sandbox.sh`               | Test data seeding script                             |
| `backend/.env.example`                  | Full list of environment variables                   |
| `docs/MULTI_CURRENCY_SUPPORT.md`        | Currency handling documentation                      |
