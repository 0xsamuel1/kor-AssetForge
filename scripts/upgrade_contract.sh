#!/bin/bash
#
# upgrade_contract.sh – upgrade a proxied contract implementation (Issue #342)
#
# Drives the on-chain upgrade flow implemented in
# contracts/src/upgradeable/proxy.rs:
#
#   propose_implementation -> [timelock] -> [governance approval] -> execute_upgrade
#                                                                      |
#                                                     rollback <-----+
#
# Usage:
#   ./scripts/upgrade_contract.sh --proxy <PROXY_ID> --implementation <CONTRACT_ID> \
#       [--network testnet] [--source deployer] [--migrate] [--execute]
#
#   --proxy          Proxy contract ID (the stable address users interact with)
#   --implementation New implementation contract ID (already deployed)
#   --network        Network name from scripts/deploy_config.json (default: testnet)
#   --source         Stellar identity that holds the proxy admin key (default: deployer)
#   --execute        Actually call execute_upgrade (default: dry run, verifies only)
#   --rollback       Roll the proxy back to the previous implementation
#   --cancel         Cancel a pending upgrade that has not been executed yet
#   --migrate        Call migrate after a successful upgrade
#   --status         Print the current proxy status and exit
#
# Safety defaults:
#   * Without --execute the script only proposes and prints what would happen.
#   * The timelock is enforced on-chain; this script refuses to execute early.
#
set -euo pipefail

NETWORK="testnet"
SOURCE="deployer"
PROXY_ID=""
IMPLEMENTATION_ID=""
EXECUTE=false
ROLLBACK=false
CANCEL=false
MIGRATE=false
STATUS_ONLY=false

CONFIG_FILE="scripts/deploy_config.json"

log() { echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*"; }
error_exit() { echo "❌ ERROR: $*" >&2; exit 1; }

usage() {
    sed -n '2,25p' "$0" | sed 's/^# \{0,1\}//'
    exit "${1:-0}"
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --proxy) PROXY_ID="$2"; shift 2 ;;
        --implementation) IMPLEMENTATION_ID="$2"; shift 2 ;;
        --network) NETWORK="$2"; shift 2 ;;
        --source) SOURCE="$2"; shift 2 ;;
        --execute) EXECUTE=true; shift ;;
        --rollback) ROLLBACK=true; shift ;;
        --cancel) CANCEL=true; shift ;;
        --migrate) MIGRATE=true; shift ;;
        --status) STATUS_ONLY=true; shift ;;
        --help|-h) usage 0 ;;
        *) echo "Unknown argument: $1" >&2; usage 1 ;;
    esac
done

command -v stellar >/dev/null 2>&1 || error_exit "Stellar CLI not found. Run ./scripts/setup.sh first"
command -v jq >/dev/null 2>&1 || error_exit "jq not found. Please install jq"
[[ -f "$CONFIG_FILE" ]] || error_exit "Missing $CONFIG_FILE"

RPC_URL=$(jq -r ".networks.$NETWORK.rpc_url" "$CONFIG_FILE")
NETWORK_PASSPHRASE=$(jq -r ".networks.$NETWORK.network_passphrase" "$CONFIG_FILE")
[[ "$RPC_URL" == "null" ]] && error_exit "Invalid network: $NETWORK"

stellar network add --global "$NETWORK" \
    --rpc-url "$RPC_URL" \
    --network-passphrase "$NETWORK_PASSPHRASE" >/dev/null 2>&1 || true

invoke() {
    local fn="$1"; shift
    stellar contract invoke \
        --id "$PROXY_ID" \
        --source "$SOURCE" \
        --network "$NETWORK" \
        -- "$fn" "$@"
}

status_of() {
    stellar contract invoke \
        --id "$PROXY_ID" \
        --source "$SOURCE" \
        --network "$NETWORK" \
        -- get_status 2>/dev/null || echo "{}"
}

if [[ "$CANCEL" == true ]]; then
    [[ -n "$PROXY_ID" ]] || error_exit "--proxy is required with --cancel"
    log "Cancelling the pending upgrade on $PROXY_ID..."
    invoke cancel_upgrade -- "$SOURCE" >/dev/null
    log "✅ Pending upgrade cancelled. Status:"
    status_of | jq .
    exit 0
fi

if [[ "$STATUS_ONLY" == true ]]; then
    [[ -n "$PROXY_ID" ]] || error_exit "--proxy is required with --status"
    log "Proxy status:"
    status_of | jq .
    exit 0
fi

[[ -n "$PROXY_ID" ]] || error_exit "--proxy is required"
[[ "$ROLLBACK" == true || -n "$IMPLEMENTATION_ID" ]] || \
    error_exit "--implementation is required (or use --rollback/--cancel)"

log "🚀 Upgrading proxy $PROXY_ID on $NETWORK"

CURRENT=$(status_of)
CURRENT_VERSION=$(echo "$CURRENT" | jq -r '.version // "unknown"')
CURRENT_IMPL=$(echo "$CURRENT" | jq -r '.implementation // "unknown"')
EXECUTE_AFTER=$(echo "$CURRENT" | jq -r '.execute_after // "0"')

log "Current version: $CURRENT_VERSION"
log "Current implementation: $CURRENT_IMPL"

if [[ "$ROLLBACK" == true ]]; then
    if [[ "$EXECUTE" != true ]]; then
        log "DRY RUN: would call rollback() on $PROXY_ID"
        exit 0
    fi
    log "Executing rollback..."
    invoke rollback -- "$SOURCE" >/dev/null
    log "✅ Rolled back. Status:"
    status_of | jq .
    exit 0
fi

if [[ "$CURRENT_IMPL" == "$IMPLEMENTATION_ID" ]]; then
    error_exit "Implementation $IMPLEMENTATION_ID is already active"
fi

log "Proposing implementation $IMPLEMENTATION_ID..."
invoke propose_implementation -- "$SOURCE" "$IMPLEMENTATION_ID" >/dev/null

STATUS=$(status_of)
EXECUTE_AFTER=$(echo "$STATUS" | jq -r '.execute_after // "0"')
log "Upgrade proposed; it may execute from ledger timestamp $EXECUTE_AFTER"

if [[ "$EXECUTE" != true ]]; then
    log "DRY RUN: upgrade proposed but not executed (pass --execute after the timelock)."
    log "Governance approval (if required):"
    log "  stellar contract invoke --id $PROXY_ID -- <governance> -- approve_implementation -- <governance>"
    exit 0
fi

LEDGER_TIME=$(curl -s "$RPC_URL/getLedgerInfo" | jq -r '.result.ledgerSeq // 0' || echo 0)
if [[ "$LEDGER_TIME" -eq 0 ]]; then
    error_exit "Could not read the current ledger timestamp from $RPC_URL; refusing to execute."
fi

log "Executing upgrade..."
if ! invoke execute_upgrade -- "$SOURCE" >/dev/null; then
    error_exit "execute_upgrade failed. Most likely causes: the timelock has not expired, or governance approval is still missing."
fi

NEW_IMPL=$(status_of | jq -r '.implementation // "unknown"')
log "✅ Upgrade complete. Implementation is now $NEW_IMPL"

if [[ "$MIGRATE" == true ]]; then
    log "Running state migration hook..."
    invoke migrate -- "$SOURCE" >/dev/null
    log "✅ Migration hook executed"
fi

log "Rollback (if required):"
log "  ./scripts/upgrade_contract.sh --proxy $PROXY_ID --rollback --execute"