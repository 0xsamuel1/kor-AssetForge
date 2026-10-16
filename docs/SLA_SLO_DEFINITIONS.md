# AssetForge SLA / SLO Definitions & Error Budget Framework

## 1. Executive Summary

This document defines the Service Level Agreements (SLAs), Service Level Objectives (SLOs), and Service Level Indicators (SLIs) for the AssetForge tokenization and asset management platform. It also establishes the incident escalation framework, error budget calculation, monthly reporting cadence, and runbooks for addressing degradation events.

---

## 2. Core Service Level Objectives (SLOs)

### 2.1 Availability Objective
- **Target**: **99.9% Availability** across all critical customer-facing API endpoints over any rolling 30-day window.
- **Error Budget**: **0.1%** allowed failure rate (43.2 minutes of total downtime / 5xx error budget per month).
- **Calculation Formula**:
  $$\text{Availability SLI} = \frac{\sum \text{Successful HTTP Requests } (2xx, 3xx, 4xx)}{\sum \text{Total Valid HTTP Requests}} \times 100$$
  *(Note: 4xx client errors are excluded from server failure counts as they represent invalid user requests).*

### 2.2 Latency Objective
- **Target**: **p99 Latency < 500ms** for critical read and write endpoints under normal operating load (up to 1,000 RPS).
- **Target**: **p95 Latency < 200ms**.
- **Target**: **p50 Latency < 50ms**.
- **Calculation Formula**:
  $$\text{Latency SLI} = \frac{\sum \text{Requests served in } \le 500\text{ms}}{\sum \text{Total Valid HTTP Requests}} \ge 99.0\%$$

---

## 3. Critical Endpoint Classification

| Endpoint Tier | Routes / Endpoints | SLA Target | Latency Target (p99) | Degradation Policy |
|---|---|---|---|---|
| **Tier 1 (Critical)** | `/api/v1/assets`, `/api/v1/listings`, `/health`, `/metrics` | 99.9% | < 500ms | Failover to L1 cache, then PostgreSQL read replicas. Rate limiting priority. |
| **Tier 2 (Core Business)** | `/api/v1/transactions`, `/api/v1/auth/*`, `/api/v1/users/balance` | 99.5% | < 750ms | Atomic DB transactions with strict isolation. Singleflight retry. |
| **Tier 3 (Batch / Admin)** | `/api/v1/batch/*`, `/api/v1/compliance/*`, `/admin/*` | 99.0% | < 2,000ms | Async background worker queues with backpressure throttling. |

---

## 4. Error Budget & Burn Rate Alerting

AssetForge uses multi-window multi-burn-rate alerting recommended by Google SRE principles to detect anomalies before exhausting monthly budgets:

| Alert | Burn Rate Factor | Time Window | Budget Consumed | Action / Severity |
|---|---|---|---|---|
| **Fast Burn (Critical)** | 14.4x | 5m / 2m alert | 2% in 1 hour | Immediate PagerDuty page to On-Call Engineer. |
| **Moderate Burn** | 6x | 30m / 15m alert | 5% in 6 hours | Slack alert to `#platform-ops`. Triage within 30 minutes. |
| **Slow Burn (Warning)** | 1x | 6h / 3h alert | 10% in 3 days | Jira incident ticket created. Review in daily standup. |

---

## 5. Circuit Breaker & Fallback Thresholds

When downstream dependencies (such as Redis) encounter faults:
- **Consecutive Error Threshold**: `5` failed requests.
- **Trip State**: Circuit transitions to `OPEN`.
- **Degradation Path**: All read traffic falls back to PostgreSQL read-only query paths without throwing 500 errors to clients.
- **Probe Interval**: After `30 seconds` cool-off, the circuit enters `HALF_OPEN` and tests single downstream calls.
- **SLA Impact**: Degraded responses served within 500ms count as compliant under the availability SLO.

---

## 6. Monthly SLA Reporting & Compliance Review

### 6.1 Report Cadence
On the 1st business day of each calendar month, the platform reliability lead publishes the **AssetForge Monthly Reliability & SLA Report** containing:
1. Rolling 30-day uptime and availability percentage by endpoint tier.
2. p50, p95, p99 latency percentiles compared to SLO thresholds.
3. Total consumed error budget and root causes of any budget burns.
4. Summary of degraded cache incidents and circuit breaker activations.
5. Action items and preventive engineering tasks for the upcoming cycle.

### 6.2 Error Budget Policy Actions
- **If remaining error budget > 50%**: Feature releases and schema migrations proceed normally.
- **If remaining error budget < 20%**: Non-critical feature rollouts are paused; sprint capacity shifts 50% to reliability fixes.
- **If error budget exhausted (0%)**: Feature freezes enacted immediately until root causes are remediated and approved by Architecture Review.

---

## 7. Runbooks for SLA Violations

Refer to the incident runbooks located at:
- [SLA Violation Runbook](file:///home/kaycee/projects/makuo/kor-AssetForge/docs/runbooks/SLA_VIOLATION_RUNBOOK.md)
