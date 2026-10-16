# Docker Deployment Guide

This guide covers running kor-AssetForge with Docker Compose in production.
The development stack is unchanged (`docker compose up`); production uses a
dedicated compose file with resource limits, health checks, secret management,
and centralized logging.

- **Production compose:** [`docker-compose.prod.yml`](../docker-compose.prod.yml)
- **Development overrides:** [`docker-compose.override.yml`](../docker-compose.override.yml)
- **Environment template:** [`.env.prod.example`](../.env.prod.example)
- **Related:** [`DISASTER_RECOVERY.md`](./DISASTER_RECOVERY.md), [`SCALING_GUIDE.md`](./SCALING_GUIDE.md), [`OBSERVABILITY.md`](./OBSERVABILITY.md)

---

## 1. Development vs Production

| | Development | Production |
|---|---|---|
| Command | `docker compose up` | `docker compose --env-file .env.prod -f docker-compose.prod.yml up -d` |
| Compose files | `docker-compose.yml` + `docker-compose.override.yml` | `docker-compose.prod.yml` |
| Secrets | Plain env / defaults | Docker secrets (`./secrets/*.txt`) |
| Resource limits | None | CPU/memory limits + reservations |
| Logging | `json-file` (local) | `fluentd` → Elasticsearch, plus rotated `json-file` |
| Datastore ports | Published to host | Internal network only |
| Restart policy | `no` | `always` |

The override file is merged automatically by `docker compose` for local work. It
is **not** used in the production command above, so production behaviour is
explicit.

---

## 2. Prerequisites

- Docker Engine 24+ and Docker Compose v2 (`docker compose version`).
- At least 4 vCPU / 8 GB RAM for the full stack (Elasticsearch + Kibana are the
  heaviest components; see §7).
- A `.env.prod` file and a `secrets/` directory with strong random values.

---

## 3. Production Quick Start

```bash
# 1. Environment configuration
cp .env.prod.example .env.prod
$EDITOR .env.prod
chmod 600 .env.prod

# 2. Generate secrets (gitignored, mode 600)
chmod +x scripts/generate_secrets.sh
./scripts/generate_secrets.sh

# 3. Build and start
docker compose --env-file .env.prod -f docker-compose.prod.yml up -d --build

# 4. Check status
docker compose --env-file .env.prod -f docker-compose.prod.yml ps
curl -fsS http://localhost:8080/health
```

The API waits for PostgreSQL, Redis, Elasticsearch, and Fluentd to report
healthy before starting.

---

## 4. Secrets Management

Passwords and signing keys are supplied as **Docker secrets**, mounted read-only
at `/run/secrets/<name>`, and are never baked into images or exposed via
`docker inspect` environment values.

| Secret file | Used by | Purpose |
|-------------|---------|---------|
| `secrets/postgres_password.txt` | PostgreSQL, backend | Database password (`POSTGRES_PASSWORD_FILE`) |
| `secrets/redis_password.txt` | Redis, backend | Redis `requirepass` |
| `secrets/jwt_secret.txt` | backend | JWT signing key |
| `secrets/backup_encryption_key.txt` | backend | Backup encryption |

Generate them:

```bash
./scripts/generate_secrets.sh
```

The backend assembles `DATABASE_URL` and `REDIS_URL` at container start from the
secret files (see the `command` in `docker-compose.prod.yml`), so credentials do
not appear in the container's environment.

Rules:

- Never commit `secrets/` or `.env.prod` (both are gitignored).
- Restrict permissions: `chmod 700 secrets && chmod 600 secrets/*.txt`.
- Rotate secrets periodically and after any suspected exposure (§11).
- For orchestrators with native secret stores (Kubernetes Secrets, AWS Secrets
  Manager), mount them to the same `/run/secrets` paths.

---

## 5. Resource Limits

Every service defines CPU/memory limits and reservations under `deploy.resources`:

| Service | CPU limit | Memory limit | Memory reservation |
|---------|-----------|--------------|--------------------|
| postgres | 2.0 | 2 GB | 512 MB |
| redis | 1.0 | 768 MB | 128 MB |
| elasticsearch | 2.0 | 3 GB | 1 GB |
| fluentd | 1.0 | 512 MB | 128 MB |
| kibana | 1.0 | 1 GB | 512 MB |
| backend | 2.0 | 1 GB | 256 MB |

Elasticsearch heap is pinned with `ES_JAVA_OPTS=-Xms1g -Xmx1g` and memory
locking (`bootstrap.memory_lock=true`); keep the container memory limit above
the heap plus filesystem cache. Adjust limits to your host capacity — see
[`SCALING_GUIDE.md`](./SCALING_GUIDE.md) for capacity planning.

---

## 6. Health Checks

All services define health checks; dependent services wait for healthy state.

| Service | Check |
|---------|-------|
| postgres | `pg_isready` |
| redis | `redis-cli ping` → `PONG` |
| elasticsearch | `_cluster/health?wait_for_status=yellow` |
| fluentd | process alive |
| kibana | `/api/status` |
| backend | `GET /health` (liveness) |

The API exposes:

- `GET /health` — liveness (process is up)
- `GET /health/ready` — readiness (PostgreSQL, Redis, Stellar dependencies reachable)

