#!/usr/bin/env bash
# generate_secrets.sh — Create Docker secrets for the production compose stack.
#
# Writes random, high-entropy values to ./secrets/*.txt (gitignored, mode 600).
# Existing files are left untouched unless FORCE=true.
#
# Usage:
#   ./scripts/generate_secrets.sh
#   SECRETS_DIR=/etc/assetforge/secrets ./scripts/generate_secrets.sh
#
# These files are consumed by docker-compose.prod.yml (see docs/DOCKER_DEPLOYMENT.md).

set -euo pipefail

SECRETS_DIR="${SECRETS_DIR:-./secrets}"
FORCE="${FORCE:-false}"

mkdir -p "${SECRETS_DIR}"
chmod 700 "${SECRETS_DIR}"

gen() {
    local name="$1" bytes="${2:-32}" file="${SECRETS_DIR}/$1.txt"
    if [ -f "${file}" ] && [ "${FORCE}" != "true" ]; then
        echo "[secrets] keeping existing ${file}"
        return
    fi
    if command -v openssl >/dev/null 2>&1; then
        openssl rand -base64 "${bytes}" | tr -d '\n' > "${file}"
    else
        LC_ALL=C tr -dc 'A-Za-z0-9_@%+=:,./-' </dev/urandom | head -c "$((bytes * 2))" > "${file}"
    fi
    chmod 600 "${file}"
    echo "[secrets] wrote ${file}"
}

gen postgres_password
gen redis_password
gen jwt_secret 48
gen backup_encryption_key 32

echo "[secrets] done. Secrets live in ${SECRETS_DIR} (mode 600). Do not commit them."
