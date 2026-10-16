# Disaster Recovery Plan

This document defines how kor-AssetForge recovers from data loss and major
outages. It covers the automated backup system, restore procedures, restore
testing, and the recovery objectives the team operates against.

- **Owner:** Platform / SRE on-call
- **Last reviewed:** 2026-10
- **Related:** [`scripts/backup_db.sh`](../scripts/backup_db.sh), [`scripts/restore_backup.sh`](../scripts/restore_backup.sh), [`scripts/test_restore.sh`](../scripts/test_restore.sh), [`backend/config/backup_config.go`](../backend/config/backup_config.go)

---

## 1. Recovery Objectives

| Objective | Target | Notes |
|-----------|--------|-------|
| **RPO** (Recovery Point Objective) | **24 hours** | Daily full backup at 03:00 UTC. Enable WAL archiving (see §7) to reach 15-minute RPO. |
| **RTO** (Recovery Time Objective) | **2 hours** | Time from declared incident to a verified, serving database for a database-only restore. |
| **MTTR** (Mean Time To Restore) | **< 60 minutes** | Measured from the most recent restore rehearsal. |
| **Backup retention** | **30 days** local, **90 days** in S3 | Configurable via `BACKUP_RETENTION_DAYS` and S3 lifecycle rules. |
| **Restore verification** | **Monthly** | Automated by [`.github/workflows/backup_test.yml`](../.github/workflows/backup_test.yml). |
| **DR drill (full)** | **Quarterly** | Tabletop + live failover exercise. |

RPO and RTO are measured end-to-end: from the last successful backup timestamp
to the moment the API passes `/health` against the restored database.

---

## 2. Backup System Overview

Backups are produced by `scripts/backup_db.sh` and the in-process scheduler
configured by `backend/config/backup_config.go`.

| Property | Value |
|----------|-------|
| Format | PostgreSQL custom format (`pg_dump -Fc`), compressed and restorable table-by-table |
| Frequency | Daily at 03:00 UTC (`BACKUP_SCHEDULE_HOUR`) |
| Local path | `BACKUP_TEMP_DIR` (default `/tmp/kor-backups` / `backups/`) |
| Object storage | `s3://$BACKUP_S3_BUCKET/$BACKUP_S3_PREFIX/` with server-side encryption (`AES256`) |
| Integrity check | `pg_restore -l` runs immediately after every dump; corrupt dumps are deleted |
| Retention | Local files older than `BACKUP_RETENTION_DAYS` (30) are pruned |
| Credentials | S3 access via instance role or `AWS_*` env vars; never embedded in the dump |

### Backup failure handling

1. The backup job exits non-zero and logs the failure reason.
2. Alert on missed backup: if no `assetforge_*.dump` appears within 26 hours,
   page the on-call engineer (metric/alert owned by monitoring).
3. If a single day is missed, RPO degrades to 48 hours until the next success —
   escalate and take a manual backup immediately:

   ```bash
   DATABASE_URL="$DATABASE_URL" BACKUP_S3_BUCKET="$BUCKET" ./scripts/backup_db.sh
   ```

---

## 3. Restore Procedures

> Always restore into a **scratch** database first and verify, then promote.
> Never restore directly over the primary unless performing a declared DR cutover.

### 3.1 Prerequisites

- `pg_restore` and `psql` installed (PostgreSQL client matching the server major version).
- Network access to the target PostgreSQL instance.
- For S3 restores: `aws` CLI configured with read access to the backup bucket.
- The target database must be empty, or the restore must be run with `RESTORE_CLEAN=true`.

### 3.2 Restore from a local backup file

```bash
export DATABASE_URL="postgresql://user:pass@restore-host:5432/assetforge"
export BACKUP_FILE="/tmp/kor-backups/assetforge_20260101_030000.dump"
./scripts/restore_backup.sh
```

### 3.3 Restore the latest backup from S3

```bash
export DATABASE_URL="postgresql://user:pass@restore-host:5432/assetforge"
export BACKUP_S3_BUCKET="kor-assetforge-backups"
export BACKUP_S3_PREFIX="kor-assetforge/backups"
./scripts/restore_backup.sh
```

Restore a specific object:

```bash
export BACKUP_S3_KEY="kor-assetforge/backups/assetforge_20260101_030000.dump"
./scripts/restore_backup.sh
```

### 3.4 Restore over an existing database (destructive)

```bash
export RESTORE_CLEAN=true
./scripts/restore_backup.sh
```

`RESTORE_CLEAN=true` passes `--clean --if-exists`, dropping existing objects
before recreating them. Confirm you are pointing at the intended target.

### 3.5 Script options

