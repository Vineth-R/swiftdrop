# Performance scenarios

These k6 scenarios are local, bounded smoke/load checks. They do not define
production SLOs.

## Prerequisites

- A running Compose stack (`make up`).
- `k6` installed (`brew install k6` on macOS).
- A real tracking number and two pricing zone IDs. The defaults match the local
  demo seed; override them with environment variables.

## Run

```sh
BASE_URL=http://localhost:8080 k6 run tests/perf/public.js
BASE_URL=http://localhost:8080 k6 run tests/perf/websocket.js
PERF_EMAIL=... PERF_PASSWORD=... k6 run tests/perf/auth.js
```

`auth.js` requires a valid seeded account and is intentionally not run by CI.
`public.js` and `websocket.js` use only public endpoints and are safe against a
local stack. Do not point these scripts at shared or production systems.

## Recorded baseline

The Phase 3 baseline is recorded in `docs/phase-3-performance-baseline.md`.
