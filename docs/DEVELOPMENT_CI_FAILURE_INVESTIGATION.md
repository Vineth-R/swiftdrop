# Development CI failure investigation

Date: 2026-11-08

## 1. Executive summary

The protected-branch CI run after merging PR #106
([run 42477050628](https://github.com/Vineth-R/swiftdrop/actions/runs/42477050628),
`f253339a7632cc6a48a8cd4cab93ab03e4142f76`) failed three checks:

- `Secret scanning (Gitleaks)`
- `Security gate`
- `Required CI gate`

Gitleaks reported one `generic-api-key` finding in
`apps/services/realtime/internal/ws/handler_test.go:25` at commit
`c3d22b3886c851ae83d062b22cf933b97819e25d`. That commit is reachable only from
the unmerged `origin/feature/m10-realtime-handler` branch. It is **not**
reachable from `development`, and the file does not exist on `development`.

The protected-branch Gitleaks step ran an unscoped `gitleaks git ... .` scan.
Because `actions/checkout` fetched every branch (`fetch-depth: 0`), that scan
covered unrelated refs, including the unmerged feature branch. The finding is a
test-only JWT signing fixture, not a production credential.

The Security gate and Required CI gate had no independent failure. They failed
closed because they correctly propagated the failed `secret-security` job.

The fix scopes the protected-branch/tag scan to the history reachable from the
pushed protected ref (`github.sha`) while retaining `-m` for merge commits. It
adds no allowlist, suppresses no rule, and rewrites no history.

## 2. Original CI failure details

| Field             | Value                                      |
| ----------------- | ------------------------------------------ |
| Workflow          | `CI`                                       |
| Run               | `42477050628`                              |
| Event             | `push` to `development`                    |
| Commit            | `f253339a7632cc6a48a8cd4cab93ab03e4142f76` |
| Successful checks | 23                                         |
| Failed checks     | 3                                          |
| Skipped checks    | 1 (`Release smoke test`)                   |

Failed jobs:

| Job                          | Cause                                                                       |
| ---------------------------- | --------------------------------------------------------------------------- |
| `Secret scanning (Gitleaks)` | One `generic-api-key` finding in full-history scan                          |
| `Security gate`              | `secret-security` result was `failure`; all other security inputs succeeded |
| `Required CI gate`           | `security-gate` result was `failure`; all other required inputs succeeded   |

All other checks passed, including Go checks, Go vulnerability scanning,
repository hygiene, SAST, integration tests, frontend checks, Playwright E2E,
and the Docker builds.

## 3. Exact Gitleaks finding

| Field           | Value                                                |
| --------------- | ---------------------------------------------------- |
| Rule ID         | `generic-api-key`                                    |
| File            | `apps/services/realtime/internal/ws/handler_test.go` |
| Line            | `25`                                                 |
| Commit          | `c3d22b3886c851ae83d062b22cf933b97819e25d`           |
| Author          | `WathsalaKJ`                                         |
| Commit date     | `2026-11-07T17:16:05Z`                               |
| Commits scanned | `289`                                                |
| Leaks found     | `1`                                                  |

The detected value is deliberately not reproduced anywhere in this repository
or this document.

## 4. Root cause and historical commit analysis

The finding is in a `const secret` block in a Go `_test.go` file. It is passed
only to `auth.NewTokenManager(secret, time.Minute)` in that test file to mint
short-lived test tokens. No production code references it; `_test.go` files are
not part of the shipped binary.

Historical checks:

```text
git branch -a --contains c3d22b3
  remotes/origin/feature/m10-realtime-handler

git merge-base --is-ancestor c3d22b3 origin/development
  exit 1 (not an ancestor)

git show origin/development:apps/services/realtime/internal/ws/handler_test.go
  fatal: path does not exist in origin/development
```

The CI defect was the scan scope, not the repository's protected history:

- `actions/checkout@v5` with `fetch-depth: 0` fetched all branches.
- The protected-branch step invoked `gitleaks git` without `--log-opts`.
- Gitleaks therefore scanned refs beyond the pushed protected branch, including
  `origin/feature/m10-realtime-handler`.

The scan was intended to be a full-history scan of the protected branch. An
unscoped scan is both broader than intended and coupled to unrelated,
unmerged work.

### Independent CI design issue: unreachable feature-branch scan

The `CI` workflow only triggers on `push` to `development`, `main`, and tags.
Its `Scan feature branch push commit range` step therefore never ran. The step
was moved to a dedicated `Secret scanning (feature branches)` workflow that
triggers on pushes to non-protected branches. This gives feature-branch pushes
the required range scan without running the entire CI pipeline on every branch.

## 5. Genuine credential or test-only fixture

The value is a test-only JWT signing fixture. It is used only by the realtime
WebSocket handler tests to issue tokens for the test server. It is not used by
an application service at runtime, is not present in configuration, and is not
a production credential.

Replacing it with an ephemeral key is still the better practice for that
feature branch. That change belongs to the realtime feature branch owner, not
to `development`, where the file does not exist.

## 6. How the historical occurrence was resolved

The protected-branch/tag scan now uses the pushed protected ref:

```text
PROTECTED_SHA=${{ github.sha }}
git cat-file -e "${PROTECTED_SHA}^{commit}"
gitleaks git ... --log-opts="${PROTECTED_SHA} -m" .
```

- `github.sha` is the tip of the pushed protected branch or tag.
- Scoping to that commit scans its full reachable history, including merged
  feature work and merge diffs (`-m`).
- Unmerged refs are no longer scanned by the protected-branch job.
- Missing or unresolvable SHAs fail closed before the scan.
- The PR scan is unchanged and still covers its range.
- The previously unreachable feature-branch scan now lives in
  `.github/workflows/secret-scan-feature.yml`, triggered by pushes to
  non-protected branches. It keeps the same fail-closed base/head resolution.

No Gitleaks allowlist was added, no rule was suppressed, and no shared history
was rewritten. The feature branch still contains the fixture and is still
detectable by a feature-branch scan. Its owner should replace it with an
ephemeral test key before that branch is merged.

## 7. Security gate root cause

The Security gate log shows:

| Input               | Result      |
| ------------------- | ----------- |
| `go-security`       | success     |
| `frontend-security` | success     |
| `secret-security`   | **failure** |
| `sast`              | success     |
| `docker`            | success     |

The gate failed on `secret-security` and on nothing else. Its `needs` list and
fail-closed logic are correct. No change is required.

## 8. Required CI gate root cause

The Required CI gate log shows every required input as `success` except:

| Input           | Result      |
| --------------- | ----------- |
| `security-gate` | **failure** |

Because the gate requires `security-gate` to succeed, it correctly failed. No
independent required check failed. No change is required.

## 9. Files modified

| File                                           | Change                                                                                                                                                                          |
| ---------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `.github/workflows/ci.yml`                     | Protected branch/tag Gitleaks step now scopes to `github.sha` with `-m` and fails closed on a missing/unresolvable SHA; the unreachable feature-branch step was removed         |
| `.github/workflows/secret-scan-feature.yml`    | New lightweight workflow that scans the pushed range for feature-branch pushes, with the same pinned Gitleaks version and checksum                                              |
| `tests/scripts/ci_secret_scan_test.go`         | Pins the protected-ref scope and the feature workflow, and adds a dynamic regression test proving unmerged refs are excluded while protected-history secrets are still detected |
| `docs/DEVELOPMENT_CI_FAILURE_INVESTIGATION.md` | This document                                                                                                                                                                   |

No application service, security scanner, threshold, or required gate was
modified.

## 10. Tests executed and results

Local Gitleaks was the pinned `8.30.1` binary.

Actual repository (values redacted; only rule/file/line/commit reported):

| Scan                    | Command scope                            | Exit                                      |
| ----------------------- | ---------------------------------------- | ----------------------------------------- |
| Unscoped (old behavior) | no `--log-opts`                          | `1` — reported the feature-branch fixture |
| Protected history       | `origin/development -m`                  | `0`                                       |
| Pushed protected SHA    | `f253339... -m`                          | `0`                                       |
| Unmerged feature branch | `origin/feature/m10-realtime-handler -m` | `1` — fixture still detectable            |

Go tests:

```text
go test -count=1 ./tests/scripts/...
  ok

GITLEAKS_BIN=<pinned gitleaks> go test -count=1 -v ./tests/scripts/ -run 'TestGitleaks'
  PASS TestGitleaksCurrentPRRange
  PASS TestGitleaksMergeOnlySecretDetected
  PASS TestGitleaksProtectedRefScope
```

The dynamic tests cover:

- PR-range scanning with merge diffs.
- Merge-only secret detection with and without `-m`.
- Protected-ref scope: unmerged refs excluded, protected-history secret still
  detected.
- The static test pins all three scan invocations, the fail-closed SHA checks,
  and the absence of an allowlist for the realtime fixture.

Additional checks:

- `gofmt -l tests/scripts/ci_secret_scan_test.go` — clean.
- `go vet ./tests/scripts/...` — clean.
- `go test ./apps/services/realtime/...` — clean (the `ws` package is not
  present on `development`).
- Both workflow YAML files parsed successfully.
- The static policy test covers both `ci.yml` and the feature workflow,
  including the shared pinned Gitleaks version and checksum.
- `git diff --check` — clean.

### GitHub Actions verification

- PR #108 CI run `42480610088`: all required checks passed, including
  `Secret scanning (Gitleaks)`, `Security gate`, and `Required CI gate`.
- Feature-branch workflow run `42480607642` (`Secret scanning (feature
branches)`): passed.
- The post-merge protected-branch run remains to be observed after review and
  merge.

## 11. Security implications

- Full-history secret detection for `development`, `main`, and tags remains
  enabled.
- Merge commits remain in scope via `-m`.
- Unmerged branches are no longer able to fail the protected-branch job.
- The finding is not suppressed: a feature-branch scan still reports it.
- No production credential is involved, so no production secret is exposed.
- The test fixture should still be replaced with an ephemeral key on its own
  branch.

## 12. Credential rotation requirements

None for production. The value is a test-only signing fixture in a `_test.go`
file and is not used by a deployed service.

If the feature branch owner later suspects the value was copied from a real
environment, they should rotate that environment's JWT signing secret. There is
no evidence of that here.

## 13. Remaining manual actions

1. Feature branch owner: replace the hardcoded test signing key in
   `apps/services/realtime/internal/ws/handler_test.go` with an ephemeral key
   before merging `feature/m10-realtime-handler`.
2. Review and merge this PR only after the PR checks pass.
3. After merge, confirm the protected-branch push run's
   `Secret scanning (Gitleaks)` job passes with the scoped scan.
4. Confirm the first push to a feature branch runs
   `Secret scanning (feature branches)` and passes.

## 14. Known limitations

- The protected-branch post-merge path cannot be exercised by a PR. It was
  validated locally with the pinned Gitleaks version against the real repository
  history; the actual post-merge run will confirm it in CI.
- The feature branch fixture remains in the object database until that branch
  is updated or deleted. It is not in protected history.
- Gitleaks is process-local and scans the fetched repository; the fix depends on
  `github.sha` being the pushed protected ref, which is guaranteed by the push
  event and validated with `git cat-file`.
