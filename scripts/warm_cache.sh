#!/usr/bin/env bash
# ==============================================================================
# AssetForge Cache Warming Script
# Pre-populates L1/L2 caches with high-priority platform data for cold starts.
# ==============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
BACKEND_DIR="${ROOT_DIR}/backend"

CONCURRENCY="${CONCURRENCY:-4}"
TARGET="${TARGET:-all}"
TIMEOUT="${TIMEOUT:-2m}"

show_help() {
  cat << EOF
Usage: $(basename "$0") [OPTIONS]

AssetForge Cache Warmer Utility

Options:
  -c, --concurrency <num>   Number of concurrent worker routines (default: ${CONCURRENCY})
  -t, --target <group>      Target data group: all, assets, taxonomy, metrics (default: ${TARGET})
  -w, --timeout <duration>  Timeout for warming run (default: ${TIMEOUT})
  -h, --help                Show this help message

Environment variables:
  DATABASE_URL              PostgreSQL connection string
  REDIS_URL                 Redis connection string
  CONCURRENCY               Default worker concurrency
  TARGET                    Default target dataset
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -c|--concurrency)
      CONCURRENCY="$2"
      shift 2
      ;;
    -t|--target)
      TARGET="$2"
      shift 2
      ;;
    -w|--timeout)
      TIMEOUT="$2"
      shift 2
      ;;
    -h|--help)
      show_help
      exit 0
      ;;
    *)
      echo "Unknown argument: $1"
      show_help
      exit 1
      ;;
  esac
done

echo "=========================================================="
echo " Starting AssetForge Cache Warming"
echo " Target:      ${TARGET}"
echo " Concurrency: ${CONCURRENCY}"
echo " Timeout:     ${TIMEOUT}"
echo "=========================================================="

cd "${BACKEND_DIR}"

if command -v go >/dev/null 2>&1; then
  go run ./cmd/warm-cache \
    -concurrency="${CONCURRENCY}" \
    -target="${TARGET}" \
    -timeout="${TIMEOUT}"
else
  echo "Error: 'go' binary not found in PATH." >&2
  exit 1
fi

echo "Cache warming execution finished successfully."
