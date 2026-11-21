# M4 backend verification evidence

Real command output from Nov 5, 2026, run on Windows 11 against commit `ebcdff4` plus the tests added on this branch.

| File | What it records | Result |
| --- | --- | --- |
| `01-up.txt` | Stack start, same commands as `make up` | All 12 containers healthy |
| `02-migrate.txt` | Migrations, same commands as `make migrate` | Passed |
| `03-check-routes.txt` | `scripts/check-routes.sh` | 24 of 24 passed |
| `04-go-test.txt` | `go test -count=1 ./...` | 45 packages passed, 2 could not build on Windows (below) |
| `05-go-test-db.txt` | The `make test-db` packages, without `-race` | 6 packages passed, 295 tests and subtests, 0 skipped |
| `06-pricing-coverage.txt` | Pricing coverage | 70.9% without a database, 71.8% with one |
| `07-gateway-calls.txt` | Estimate, zones and tracking through the gateway | All 6 returned the expected status |

## Not run locally

- **`-race` and `scripts/coverage.sh`.** The race detector needs a C compiler, which this machine doesn't have. CI runs both, including the 70% pricing gate.
- **`notification-worker` and `webhook-worker` tests in `04-go-test.txt`.** Their `main.go` uses `syscall.Kill`, which doesn't exist on Windows. Both images built on Linux in Docker.
- **`make` itself.** It isn't installed here, so the commands were copied from the Makefile.

## Test data used for the gateway calls

- Parcel `SDK7M2Q9XP4` was inserted by SQL with fake data, because `POST /parcels` isn't built yet.
- The GLE to JFN rule was deactivated for the unpriced route call, then restored.
- The gateway ran on port 8088, set by the local `.env`.
