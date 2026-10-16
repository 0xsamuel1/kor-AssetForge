# Database Read Replication Guide

kor-AssetForge runs a single PostgreSQL primary for writes and can fan
read-only queries out to one or more PostgreSQL **read replicas**. This reduces
load on the primary and lets read-heavy endpoints scale independently.

- **Related:** [`SCALING_GUIDE.md`](./SCALING_GUIDE.md), [`DISASTER_RECOVERY.md`](./DISASTER_RECOVERY.md), [`../backend/database/router.go`](../backend/database/router.go), [`../backend/database/connection.go`](../backend/database/connection.go)

---

## 1. Concepts

| Term | Meaning |
|------|---------|
| **Primary** | The single writable PostgreSQL instance. Serves all writes and transactions. |
| **Replica** | A streaming-replication standby that serves read-only queries. |
| **Replication lag** | How far behind the replica is relative to the primary. |
| **Read/write splitting** | Routing `SELECT`s to replicas and mutations to the primary. |
| **Failover** | Sending reads to the primary when no replica is healthy. |

Writes always go to the primary. Reads prefer a healthy, non-lagging replica and
fall back to the primary. This is implemented in `backend/database`.

---

## 2. Configuration

All settings are environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_REPLICA_URLS` | _(empty)_ | Comma-separated replica DSNs. Empty disables replicas (single-DB mode). |
| `DB_REPLICA_LAG_THRESHOLD` | `30s` | Max acceptable replication lag before a replica is skipped. |
| `DB_REPLICA_HEALTH_INTERVAL` | `15s` | How often health/lag probes run. |
| `DB_REPLICA_FAILOVER_TO_PRIMARY` | `true` | Route reads to the primary when no replica is usable. |
| `DB_REPLICA_MAX_IDLE_TIME` | `5m` | Replica connection max idle time. |
| `DB_REPLICA_MAX_LIFETIME` | `30m` | Replica connection max lifetime. |

The primary DSN remains `DATABASE_URL`. Pool sizing (`DB_MAX_OPEN_CONNS`, etc.)
is shared with the primary via `config.LoadDBPoolConfig`.

Example:

```bash
export DATABASE_URL="postgresql://app:secret@primary.internal:5432/assetforge?sslmode=require"
export DATABASE_REPLICA_URLS="postgresql://app:secret@replica1.internal:5432/assetforge?sslmode=require,postgresql://app:secret@replica2.internal:5432/assetforge?sslmode=require"
export DB_REPLICA_LAG_THRESHOLD=30s
export DB_REPLICA_FAILOVER_TO_PRIMARY=true
```

When `DATABASE_REPLICA_URLS` is unset, nothing changes — the application keeps
using the primary for everything.

---

## 3. Setting Up Replicas

### 3.1 Managed PostgreSQL

Most managed providers (RDS, Aurora, Cloud SQL, Azure Database) can create a
read replica from the primary through their console or API. Point
`DATABASE_REPLICA_URLS` at the replica endpoints.

### 3.2 Self-managed streaming replication

On the primary (`postgresql.conf`):

```conf
wal_level = replica
max_wal_senders = 10
wal_keep_size = 1GB
```

Allow replication connections in `pg_hba.conf`:

```conf
host replication replicator 10.0.0.0/24 scram-sha-256
```

Take a base backup and start the standby:

```bash
pg_basebackup -h primary.internal -D /var/lib/postgresql/data \
  -U replicator -P -R -X stream

# standby.signal is created by -R; start PostgreSQL on the replica
```

Verify replication on the primary:

```sql
SELECT client_addr, state, sync_state, sent_lsn, replay_lsn
FROM pg_stat_replication;
```

Verify lag on the replica:

```sql
SELECT now() - pg_last_xact_replay_timestamp() AS replication_lag;
```

---

## 4. Read/Write Splitting in the Application

Two ways to route queries:

### 4.1 Automatic routing (recommended)

Install the GORM plugin once and use the primary handle normally. Reads are
routed to replicas; writes stay on the primary.

```go
primary, err := gorm.Open(postgres.Open(os.Getenv("DATABASE_URL")), &gorm.Config{})
if err != nil { /* handle */ }

cluster := database.Open(/* ... */)
router := database.NewRouter(cluster)

db, err := router.Auto() // installs database.ReadWritePlugin on primary
if err != nil { /* handle */ }