| Variable | Default | Purpose |
|----------|---------|---------|
| `DATABASE_URL` | — | Target DSN (required) |
| `BACKUP_FILE` | — | Local dump to restore |
| `BACKUP_S3_BUCKET` / `BACKUP_S3_KEY` | — | S3 source (key optional; latest used) |
| `RESTORE_CLEAN` | `false` | Drop objects before restore |
| `RESTORE_JOBS` | `4` | Parallel restore workers |
| `RESTORE_VERIFY` | `true` | Run post-restore table-count verification |

Exit codes: `0` success, `1` usage/config error, `2` restore failure,
`3` verification failure.

---

## 4. Restore Verification Checklist

Run through this list after **every** restore, and record the result in the
incident ticket or monthly rehearsal log.

- [ ] `pg_restore -l <dump>` succeeds (archive is readable).
- [ ] `pg_restore` exits `0` or `1` (warnings only); anything `>=2` is a failure.
- [ ] Target database contains the expected `public` schema tables (see
      `backend/migrations/` for the authoritative list).
- [ ] Row counts for core tables (`users`, `assets`, `listings`,
      `transactions`) are within the expected range of the source.
- [ ] Foreign keys and indexes exist (`\d+ <table>`).
- [ ] The API starts against the restored database and passes `/health`.
- [ ] A smoke test login + read of a known asset succeeds.
- [ ] The restored timestamp is recorded, and actual RPO/RTO are compared
      against the targets in §1.
- [ ] The scratch database is dropped, or retained with a note explaining why.

### 4.1 One-liner verification

```bash
psql "$DATABASE_URL" -c "\dt" \
  && psql "$DATABASE_URL" -tAc "SELECT count(*) FROM users;" \
  && psql "$DATABASE_URL" -tAc "SELECT count(*) FROM assets;"
```

---

## 5. Automated Monthly Restore Test

`scripts/test_restore.sh` performs an end-to-end rehearsal:

1. `pg_dump` the source database.
2. Create a throwaway scratch database.
3. `pg_restore` the dump into the scratch database.
4. Compare `public` table counts between source and restored database.
5. Drop the scratch database.

Run it locally or manually:

```bash
DATABASE_URL="postgresql://user:pass@localhost:5432/assetforge" ./scripts/test_restore.sh
```

CI runs the same rehearsal on a monthly schedule (and on demand) via
[`.github/workflows/backup_test.yml`](../.github/workflows/backup_test.yml).
A failing workflow means the backups are **not** currently restorable — treat
it as a P1 and block reliance on backups until fixed.

---

## 6. Disaster Scenarios & Playbooks

### 6.1 Accidental data deletion / corruption

1. Declare the incident; freeze writes if possible (put the API in maintenance mode).
2. Identify the last good backup before the corruption.
3. Restore into a scratch DB (§3.2/§3.3) and verify (§4).
4. If a full revert is required, promote the scratch DB or run a clean restore (§3.4).
5. For partial recovery, restore into scratch and copy affected rows across.

### 6.2 Primary database instance lost

1. Provision a replacement PostgreSQL instance (see `docs/DOCKER_DEPLOYMENT.md` / infra runbooks).
2. Restore the latest backup (§3.3).
3. Update `DATABASE_URL` for the API (and read replicas if configured).
4. Verify (§4) and resume traffic.

### 6.3 S3 / object storage unavailable

Backups remain available locally for `BACKUP_RETENTION_DAYS`. Restore from the
local copy (§3.2) and re-upload once S3 recovers. Investigate the `aws s3 cp`
step in `scripts/backup_db.sh`, which logs failures without failing the dump.

### 6.4 Region-wide outage

Fail over to the standby region using the replicated S3 bucket. RTO depends on
infrastructure provisioning time; the data restore itself is bounded by §1.

---

## 7. Improving RPO (Optional)

The default RPO is 24 hours. To reduce it to ~15 minutes, enable WAL archiving
on the primary and stream to object storage:

```conf
# postgresql.conf
archive_mode = on
archive_command = 'aws s3 cp %p s3://kor-assetforge-backups/wal/%f'
wal_level = replica
```

This enables point-in-time recovery (PITR): restore the last base backup, then
replay WAL to the desired timestamp. PITR requires operational testing before
being relied upon.

---

## 8. Team Training & Responsibilities

| Role | Responsibility |
|------|----------------|
| On-call engineer | First responder; executes restore, runs the checklist. |
| Platform / SRE lead | Declares DR incidents, approves destructive restores. |
| Backend lead | Confirms data correctness and application compatibility. |
| Security | Reviews backup encryption, access control, and retention. |

Training requirements:

- Every on-call engineer completes a documented restore rehearsal before joining the rotation.
- New team members walk through this document and run `scripts/test_restore.sh` in a sandbox.
- Monthly automated rehearsal failures are reviewed in the reliability standup.
- Quarterly full DR drill: tabletop scenario + live restore into an isolated environment.

Record every drill in the runbook log with date, participants, actual RPO/RTO,
and follow-up actions.