Use `/health` for the container health check and `/health/ready` for load
balancer readiness (see [`LOAD_BALANCER_CONFIG.md`](./LOAD_BALANCER_CONFIG.md)).

Inspect health:

```bash
docker compose --env-file .env.prod -f docker-compose.prod.yml ps
docker inspect --format '{{.State.Health.Status}}' assetforge-prod-backend-1
```

---

## 7. Logging

Two layers:

1. **Container logs** — the `backend` service uses the Docker `fluentd` logging
   driver (`non-blocking`) and ships structured logs to the `fluentd` service,
   which forwards them to Elasticsearch (view in Kibana). Datastores use
   `json-file` with size/count rotation so disk usage stays bounded.
2. **Retention** — `json-file` rotation caps log size (`max-size`, `max-file`).

View logs:

```bash
docker compose --env-file .env.prod -f docker-compose.prod.yml logs -f backend
```

Fluentd only publishes `24224` on `127.0.0.1` because the Docker logging driver
talks to the host daemon. Elasticsearch and Kibana are internal-only.

> Elasticsearch runs with `xpack.security.enabled=false` on an isolated
> internal network. For regulated deployments, enable X.509/TLS and
> authentication and update `infrastructure/logging/fluentd.conf` accordingly.

---

## 8. Networking & Ports

| Network | Members | Exposure |
|---------|---------|----------|
| `edge` | backend | Host-published `API_PORT:8080` |
| `data` (internal) | postgres, redis, elasticsearch, fluentd, kibana, backend | No external route |

Only the API port and the loopback Fluentd forwarder are published. Put a load
balancer or reverse proxy in front of the API for TLS termination and horizontal
scaling — see [`LOAD_BALANCER_CONFIG.md`](./LOAD_BALANCER_CONFIG.md).

To expose Kibana for operators, add a port mapping on a trusted management
network rather than the public one.

---

## 9. Common Operations

```bash
# Start / stop / restart
docker compose --env-file .env.prod -f docker-compose.prod.yml up -d
docker compose --env-file .env.prod -f docker-compose.prod.yml down
docker compose --env-file .env.prod -f docker-compose.prod.yml restart backend

# Rebuild and rolling restart the API
docker compose --env-file .env.prod -f docker-compose.prod.yml up -d --build backend

# Run migrations (the API applies SQL migrations + AutoMigrate on startup)
docker compose --env-file .env.prod -f docker-compose.prod.yml run --rm backend /root/main

# Backup the database (see DISASTER_RECOVERY.md)
DATABASE_URL="postgresql://assetforge:<pw>@localhost:5432/assetforge" ./scripts/backup_db.sh

# Pull latest images and recreate
docker compose --env-file .env.prod -f docker-compose.prod.yml pull
docker compose --env-file .env.prod -f docker-compose.prod.yml up -d
```

### Scaling the API

A single host port cannot be published by multiple replicas. For horizontal
scaling, run the API behind a load balancer (or in Kubernetes) rather than
publishing `8080` directly — see [`SCALING_GUIDE.md`](./SCALING_GUIDE.md) and
[`../kubernetes/deployment.yaml`](../kubernetes/deployment.yaml).

---

## 10. Upgrades & Rollback

1. Set `IMAGE_TAG` in `.env.prod` to an immutable tag (avoid `latest` in prod).
2. `docker compose ... pull` (or `--build`) then `up -d`.
3. Verify `GET /health/ready` and run a smoke test.
4. Rollback by resetting `IMAGE_TAG` to the previous version and `up -d`.

Database migrations run automatically at startup and are additive; review
[`MIGRATIONS.md`](./MIGRATIONS.md) before major upgrades and take a backup first.

---

## 11. Secret Rotation

1. Update the value in the relevant `secrets/*.txt` (or rotate in your secret
   manager).
2. Recreate affected services so they re-read the file:
   - DB password: update PostgreSQL and the backend together to avoid downtime;
     `ALTER USER assetforge PASSWORD '...';` then restart `backend`.
   - Redis password: `CONFIG SET requirepass` plus restart `backend`, or restart
     `redis` and `backend` together.
   - JWT secret: rotating invalidates existing tokens; schedule during a window.
3. Confirm health and log in again.

---

## 12. Troubleshooting

| Symptom | Likely cause | Fix |
|---------|--------------|-----|
| `backend` restarts repeatedly | DB/Redis not healthy or wrong secret | `docker compose ... logs backend`, verify secrets and `POSTGRES_PASSWORD_FILE` |
| `permission denied` reading secrets | Wrong ownership/permissions on `secrets/` | `chmod 700 secrets && chmod 600 secrets/*.txt` |
| `elasticsearch` unhealthy / OOM | Heap or memory lock misconfigured | Increase container memory, verify `ES_JAVA_OPTS`, `ulimits.memlock` |
| No logs in Kibana | Fluentd driver cannot reach `localhost:24224` | Confirm `127.0.0.1:24224` is published and Fluentd is healthy |
| Port `8080` already in use | Another process bound | Set `API_PORT` in `.env.prod` |
| Health check fails but app works | Missing `wget`/network | Check `GET /health` manually; see container logs |

```bash
# Validate the composed configuration before deploying
docker compose --env-file .env.prod -f docker-compose.prod.yml config
```
