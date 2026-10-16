#!/usr/bin/env bash
# restore_backup.sh — Restore a PostgreSQL backup produced by scripts/backup_db.sh.
#
# Required environment variables:
#   DATABASE_URL            Target PostgreSQL connection string
#
# Backup source (choose one):
#   BACKUP_FILE             Path to a local .dump file
#   BACKUP_S3_BUCKET        S3 bucket holding the backup (latest is downloaded if BACKUP_FILE unset)
#
# Optional:
#   BACKUP_S3_REGION        AWS region (default: us-east-1)
#   BACKUP_S3_PREFIX        S3 key prefix (default: kor-assetforge/backups)
#   BACKUP_S3_KEY           Explicit S3 object key to restore
#   RESTORE_CLEAN           "true" to drop objects before restore (default: false)
#   RESTORE_TEMP_DIR        Local download directory (default: /tmp/kor-restore)
#   RESTORE_JOBS            Parallel restore jobs (default: 4)
#   RESTORE_VERIFY          "false" to skip post-restore verification (default: true)
#
# Exit codes:
#   0 success, 1 usage/config error, 2 restore failure, 3 verification failure

set -euo pipefail

log() { echo "[restore] $*"; }
die() { echo "[restore] ERROR: $*" >&2; exit "${2:-1}"; }

DATABASE_URL="${DATABASE_URL:-}"
BACKUP_FILE="${BACKUP_FILE:-}"
BACKUP_S3_BUCKET="${BACKUP_S3_BUCKET:-}"
BACKUP_S3_REGION="${BACKUP_S3_REGION:-us-east-1}"
BACKUP_S3_PREFIX="${BACKUP_S3_PREFIX:-kor-assetforge/backups}"
BACKUP_S3_KEY="${BACKUP_S3_KEY:-}"
RESTORE_CLEAN="${RESTORE_CLEAN:-false}"
RESTORE_TEMP_DIR="${RESTORE_TEMP_DIR:-/tmp/kor-restore}"
RESTORE_JOBS="${RESTORE_JOBS:-4}"
RESTORE_VERIFY="${RESTORE_VERIFY:-true}"

command -v pg_restore >/dev/null 2>&1 || die "pg_restore not found in PATH"
[ -n "${DATABASE_URL}" ] || die "DATABASE_URL is required"

# --- Resolve the backup file (local or S3) ---
if [ -z "${BACKUP_FILE}" ]; then
    command -v aws >/dev/null 2>&1 || die "aws CLI required when BACKUP_FILE is not set"
    [ -n "${BACKUP_S3_BUCKET}" ] || die "BACKUP_S3_BUCKET or BACKUP_FILE is required"
    mkdir -p "${RESTORE_TEMP_DIR}"

    if [ -z "${BACKUP_S3_KEY}" ]; then
        log "Resolving latest backup under s3://${BACKUP_S3_BUCKET}/${BACKUP_S3_PREFIX}/"
        BACKUP_S3_KEY="$(aws s3api list-objects-v2 \
            --bucket "${BACKUP_S3_BUCKET}" \
            --prefix "${BACKUP_S3_PREFIX}/" \
            --query 'reverse(sort_by(Contents[?contains(Key, `.dump`)], &LastModified))[0].Key' \
            --output text \
            --region "${BACKUP_S3_REGION}")"
        [ "${BACKUP_S3_KEY}" != "None" ] && [ -n "${BACKUP_S3_KEY}" ] || die "no backup objects found in S3"
    fi

    BACKUP_FILE="${RESTORE_TEMP_DIR}/$(basename "${BACKUP_S3_KEY}")"
    log "Downloading s3://${BACKUP_S3_BUCKET}/${BACKUP_S3_KEY}"
    aws s3 cp "s3://${BACKUP_S3_BUCKET}/${BACKUP_S3_KEY}" "${BACKUP_FILE}" \
        --region "${BACKUP_S3_REGION}"
fi

[ -f "${BACKUP_FILE}" ] || die "backup file not found: ${BACKUP_FILE}" 1

# --- Verify the archive is readable before touching the target ---
log "Verifying archive integrity: ${BACKUP_FILE}"
pg_restore -l "${BACKUP_FILE}" >/dev/null 2>&1 || die "archive integrity check failed" 2

# --- Restore ---
restore_opts=(-d "${DATABASE_URL}" --no-owner --no-privileges -j "${RESTORE_JOBS}" --verbose)
if [ "${RESTORE_CLEAN}" = "true" ]; then
    log "Restoring with --clean --if-exists (destructive)"
    restore_opts+=(--clean --if-exists)
fi

log "Starting restore into ${DATABASE_URL%%\?*}"
set +e
pg_restore "${restore_opts[@]}" "${BACKUP_FILE}"
restore_rc=$?
set -e
# pg_restore returns 1 for non-fatal warnings; only >=2 is a hard failure.
if [ "${restore_rc}" -ge 2 ]; then
    die "pg_restore failed with exit code ${restore_rc}" 2
elif [ "${restore_rc}" -eq 1 ]; then
    log "pg_restore completed with warnings (exit 1); review output above"
fi

# --- Post-restore verification ---
if [ "${RESTORE_VERIFY}" = "true" ]; then
    log "Verifying restore..."
    if ! command -v psql >/dev/null 2>&1; then
        log "WARNING: psql not found; skipping verification"
    else
        tables="$(psql "${DATABASE_URL}" -tAc \
            "SELECT count(*) FROM information_schema.tables WHERE table_schema='public';")"
        log "Restored public tables: ${tables}"
        if [ "${tables}" -eq 0 ]; then
            die "verification failed: no tables found in public schema" 3
        fi
    fi
fi

log "Restore completed at $(date -u)"
