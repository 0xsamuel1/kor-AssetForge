# Load Balancer Configuration

This guide describes how to place a load balancer (LB) in front of the
stateless kor-AssetForge API so traffic is distributed across instances,
unhealthy instances are removed, and TLS is terminated at the edge.

- **Related:** [`SCALING_GUIDE.md`](./SCALING_GUIDE.md), [`ARCHITECTURE_DIAGRAM.md`](./ARCHITECTURE_DIAGRAM.md), [`../kubernetes/deployment.yaml`](../kubernetes/deployment.yaml)

---

## 1. Requirements

| Requirement | Value / Behaviour |
|-------------|-------------------|
| Health endpoint | `GET /health` → `200` when the process is alive |
| Readiness endpoint | `GET /ready` → `200` only when DB/Redis dependencies are reachable |
| Session affinity | **Not required** — the API is stateless; disable sticky sessions |
| Balancing algorithm | `least_conn` (NGINX) or round-robin |
| TLS | Terminate at the LB; forward `X-Forwarded-For`/`-Proto` |
| Timeouts | `proxy_read_timeout` ≥ p99 latency (e.g. 60s for long polls) |
| Keep-alive | Enabled between LB and upstream |

> Do **not** enable sticky sessions. Session state is carried in signed JWTs,
> so any instance can serve any request.

---

## 2. NGINX Configuration

```nginx
upstream assetforge_api {
    least_conn;
    keepalive 64;

    server api-1.internal:8080 max_fails=3 fail_timeout=10s;
    server api-2.internal:8080 max_fails=3 fail_timeout=10s;
    server api-3.internal:8080 max_fails=3 fail_timeout=10s;
}

server {
    listen 443 ssl http2;
    server_name api.assetforge.example;

    ssl_certificate     /etc/nginx/tls/tls.crt;
    ssl_certificate_key /etc/nginx/tls/tls.key;
    ssl_protocols       TLSv1.2 TLSv1.3;

    # Security headers
    add_header Strict-Transport-Security "max-age=63072000; includeSubDomains" always;

    # WebSocket / real-time endpoints
    location /ws/ {
        proxy_pass         http://assetforge_api;
        proxy_http_version 1.1;
        proxy_set_header   Upgrade $http_upgrade;
        proxy_set_header   Connection "upgrade";
        proxy_set_header   Host $host;
        proxy_set_header   X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header   X-Forwarded-Proto $scheme;
        proxy_read_timeout 3600s;
    }

    location / {
        proxy_pass         http://assetforge_api;
        proxy_http_version 1.1;
        proxy_set_header   Connection "";
        proxy_set_header   Host $host;
        proxy_set_header   X-Real-IP $remote_addr;
        proxy_set_header   X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header   X-Forwarded-Proto $scheme;

        proxy_connect_timeout 5s;
        proxy_send_timeout    60s;
        proxy_read_timeout    60s;

        # Retry idempotent requests on another upstream
        proxy_next_upstream error timeout http_502 http_503 http_504;
        proxy_next_upstream_tries 3;
    }

    # Do not route health probes through rate limits / auth
    location = /health {
        proxy_pass http://assetforge_api;
        access_log off;
    }
}

# Redirect HTTP to HTTPS
server {
    listen 80;
    server_name api.assetforge.example;
    return 301 https://$host$request_uri;
}
```

Notes:

- `keepalive 64` plus `proxy_set_header Connection ""` enables upstream
  connection reuse, reducing handshake overhead under load.
- `least_conn` spreads work evenly when request cost varies.
- `proxy_next_upstream` retries safe requests when an instance fails.

---

## 3. Kubernetes Ingress / Service

In Kubernetes the `Service` provides L3/L4 load balancing and the `Ingress`
provides L7 routing and TLS. See [`../kubernetes/deployment.yaml`](../kubernetes/deployment.yaml).

```yaml
apiVersion: v1
kind: Service
metadata:
  name: assetforge-api
spec:
  selector:
    app: assetforge-api
  ports:
    - name: http
      port: 80
      targetPort: 8080
  # ClusterIP; Ingress/LB handles north-south traffic
```

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: assetforge-api
  annotations:
    nginx.ingress.kubernetes.io/proxy-read-timeout: "60"
    nginx.ingress.kubernetes.io/proxy-body-size: "10m"
    nginx.ingress.kubernetes.io/affinity: "none"
spec:
  tls:
    - hosts: ["api.assetforge.example"]
      secretName: assetforge-tls
  rules:
    - host: api.assetforge.example
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: assetforge-api
                port:
                  number: 80
```

The cloud provider's `LoadBalancer` service type or a managed ingress
controller (AWS ALB, GCP LB) provisions the external load balancer.

---

## 4. Cloud-Managed Load Balancer (AWS ALB)

| Setting | Value |
|---------|-------|
| Scheme | internet-facing |
| Listener | HTTPS `:443`, redirect HTTP → HTTPS |
| Target group | IP targets on container port `8080` |
| Health check | path `/health`, interval 15s, timeout 5s, healthy 2, unhealthy 3 |
| Algorithm | `least_outstanding_requests` |
| Stickiness | **disabled** |
| Deregistration delay | 30s (drain in-flight requests) |
| Cross-zone balancing | enabled |

Enable access logs and associate a WAF for edge protection. Use
`X-Forwarded-Proto` awareness in the API so generated URLs use `https`.

---

## 5. Health Checks, Draining & Failover

1. The LB polls `/ready`. When an instance becomes unready (e.g. DB down), the
   LB removes it from rotation.
2. On deploy, new instances register only after `/ready` returns `200`.
3. On shutdown, the instance flips `/ready` to failing, the LB drains it, then
   the process exits after the drain window (configure
   `terminationGracePeriodSeconds` accordingly).
4. Failed instances are retried on other upstreams automatically.

| Parameter | Recommended |
|-----------|-------------|
| Health interval | 10–15s |
| Healthy threshold | 2 |
| Unhealthy threshold | 3 |
| Deregistration/drain delay | 30s |
| Health timeout | 5s |

---

## 6. TLS & Security

- Use TLS 1.2+ only; prefer automated certificates (cert-manager / ACM).
- Set HSTS, and forward `X-Forwarded-For`/`-Proto` to the API.
- The API rate limiter uses the client IP; ensure it reads the forwarded IP
  (`X-Forwarded-For`) rather than the LB IP.
- Restrict `/metrics` to internal networks only.

---

## 7. Observability

Monitor the LB tier itself:

- Upstream 5xx rate, p95/p99 latency, active connections.
- Healthy/unhealthy target counts (alert if healthy < 2).
- Connection errors / retries to upstreams.
- TLS certificate expiry.

These complement the API-level metrics described in
[`OBSERVABILITY.md`](./OBSERVABILITY.md) and the SLOs in
[`SLA_SLO_DEFINITIONS.md`](./SLA_SLO_DEFINITIONS.md).
