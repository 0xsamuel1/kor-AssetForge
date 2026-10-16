#!/usr/bin/env bash
# test_restore.sh — Automated restore rehearsal.
#
# Creates a backup from a source database, restores it into a throwaway
# database, and verifies the restored schema matches. This is the monthly
# restore test referenced by docs/DISASTER_RECOVERY.md and
# .github/workflows/backup_test.yml.
#
# Required environment variables:
#   DATABASE_URL            Source database to back up
#
# Optional:
#   RESTORE_TEST_DATABASE_URL   Target DB for the rehearsal. Defaults to a
#                               scratch database created next to DATABASE_URL.
#   BACKUP_TEMP_DIR             Where the temporary dump is written (default: /tmp/kor-restore-test)
#   KEEP_TEST_DB               "true" to leave the scratch database in place
#
# Exit codes: 0 pass, 1 setup error, 2 backup failure, 3 restore failure, 4 verification failure

set -euo pipefail

log() { echo "[test-restore] $*"; }
die() { echo "[test-restore] ERROR: $*" >&2; exit "${2:-1}"; }

DATABASE_URL="${DATABASE_URL:-}"
BACKUP_TEMP_DIR="${BACKUP_TEMP_DIR:-/tmp/kor-restore-test}"
KEEP_TEST_DB="${KEEP_TEST_DB:-false}"
SCRATCH_DB="kor_restore_test_$$"

[ -n "${DATABASE_URL}" ] || die "DATABASE_URL is required"

command -v pg_dump >/dev/null 2>&1 || die "pg_dump not found"
command -v pg_restore >/dev/null 2>&1 || die "pg_restore not found"
command -v psql >/dev/null 2>&1 || die "psql not found"

mkdir -p "${BACKUP_TEMP_DIR}"
DUMP_FILE="${BACKUP_TEMP_DIR}/restore_test_$(date -u +%Y%m%d_%H%M%S).dump"

# Derive admin connection (to the maintenance DB) and scratch target URL.
# Strip any existing path/query from the source URL for the admin connection.
base_url="${DATABASE_URL%%\?*}"
admin_url="${base_url%/*}/postgres"
RESTORE_TEST_DATABASE_URL="${RESTORE_TEST_DATABASE_URL:-${base_url%/*}/${SCRATCH_DB}}"

cleanup() {
    if [ "${KEEP_TEST_DB}" != "true" ]; then
        log "Dropping scratch database ${SCRATCH_DB}"
        psql "${admin_url}" -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS \"${SCRATCH_DB}\";" >/dev/null 2>&1 || true
    fi
    rm -f "${DUMP_FILE}"
}
trap cleanup EXIT

# --- 1. Back up ---
log "Backing up source database"
pg_dump -Fc "${DATABASE_URL}" > "${DUMP_FILE}" || die "pg_dump failed" 2
pg_restore -l "${DUMP_FILE}" >/dev/null 2>&1 || die "produced dump is not readable" 2
log "Backup created: ${DUMP_FILE} ($(du -sh "${DUMP_FILE}" | cut -f1))"

# --- 2. Prepare scratch database ---
log "Creating scratch database ${SCRATCH_DB}"
psql "${admin_url}" -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS \"${SCRATCH_DB}\";" >/dev/null
psql "${admin_url}" -v ON_ERROR_STOP=1 -c "CREATE DATABASE \"${SCRATCH_DB}\";" >/dev/null

# --- 3. Restore ---
log "Restoring into scratch database"
set +e
pg_restore -d "${RESTORE_TEST_DATABASE_URL}" --no-owner --no-privileges -j 2 "${DUMP_FILE}"
rc=$?
set -e
[ "${rc}" -ge 2 ] && die "pg_restore failed with exit code ${rc}" 3

# --- 4. Verify source and restored table counts match ---
log "Verifying restored schema"
count_tables() {
    psql "$1" -tAc "SELECT count(*) FROM information_schema.tables WHERE table_schema='public';"
}
src_tables="$(count_tables "${DATABASE_URL}")"
dst_tables="$(count_tables "${RESTORE_TEST_DATABASE_URL}")"
log "Source tables: ${src_tables}, restored tables: ${dst_tables}"

if [ "${src_tables}" -ne "${dst_tables}" ]; then
    die "table count mismatch (source=${src_tables}, restored=${dst_tables})" 4
fi

log "Restore rehearsal PASSED at $(date -u)"
