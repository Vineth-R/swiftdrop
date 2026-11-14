# Phase 3 Performance Baseline

Date: 2026-11-14
Baseline commit: `686a03dc25538f6353839fbb16dd83c0c56457e5` (before Phase 3 changes; the scenarios target unchanged public endpoints)
Tool: k6 v2.3.0 (darwin/arm64)

## Environment

| Item                    | Value                                                             |
| ----------------------- | ----------------------------------------------------------------- |
| Host                    | Apple silicon development host                                    |
| Docker                  | Docker Desktop, 8 CPUs / ~4 GiB assigned to the VM                |
| Stack                   | Full Docker Compose stack, all services healthy                   |
| Database                | Local PostgreSQL 16 with seeded demo users, merchants and parcels |
| Gateway                 | `http://localhost:8080`                                           |
| Public tracking fixture | `SDKUWNZTH2U`                                                     |
| Pricing fixture         | zones `CMB` and `KDY`, weight 2 kg, `SMALL`                       |

These are local smoke/load observations, not production SLOs. No arbitrary
threshold has been adopted.

## Scenarios

| Script                    | Workload                                                                  | Result                                     |
| ------------------------- | ------------------------------------------------------------------------- | ------------------------------------------ |
| `tests/perf/public.js`    | 1 VU, 20 s, health + public tracking + pricing estimate, 2.5 s think time | 8 iterations, 24 HTTP requests, 0 failures |
| `tests/perf/websocket.js` | 3 VUs, 15 s, anonymous `/ws` subscribe and hold                           | 9 connections, 18 checks, 100% pass        |
| `tests/perf/auth.js`      | 1 VU, 30 s, login with a temporary verified account, 6 s think time       | 5 logins, 0 failures                       |

### Public API latency

| Metric          |                                  Value |
| --------------- | -------------------------------------: |
| HTTP requests   |                                     24 |
| Failed requests |                                      0 |
| Average         |                                5.40 ms |
| p50             |                                4.79 ms |
| p95             |                               11.54 ms |
| p99             |                               12.28 ms |
| Maximum         |                               12.46 ms |
| Iteration rate  | ~0.40 iterations/s (deliberate pacing) |

The public tracking route is limited to 30 requests/minute per client address,
so the `public.js` scenario uses a 2.5 s think time. A high-VU run without pacing
returns expected `429` responses; that is the limiter working, not a server
failure.

### Authentication latency

| Metric        |     Value |
| ------------- | --------: |
| Logins        |         5 |
| Failed logins |         0 |
| Average       | 393.88 ms |
| p50           | 399.35 ms |
| p95           | 432.76 ms |
| p99           | 438.99 ms |
| Maximum       | 440.54 ms |

Login is dominated by the password-hash work factor, not database contention.
The login endpoint is rate limited to 10 requests/minute per client address, so
this scenario is a paced latency check rather than a throughput test.

### WebSocket

| Metric                     | Value |
| -------------------------- | ----: |
| Connections attempted      |     9 |
| Upgrade 101                |     9 |
| Subscription confirmations |     9 |
| Check failures             |     0 |

### Resource snapshot during the public scenario

One `docker stats --no-stream` sample:

| Container  |    CPU |    Memory |
| ---------- | -----: | --------: |
| `rabbitmq` | 80.43% | 132.2 MiB |
| `postgres` |  2.02% |  86.9 MiB |
| `nginx`    |  2.11% |  17.2 MiB |
| `pricing`  |  1.20% |  33.2 MiB |
| `parcel`   |  1.00% |  50.1 MiB |
| `web`      |  0.00% | 154.9 MiB |
| `identity` |  0.07% |  41.6 MiB |
| `realtime` |  0.24% |  32.1 MiB |
| Others     |  <0.1% | 20-42 MiB |

The RabbitMQ sample is a single instantaneous observation and is not a steady
state CPU baseline. It should be measured over time in a later load run.

## Reproduce

```sh
make up
VUS=1 DURATION=20s k6 run tests/perf/public.js
VUS=3 DURATION=15s k6 run tests/perf/websocket.js
PERF_EMAIL=<seeded-account> PERF_PASSWORD=<password> VUS=1 DURATION=30s \
  k6 run tests/perf/auth.js
```

## Limitations / missing measurements

- Merchant order creation, parcel creation throughput and courier lifecycle
  load were not run; they require controlled accounts and mutate data.
- Outbox drain rate, queue depth, consumer throughput and retry recovery were not
  measured as a sustained load. Broker stop/recovery and consumer readiness are
  covered by operational checks and integration tests.
- CPU and memory were sampled once, not continuously.
- No production target, SLA or capacity claim is made.
