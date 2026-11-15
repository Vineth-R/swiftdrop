# Phase 4 Write-Path Performance Baseline

Status: local measurement, not a production SLO.

## Scope

The Phase 4 k6 scenario exercises the customer write path end to end:

1. customer login and zone read;
2. real delivery-fee estimate;
3. parcel creation with an idempotency key;
4. dispatcher assignment;
5. courier `PICKED_UP` status update;
6. bounded polling until public tracking returns `PICKED_UP`.

The scenario is intentionally low-volume so it can run on a laptop and in a
disposable Compose stack without becoming a load test for shared infrastructure.

## Environment

| Item             | Value                                     |
| ---------------- | ----------------------------------------- |
| Date             | 2026-11-15                                |
| Host             | Local macOS Docker Desktop Compose stack  |
| Gateway          | `http://localhost:8080`                   |
| k6               | Local installation                        |
| Scenario         | `tests/perf/delivery-flow.js`             |
| VUs / duration   | 1 VU / 10 seconds                         |
| Iteration pacing | 1 second sleep after each successful flow |

## Command

```sh
BASE_URL=http://localhost:8080 \
PERF_CUSTOMER_EMAIL=... PERF_CUSTOMER_PASSWORD=... \
PERF_DISPATCHER_EMAIL=... PERF_DISPATCHER_PASSWORD=... \
PERF_COURIER_EMAIL=... PERF_COURIER_PASSWORD=... \
PERF_VUS=1 PERF_DURATION=10s \
k6 run tests/perf/delivery-flow.js
```

## Observed result

| Metric                     | Result       |
| -------------------------- | ------------ |
| Iterations                 | 10           |
| HTTP requests              | 55           |
| Failed HTTP requests       | 0 (0.00%)    |
| Checks                     | 55/55 (100%) |
| Check rate                 | 100.00%      |
| HTTP latency average       | 36.91 ms     |
| HTTP latency p95           | 232.36 ms    |
| Iteration duration p95     | 1.26 s       |
| Tracking propagation check | 100%         |

The iteration duration includes the deliberate 1-second pacing sleep and the
bounded poll for asynchronous status propagation. Do not compare it directly
with a raw request latency SLO.

## Limitations

- One developer laptop and one Compose replica; not a capacity result.
- Writes create real local parcels and are not cleaned up automatically.
- Background worker/outbox processing is measured only through the bounded
  tracking poll, not as a separate throughput curve.
- No production thresholds are claimed. `checks > 0.99` and
  `http_req_failed < 0.05` are smoke bounds for the local scenario.
