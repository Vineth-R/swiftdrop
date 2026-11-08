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

PR #109 replaced the hardcoded fixture with an ephemeral key in the current
`feature/m10-realtime-handler` source. The historical commit that introduced
the value remains in that branch's ancestry, but the current source no longer
contains it.

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
was rewritten. PR #109 replaced the fixture in the current feature-branch
source. The historical commit `c3d22b3` remains in feature-branch ancestry and
is still detectable by a full-history scan; it is not reachable from
`development`. The future feature-to-development integration should use a
squash merge (or an approved history rewrite) so the original commit does not
enter protected history.

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

| File                                           | Change                                                                                                                                                                                                                                            |
| ---------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `.github/workflows/ci.yml`                     | Protected branch/tag Gitleaks step scopes to `github.sha` with `-m`; the PR scan now loads `.gitleaks.toml` from the trusted PR base SHA into a temporary file and fails closed if it is missing; the unreachable feature-branch step was removed |
| `.github/workflows/secret-scan-feature.yml`    | New lightweight workflow that scans the pushed range for feature-branch pushes, using the trusted `origin/development` configuration, with the same pinned Gitleaks version and checksum                                                          |
| `.github/CODEOWNERS`                           | New ownership rules for `.github/workflows/`, `.gitleaks.toml`, and CODEOWNERS itself; enforced only when code-owner review is required by branch rules                                                                                           |
| `tests/scripts/ci_secret_scan_test.go`         | Pins the protected-ref scope, the trusted PR/feature configuration strategy, and the feature workflow; adds dynamic tests proving unmerged refs are excluded and a PR/feature cannot suppress its own secret by editing `.gitleaks.toml`          |
| `docs/DEVELOPMENT_CI_FAILURE_INVESTIGATION.md` | This document                                                                                                                                                                                                                                     |

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
  PASS TestGitleaksPRConfigBypass
  PASS TestGitleaksFeatureConfigBypass
