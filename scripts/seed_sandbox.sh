#!/usr/bin/env bash
# seed_sandbox.sh — Populate the sandbox database with realistic test data.
#
# Usage:
#   ./scripts/seed_sandbox.sh
#
# Environment variables (all optional, sensible defaults provided):
#   API_BASE_URL          Base URL of the running sandbox API  (default: http://localhost:8080)
#   SANDBOX_ADMIN_EMAIL   Email for the seeded admin account   (default: sandbox@kor-assetforge.dev)
#   SANDBOX_ADMIN_PASS    Password for the seeded admin account(default: SandboxPass123!)
#
# Prerequisites: curl, jq
#
# The script is idempotent — running it multiple times is safe.

set -euo pipefail

API="${API_BASE_URL:-http://localhost:8080}"
ADMIN_EMAIL="${SANDBOX_ADMIN_EMAIL:-sandbox@kor-assetforge.dev}"
ADMIN_PASS="${SANDBOX_ADMIN_PASS:-SandboxPass123!}"

# ─── Helpers ──────────────────────────────────────────────────────────────────

info()  { echo "[seed] $*"; }
error() { echo "[seed] ERROR: $*" >&2; exit 1; }

require_cmd() {
  command -v "$1" &>/dev/null || error "'$1' is required but not installed."
}

post() {
  local path="$1"
  local body="$2"
  local token="${3:-}"
  local auth_header=""
  [[ -n "$token" ]] && auth_header="-H \"Authorization: Bearer $token\""

  curl -s -X POST "${API}${path}" \
    -H "Content-Type: application/json" \
    ${token:+-H "Authorization: Bearer $token"} \
    -d "$body"
}

# ─── Preflight ────────────────────────────────────────────────────────────────

require_cmd curl
require_cmd jq

info "Target API: $API"
info "Checking API health..."
health=$(curl -s -o /dev/null -w "%{http_code}" "${API}/health")
[[ "$health" == "200" ]] || error "API health check failed (HTTP $health). Is the sandbox server running?"

# ─── 1. Register / login admin user ───────────────────────────────────────────

info "Registering sandbox admin user..."
reg_resp=$(post "/api/v1/auth/register" "{
  \"email\": \"${ADMIN_EMAIL}\",
  \"password\": \"${ADMIN_PASS}\",
  \"full_name\": \"Sandbox Admin\"
}" 2>/dev/null || true)

info "Logging in as sandbox admin..."
login_resp=$(post "/api/v1/auth/login" "{
  \"email\": \"${ADMIN_EMAIL}\",
  \"password\": \"${ADMIN_PASS}\"
}")

TOKEN=$(echo "$login_resp" | jq -r '.token // .access_token // empty')
[[ -n "$TOKEN" ]] || error "Login failed. Response: $login_resp"
info "Authenticated. Token acquired."

# ─── 2. Seed test assets ──────────────────────────────────────────────────────

info "Seeding test assets..."

post "/api/v1/assets" '{
  "name": "Sandbox Office Building",
  "description": "A test commercial real-estate asset for sandbox exploration.",
  "asset_type": "real_estate",
  "total_value": 1000000,
  "currency": "USD",
  "total_tokens": 1000,
  "token_price": 1000
}' "$TOKEN" | jq -r '"  Created asset: " + (.name // "unknown")' 2>/dev/null || info "  Asset already exists or creation skipped."

post "/api/v1/assets" '{
  "name": "Sandbox Art Collection",
  "description": "A fractional art investment token for sandbox testing.",
  "asset_type": "art",
  "total_value": 250000,
  "currency": "USD",
  "total_tokens": 2500,
  "token_price": 100
}' "$TOKEN" | jq -r '"  Created asset: " + (.name // "unknown")' 2>/dev/null || info "  Asset already exists or creation skipped."

post "/api/v1/assets" '{
  "name": "Sandbox Gold Reserve",
  "description": "Tokenized gold reserve — sandbox only.",
  "asset_type": "commodity",
  "total_value": 500000,
  "currency": "USD",
  "total_tokens": 500,
  "token_price": 1000
}' "$TOKEN" | jq -r '"  Created asset: " + (.name // "unknown")' 2>/dev/null || info "  Asset already exists or creation skipped."

# ─── 3. Seed test users ───────────────────────────────────────────────────────

info "Seeding test user accounts..."

for i in 1 2 3; do
  post "/api/v1/auth/register" "{
    \"email\": \"sandbox-user${i}@kor-assetforge.dev\",
    \"password\": \"TestUser${i}Pass!\",
    \"full_name\": \"Sandbox User ${i}\"
  }" 2>/dev/null | jq -r '"  Registered: " + (.email // "already exists")' 2>/dev/null || true
done

# ─── 4. Done ──────────────────────────────────────────────────────────────────

info ""
info "Sandbox seeding complete."
info ""
info "Credentials:"
info "  Admin:  $ADMIN_EMAIL / $ADMIN_PASS"
info "  Users:  sandbox-user{1,2,3}@kor-assetforge.dev / TestUser{1,2,3}Pass!"
info ""
info "Swagger UI: ${API}/swagger/index.html"
info "To reset sandbox data, call: POST ${API}/api/v1/sandbox/reset  (admin token required)"
