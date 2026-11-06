# Security scanning

This document describes the SEC-04 automated security controls. It reflects the
implementation in `.github/workflows/ci.yml` and `.github/workflows/codeql.yml`.
A control is not considered enabled unless the workflow configuration is present
and the corresponding job runs in CI.

## Scanners and thresholds

| Control                 | Tool                           | Version                   | Runs on                                                | Fails on                                 |
| ----------------------- | ------------------------------ | ------------------------- | ------------------------------------------------------ | ---------------------------------------- |
| Go vulnerabilities      | `govulncheck`                  | `v1.9.0`                  | PRs and pushes to `development`/`main`                 | Reachable (symbol-level) vulnerabilities |
| Production dependencies | `pnpm audit`                   | pnpm `12.15.0`, Node 24   | PRs and pushes to `development`/`main`                 | High or critical **production** findings |
| Secrets                 | Gitleaks                       | `v8.30.1`                 | PRs and pushes to `development`/`main`                 | Any configured secret rule match         |
| Container images        | Trivy                          | `v0.76.1`                 | Every image built by the changed-service Docker matrix | High or critical image findings          |
| SBOM                    | Syft via `anchore/sbom-action` | `v1.56.0`                 | Every image built by the changed-service Docker matrix | SBOM generation failure                  |
| SAST                    | CodeQL                         | `github/codeql-action@v4` | PRs and pushes to `development`/`main`                 | CodeQL analysis/upload failure           |

The existing `Required CI gate` remains the single required check. It now
depends on a `Security gate` job that aggregates the Go, frontend, secret,
SAST, and container security results. The security jobs do not use
`continue-on-error`, blanket severity suppression, or silent exclusions.

## Go vulnerability scan

CI installs the pinned scanner and runs it from the repository root:

```sh
go install golang.org/x/vuln/cmd/govulncheck@v1.9.0
"$(go env GOPATH)/bin/govulncheck" -scan=symbol ./...
```

`-scan=symbol` fails only for vulnerabilities reachable from the module's code.
Module-level advisories that are not called are reported in verbose output but
do not fail the job. Do not weaken this to a module-level check without a
separate policy decision.

Local run:

```sh
go install golang.org/x/vuln/cmd/govulncheck@v1.9.0
"$(go env GOPATH)/bin/govulncheck" -scan=symbol ./...
```

## Frontend production dependency audit

CI installs the frozen lockfile and audits the production dependency graph:

```sh
pnpm install --frozen-lockfile
pnpm audit --prod --audit-level=high
```

`--prod` deliberately excludes development-only tooling. Dev-only advisories
are not reported as production findings. Do not add blanket exclusions; if an
exception is unavoidable, document the advisory, package, reachability, and a
review/expiry condition.

Local run:

```sh
pnpm install --frozen-lockfile
pnpm audit --prod --audit-level=high
```

## Secret scanning

CI installs a checksum-verified Gitleaks binary and scans full Git history with
redaction:

```sh
gitleaks git --config .gitleaks.toml --redact --no-banner --no-color --verbose --exit-code 1 .
```

Rules come from the upstream default ruleset. `.gitleaks.toml` contains only
narrow, context-scoped allowlists for verified false positives (test
placeholders and third-party example documentation). Each allowlist requires
both a specific path and a specific match context, so a different credential in
the same file is still detected. Do not add broad directory or file-pattern
allowlists to obtain a green scan.

Local run:

```sh
# Install the pinned release and verify its checksum before use.
gitleaks git --config .gitleaks.toml --redact --no-banner --no-color --verbose --exit-code 1 .
```

Secret values are redacted. Do not upload unredacted Gitleaks reports.

## Container scanning and SBOMs

The existing changed-service Docker matrix builds an image tagged
`swiftdrop/<service>:<github.sha>`, verifies that the image exists, then:

1. generates a CycloneDX JSON SBOM with Syft `v1.56.0`,
2. uploads the SBOM as `sbom-<service>-<sha>`,
3. scans the image with Trivy `v0.76.1`, and
4. uploads the redacted/table report as `trivy-<service>-<sha>`.

`.github/trivy.yaml` sets the policy: HIGH and CRITICAL findings, including
unfixed findings, fail the job. `.github/trivyignore.yaml` is the exception
file and is empty. Per the policy in that file, an exception must be narrowly
scoped, have written owner acceptance, and include an `expired_at` date within
90 days.

Only images actually built by the matrix are scanned. A docs-only change does
not trigger Docker builds or image scans. Access-only changes may build the full
Compose stack for access tests but are not scanned by this matrix; a scheduled
full-matrix scan is a possible future improvement.

Local run:

```sh
docker build -f infra/docker/go.Dockerfile --build-arg SERVICE=identity -t swiftdrop/identity:local .
trivy image --config .github/trivy.yaml --severity HIGH,CRITICAL --exit-code 1 swiftdrop/identity:local

syft swiftdrop/identity:local -o cyclonedx-json > identity.cdx.json
```

For the web image use `infra/docker/web.Dockerfile` and
`swiftdrop/web:local`. The web runtime intentionally removes the unused npm
installation to avoid scanning npm's bundled dependencies and to reduce image
size.

## CodeQL

The `sast` matrix job in `.github/workflows/ci.yml` runs advanced CodeQL
analysis for Go and JavaScript/TypeScript. Go uses `autobuild`;
JavaScript/TypeScript uses `build-mode: none`. Results are categorized per
language so the two analyses do not overwrite each other. The job has
least-privilege permissions (`contents: read`, `pull-requests: read`, and
`security-events: write` only at the analysis job).

CodeQL's default setup must remain disabled; enabling both default and advanced
setup creates duplicate/conflicting analysis. No GitHub settings were changed by
this repository change.

## Dependency updates

Dependabot is not enabled in this repository. Go modules, npm workspace
dependencies, GitHub Actions, and container images are updated manually by the
development team. Automated update pull requests are not opened. When a
dependency is upgraded, make the change deliberately, update the corresponding
lockfile or image tag, and run the full `Required CI gate`.

The scanners above still detect vulnerable dependencies. Removing automated
update PRs changes who opens the upgrade, not whether vulnerabilities are
detected.

## Failed security gate

1. Identify the failing job and read its annotation.
2. For Go: identify the advisory and whether it is reachable. Prefer upgrading
   the module. Do not switch to a module-only scan to hide a reachable finding.
3. For pnpm: confirm the finding is in the production graph. Dev-only findings
   belong to a separate tooling update, not this gate.
4. For Gitleaks: confirm whether the finding is a real credential. Revoke and
   rotate any real secret immediately. Add a narrow allowlist only for a
   verified false positive.
5. For Trivy: identify the image, package, installed version, fixed version, and
   whether the vulnerable component is used at runtime. Prefer upgrading,
   removing the component, or updating the base image. Add an expiring exception
   only when no fix exists or the path is demonstrably unreachable.
6. Re-run the security job and then the full `Required CI gate`.

Never mark a scanner step `continue-on-error`, lower the severity threshold, or
allowlist a broad path to make a red security gate green.