// Automatically routed to a replica:
var assets []models.Asset
db.Find(&assets)

// Automatically routed to the primary:
db.Create(&models.Asset{ /* ... */ })
```

Under the hood `database.ReadWritePlugin` hooks GORM's query and row callbacks
(sends them to a replica) and classifies raw statements (`SELECT`, `WITH ...
SELECT`, `EXPLAIN`, `SHOW` go to replicas; everything else to the primary).

### 4.2 Explicit routing

```go
router := database.NewRouter(cluster)

router.Read().Find(&assets)              // replica (or primary fallback)
router.Write().Create(&models.Listing{}) // primary
router.DB("SELECT ...").Scan(&rows)      // classified by SQL
```

### 4.3 Read-your-writes consistency

Replica lag means a read immediately after a write may not see it. Pin the
session to the primary when consistency matters:

```go
db = database.UsePrimary(db)
db.First(&asset, id) // guaranteed to read from the primary
```

Transactions always execute on the primary, so operations inside a
`db.Transaction(...)` are not split.

---

## 5. Replica Lag Monitoring

`database.Cluster` probes each replica on the health interval using:

```sql
SELECT COALESCE(EXTRACT(EPOCH FROM (now() - pg_last_xact_replay_timestamp())), 0);
```

Behaviour:

- Lag **≤** `DB_REPLICA_LAG_THRESHOLD` → replica is used for reads.
- Lag **>** threshold → replica is marked unhealthy and skipped.
- Probe failure / unreachable → replica marked unhealthy.

Start the monitor at boot:

```go
cluster.StartHealthChecks(ctx, cfg.HealthInterval)
```

Inspect state programmatically:

```go
cluster.HealthyReplicas()          // number of usable replicas
for _, node := range cluster.Replicas() {
    log.Printf("%s healthy=%v lag=%s", node.Name, node.Healthy(), node.Lag())
}
```

### Alerting

Export replica lag and health to Prometheus and alert on:

- No healthy replicas for > 1 minute.
- Replica lag approaching the threshold (e.g. > 80% of `DB_REPLICA_LAG_THRESHOLD`).
- Replica connection errors.

A reasonable query (once exported as `db_replica_lag_seconds`):

```promql
db_replica_lag_seconds > 24
```

---

## 6. Failover Behaviour

- If a replica is unhealthy or too far behind, `Cluster.Replica()` skips it and
  uses the next healthy replica.
- If **no** replica is usable and `DB_REPLICA_FAILOVER_TO_PRIMARY=true`, reads
  go to the primary. Availability is preserved at the cost of extra primary load.
- If `DB_REPLICA_FAILOVER_TO_PRIMARY=false`, reads still go to the primary but
  the misconfiguration is logged via health state; set this only if you would
  rather fail than serve reads from the primary.

> ⚠️ This failover only affects **read routing**. It does not promote a replica
> to primary. Promoting a replica after a primary failure is a separate
> operational procedure (see [`DISASTER_RECOVERY.md`](./DISASTER_RECOVERY.md)).

---

## 7. Testing

The read/write splitting logic is covered by unit tests that use two in-memory
SQLite databases to stand in for the primary and replica:

```bash
cd backend
go test ./database/... ./config/... -v
```

Tests cover:

- SQL classification (`IsReadQuery`).
- Explicit read (replica) and write (primary) paths.
- Automatic split via the GORM plugin.
- Failover to the primary when a replica is unhealthy.
- Marking a replica unhealthy when lag exceeds the threshold.
- Replica configuration parsing and defaults.

Run a manual replica check against a real environment:

```bash
psql "$DATABASE_URL" -c "SELECT client_addr, state, sync_state FROM pg_stat_replication;"
```

---

## 8. Operational Checklist

- [ ] Primary has `wal_level = replica` and `max_wal_senders` configured.
- [ ] Replica(s) streaming and lag within threshold.
- [ ] `DATABASE_REPLICA_URLS` set for the API deployment.
- [ ] Pool sizes sized for `replicas × DB_MAX_OPEN_CONNS` (see [`SCALING_GUIDE.md`](./SCALING_GUIDE.md)).
- [ ] Health monitor started and lag/health metrics exported.
- [ ] Alerts configured for unhealthy replicas and high lag.
- [ ] Read-after-write paths use `UsePrimary` where consistency is required.
- [ ] Failover procedure documented and rehearsed.