```

Trusted-configuration bypass result:

| Configuration       | Exit | Meaning                  |
| ------------------- | ---- | ------------------------ |
| PR-controlled       | `0`  | Bypass is possible       |
| Trusted base        | `1`  | Secret is still detected |
| Feature-controlled  | `0`  | Bypass is possible       |
| Trusted development | `1`  | Secret is still detected |

The dynamic tests cover:

- PR-range scanning with merge diffs using the trusted base configuration.
- Merge-only secret detection with and without `-m`.
- Protected-ref scope: unmerged refs excluded, protected-history secret still
  detected.
- A secret in an ancestor commit with a clean tip (full-history detection).
- Feature-range detection with the trusted development configuration.
- PR and feature configuration-bypass proofs: the head config allows a
  self-introduced secret, while the trusted base/development config detects it.
- The static test pins all scan invocations, trusted-config retrieval, the
  fail-closed SHA checks, and the absence of an allowlist for the realtime
  fixture.

Additional checks:

- `gofmt -l tests/scripts/ci_secret_scan_test.go` — clean.
- `go vet ./tests/scripts/...` — clean.
- `go test ./apps/services/realtime/...` — clean (the `ws` package is not
  present on `development`).
- Both workflow YAML files parsed successfully.
- The static policy test covers both `ci.yml` and the feature workflow,
  including the shared pinned Gitleaks version and checksum.
- `git diff --check` — clean.

### Trusted configuration strategy

- PR scans load `.gitleaks.toml` from `${PR_BASE_SHA}` into a temporary file
  and pass it with `--config`. The PR head cannot weaken its own scan.
- Feature-branch scans load `.gitleaks.toml` from `origin/development`. The
  feature branch cannot weaken its own scan.
- Protected branch/tag scans use `.gitleaks.toml` from the protected ref, which
  is part of the protected branch's trusted history.
- A missing trusted configuration fails the job closed; there is no fallback to
  the untrusted checkout.

### GitHub Actions verification

- PR #108 CI run `42481123424`: all required checks passed, including
  `Secret scanning (Gitleaks)`, `Security gate`, and `Required CI gate`.
- Feature-branch workflow run `42481119844` (`Secret scanning (feature
branches)`): passed.
- The hardening branch's CI results are recorded by its own workflow runs.
- The post-merge protected-branch run remains to be observed after review and
  merge.

## 11. Security implications

- Full-history secret detection for `development`, `main`, and tags remains
  enabled.
- Merge commits remain in scope via `-m`.
- Unmerged branches are no longer able to fail the protected-branch job.
- PR and feature scans use trusted configuration, so a PR or feature branch
  cannot suppress its own introduced secret by editing `.gitleaks.toml`.
- The historical finding is not suppressed: a feature-branch full-history scan
  still reports `c3d22b3`.
- No production credential is involved, so no production secret is exposed.
- PR #109 fixed the current feature-branch source; the historical commit
  remains until the feature branch is squash-merged or its history is rewritten
  with approval.

## 12. Credential rotation requirements

None for production. The value is a test-only signing fixture in a `_test.go`
file and is not used by a deployed service.

If the feature branch owner later suspects the value was copied from a real
environment, they should rotate that environment's JWT signing secret. There is
no evidence of that here.

## 13. Remaining manual actions

1. Use a **squash merge** for the eventual
   `feature/m10-realtime-handler` → `development` integration so `c3d22b3`
   does not become reachable from protected history. PR #109 already fixed the
   current feature-branch source.
2. Review and merge the hardening branch into PR #108, then re-run PR #108's
   checks. Do not merge PR #108 automatically.
3. After merge, confirm the protected-branch push run's
   `Secret scanning (Gitleaks)` job passes with the scoped scan.
4. Confirm the first push to a feature branch runs
   `Secret scanning (feature branches)` with the trusted development config.
5. Apply the branch rules in section 16. If the token lacks ruleset
   administration permission, this requires repository-owner action.

## 14. Known limitations

- The protected-branch post-merge path cannot be exercised by a PR. It was
  validated locally with the pinned Gitleaks version against the real repository
  history; the actual post-merge run will confirm it in CI.
- The historical feature-branch commit remains in the object database until
  that branch is squash-merged or its history is rewritten with approval. The
  current source is fixed.
- Gitleaks is process-local and scans the fetched repository; the fix depends on
  `github.sha` being the pushed protected ref, which is guaranteed by the push
  event and validated with `git cat-file`.
- The repository's `pull_request` trigger only covers `development` and `main`.
  A PR targeting a feature branch does not run the full CI workflow; rely on
  the push feature-branch scan and local validation for that stacked PR.

## 15. Trusted Gitleaks configuration

| Scan                   | Configuration source                                        |
| ---------------------- | ----------------------------------------------------------- |
| Pull request           | `${PR_BASE_SHA}:.gitleaks.toml`, written to a temp file     |
| Feature branch push    | `origin/development:.gitleaks.toml`, written to a temp file |
| Protected branch / tag | `.gitleaks.toml` from the pushed protected ref              |

The temporary files are removed by an EXIT trap. A missing configuration fails
the job with an explicit error; there is no fallback to the PR/branch checkout.
Changes to `.gitleaks.toml` in a PR or feature branch do not take effect for
that branch's own scan; they must land on the trusted base first.

## 16. Branch protection

The active **Development Branch Rules** ruleset was updated through the GitHub
API while preserving the existing deletion and non-fast-forward protections.
It now has:

- `required_approving_review_count: 1`
- `dismiss_stale_reviews_on_push: true`
- `require_code_owner_review: true`
- Required status checks: `Required CI gate` and `Security gate`
- `strict_required_status_checks_policy: true`
- Allowed merge methods remain `merge`, `squash`, and `rebase`

CODEOWNERS was added for `.github/workflows/`, `.gitleaks.toml`, and
`.github/CODEOWNERS` and names `@Vineth-R`. Code-owner review is only enforced
once the ruleset requires it and the CODEOWNERS file exists on the base branch;
the ruleset now does its part.

Effects to expect:

- PR #108 currently reports `BEHIND` because `development` has advanced and
  branches must be up to date. Update the PR branch with `development` before
  merging.
- At least one approving review is now required on PRs to `development`.
- A PR that changes a workflow or the Gitleaks configuration needs the
  code-owner review once CODEOWNERS is on the base branch.
- The required checks are the exact job contexts observed on recent runs:
  `Required CI gate` and `Security gate`.
