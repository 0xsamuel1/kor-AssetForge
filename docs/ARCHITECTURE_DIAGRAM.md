# Architecture Diagrams

Visual reference for the kor-AssetForge system, including the horizontally
scaled deployment topology. For the narrative overview see
[`architecture.md`](./architecture.md); for scaling specifics see
[`SCALING_GUIDE.md`](./SCALING_GUIDE.md) and [`LOAD_BALANCER_CONFIG.md`](./LOAD_BALANCER_CONFIG.md).

---

## 1. High-Level System Architecture

```
                         ┌─────────────────────────────┐
   Web / Mobile / SDK ──►│  CDN + WAF  (edge cache)     │
                         └───────────────┬─────────────┘
                                         │ HTTPS
                                         ▼
                         ┌─────────────────────────────┐
                         │  Load Balancer (TLS term.)  │
                         │  /health  /health/ready     │
                         └───────────────┬─────────────┘
                                         │ least_conn
             ┌───────────────────────────┼───────────────────────────┐
             ▼                           ▼                           ▼
      ┌─────────────┐             ┌─────────────┐             ┌─────────────┐
      │  API #1     │             │  API #2     │             │  API #N     │
      │ (stateless) │             │ (stateless) │   ......    │ (stateless) │
      └──────┬──────┘             └──────┬──────┘             └──────┬──────┘
             │                           │                           │
             └───────────┬───────────────┴───────────┬───────────────┘
                         ▼                           ▼
                 ┌───────────────┐           ┌───────────────┐
                 │     Redis     │           │ Elasticsearch │
                 │ L2 / limits / │           │    search     │
                 │  locks/jobs   │           └───────────────┘
                 └───────┬───────┘
                         │
                         ▼
             ┌───────────────────────────┐
             │   PostgreSQL (primary)    │
             │   writes + transactions   │
             └─────────────┬─────────────┘
                           │ streaming replication
        ┌──────────────────┼──────────────────┐
        ▼                  ▼                  ▼
 ┌────────────┐    ┌────────────┐     ┌────────────┐
 │ Read       │    │ Read       │     │ Read       │
 │ Replica #1 │    │ Replica #2 │ ... │ Replica #N │
 └────────────┘    └────────────┘     └────────────┘
                           │
                           ▼
                 ┌───────────────────┐     ┌───────────────────┐
                 │ Stellar / Soroban │     │ S3 (backups, obj) │
                 └───────────────────┘     └───────────────────┘
```

---

## 2. Scaling Architecture

```
   Client traffic grows
            │
            ▼
   ┌──────────────────┐    HPA scales replicas on CPU/memory
   │ HorizontalPod    │    min 2 ──► max 10
   │ Autoscaler       │
   └────────┬─────────┘
            │
            ▼
   ┌──────────────────────────────────────────────────────┐
   │  Stateless API Replica Set  (no sticky sessions)      │
   │  ┌─────┐ ┌─────┐ ┌─────┐ ┌─────┐ ┌─────┐              │
   │  │ API │ │ API │ │ API │ │ API │ │ API │  ...         │
   │  └──┬──┘ └──┬──┘ └──┬──┘ └──┬──┘ └──┬──┘              │
   └─────┼───────┼───────┼───────┼───────┼──────────────────┘
         │       │       │       │       │   each with its own
         │       │       │       │       │   bounded DB pool
         └───────┴───────┼───────┴───────┘
                         ▼
             ┌───────────────────────┐
             │  PgBouncer (optional) │  caps server connections
             │  transaction pooling  │
             └───────────┬───────────┘
                         │
              ┌──────────┴──────────┐
              ▼                     ▼
     ┌────────────────┐    ┌────────────────┐
     │ PG primary     │───►│ PG read replica│
     │ (reads+writes) │    │ (reads only)   │
     └────────────────┘    └────────────────┘
```

Total database connections scale as `replicas × DB_MAX_OPEN_CONNS`; size pools
so this stays below 80% of PostgreSQL `max_connections`, or front PostgreSQL
with PgBouncer. Read-heavy endpoints are routed to replicas to protect the
primary. See [`SCALING_GUIDE.md`](./SCALING_GUIDE.md) §3 and
[`DATABASE_REPLICATION.md`](./DATABASE_REPLICATION.md).

---

## 3. Request Lifecycle

```
Client ─► LB ─► API instance
                    │
                    ├─► L1 in-memory cache (hit? return)
                    │        │ miss
                    ├─► L2 Redis cache (hit? return, populate L1)
                    │        │ miss (coalesced via distributed lock)
                    ├─► PostgreSQL read replica (SELECTs)
                    └─► PostgreSQL primary (writes / transactions)
```

Reads prefer L1 → L2 → read replica. Writes always target the primary. Cache
regeneration across instances is serialized with a Redis lock to avoid stampedes.

---

## 4. Deployment Topology (Kubernetes)

```
                    ┌────────────────────────┐
                    │  Ingress / Cloud LB    │  TLS, WAF, access logs
                    └───────────┬────────────┘
                                ▼
                    ┌────────────────────────┐
                    │ Service: assetforge-api│  (ClusterIP, lb:http)
                    └───────────┬────────────┘
                                ▼
        ┌────────────────────────────────────────────┐
        │ Deployment: assetforge-api  (replicas N)    │
        │  • resources/limits   • liveness/readiness  │
        │  • preStop drain      • HPA 2..10           │
        │  • PodDisruptionBudget minAvailable: 2      │
        └────────────────────────────────────────────┘
                                │
                                ▼ (Stateful services, external or operators)
        PostgreSQL primary + replicas │ Redis │ Elasticsearch
```

Manifests live in [`../kubernetes/deployment.yaml`](../kubernetes/deployment.yaml).
