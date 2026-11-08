# SwiftDrop AI Assistant — PR Preparation Report

Date: 2026-11-08
Prepared by: PR preparation audit
Status: Feature branch created and populated; no commit, push, merge or PR was performed (not authorized).

## 1. Original branch and starting state

- Original branch: `fix/booking-fee-post-merge-hardening`
- Original HEAD: `4831496 docs: add booking fee tests to the E2E inventories`
- Upstream: `origin/fix/booking-fee-post-merge-hardening` (0 ahead / 0 behind at start)
- The AI Assistant work was entirely uncommitted, on top of three booking-fee commits:

```
4831496 docs: add booking fee tests to the E2E inventories
0dae61d test(web): cover booking fee rate limits and error ownership
fcadcd8 fix(web): enforce booking fee rate limits and isolate estimate errors
```

The original branch and its working tree were preserved. No reset, clean, force-push, branch deletion or checkout of the original branch was performed.

## 2. Target base branch and commit

- Target: `origin/development`
- Base commit: `e3ca384c314210424b82eebfc1e37f85db3f23db`
- The feature branch was created directly from this commit.
- Verified: `git merge-base HEAD origin/development` equals `e3ca384c314210424b82eebfc1e37f85db3f23db`.

## 3. New feature branch

- Branch: `feat/swiftdrop-ai-assistant`
- Worktree: `/var/folders/s6/rkrnyxh51_ggggbvy5blmc2c0000gn/T/opencode/swiftdrop-ai-feature`
- Base: `origin/development` at `e3ca384`
- Commits on the branch: **0**. All AI changes are uncommitted in the new worktree, pending explicit approval to commit.
- The original branch/worktree remains available at `/Users/vinethranathunga/Documents/Repositories/swiftdrop`.

## 4. Files transferred

### New AI files

```text
apps/services/ai-assistant/                                  (49 files)
apps/web/src/components/ai-chat/                             (6 files)
apps/web/src/lib/ai-chat-api.ts
apps/web/src/lib/ai-chat-api.spec.ts
apps/web/src/lib/use-ai-chat.ts
apps/web-e2e/src/ai-chat.spec.ts
contracts/openapi/ai-assistant.yaml
docs/ai-assistant.md
docs/AI_ASSISTANT_FINAL_AUDIT.md
docs/AI_ASSISTANT_PR_PREPARATION.md                          (this report)
```

### Modified AI files

```text
.env.example
.github/workflows/ci.yml
AGENTS.md
Makefile
README.md
apps/web/src/app/layout.tsx
apps/web/src/lib/api-errors.ts
contracts/openapi/README.md
docker-compose.yml
docs/architecture.md
go.mod
go.sum
infra/nginx/default.conf
scripts/check-routes.sh
tests/nginxconfig/nginx_config_test.go
tests/scripts/check_routes_test.go
tests/scripts/ci_path_filter_test.go
```

No `.env`, `node_modules`, `.next`, `dist`, or `docs/audits/` content was copied. The worktree has no `.env`.

## 5. Shared files reconciled

### `README.md`

The AI patch applied cleanly to the `origin/development` version. It adds:

- The AI Assistant implementation paragraph.
- The `ai-assistant` server row (port 8087).
- The `apps/services/ai-assistant/` tree entry.
- The AI-related `.env.example` paragraph.
- The “SwiftDrop Delivery Assistant” section.
- The `ai-assistant` Nx project in the run-many example.
- The end-to-end assistant paragraph.
- Removal of “the AI assistant” from the not-implemented list.

No booking-fee documentation was added, removed or rewritten on the AI branch.

### `AGENTS.md`

The AI additions were manually reconciled onto the `origin/development` version, not copied from the booking-fee branch. Added:

- AI delivery assistant in the current-phase heading.
- AI implementation status and AI Assistant table row.
- AI chat widget in the Frontend and Frontend E2E rows.
- AI HTTP service, contracts, tests and repository-map entries.
- `ai-assistant` planned-responsibility bullet.
- `/api/v1/ai -> ai-assistant on 8087` route mapping.
- AI environment configuration paragraph.

The booking-fee wording (`booking wizard fee-estimate error and rate-limit states`) was intentionally **not** carried into this PR. The booking-fee work, including its documentation, remains intact on the original branch. The AI branch is based on `origin/development`, where the booking-fee commits are not present. If booking-fee merges first, this branch should be rebased and `AGENTS.md`/`README.md` rechecked.

## 6. Booking-fee changes excluded

The three booking-fee commits were not transferred. Their files remain at the `origin/development` versions in the AI worktree:

- `apps/web/src/components/parcels/new-delivery-view.tsx`
- `apps/web/src/components/parcels/parcel-problem.tsx`
- `apps/web-e2e/src/booking-fee.spec.ts`

The AI branch has **0 commits** ahead of `origin/development`; no booking-fee commit is present. The booking-fee code, tests and documentation remain recoverable on `fix/booking-fee-post-merge-hardening`.

## 7. Validation commands and results

Run in the AI worktree unless noted.

| Command                                                                                         | Result                                            |
| ----------------------------------------------------------------------------------------------- | ------------------------------------------------- |
| `git merge-base HEAD origin/development`                                                        | equals base `e3ca384`                             |
| `git log --oneline origin/development..HEAD`                                                    | 0 commits                                         |
| `git status --short`                                                                            | only intended AI modifications/untracked AI files |
| `git diff --check`                                                                              | PASS                                              |
| `go build ./...`                                                                                | PASS                                              |
| `go vet ./...`                                                                                  | PASS                                              |
| `go test -count=1 ./apps/services/ai-assistant/... ./tests/nginxconfig/... ./tests/scripts/...` | PASS                                              |
| `pnpm exec nx run-many -t build test lint -p ai-assistant --parallel=1`                         | PASS                                              |
| `pnpm exec nx test @org/web --skip-nx-cache`                                                    | PASS, 65 tests                                    |
| `pnpm exec nx lint @org/web --skip-nx-cache`                                                    | PASS                                              |
| `pnpm exec nx typecheck @org/web --skip-nx-cache`                                               | PASS                                              |
| `pnpm exec nx build @org/web --skip-nx-cache`                                                   | PASS                                              |
| `pnpm exec nx typecheck @org/web-e2e --skip-nx-cache`                                           | PASS                                              |
| `pnpm exec nx e2e @org/web-e2e --skip-nx-cache --args="--project=chromium src/ai-chat.spec.ts"` | PASS, 19/19                                       |
| `docker compose config --quiet`                                                                 | PASS                                              |
| `.env` present in worktree?                                                                     | No                                                |
| `docs/audits/` copied?                                                                          | No                                                |
| Booking-fee-only files modified?                                                                | No                                                |

Full CI was not executed. The above are the relevant local checks.

## 8. Remaining concerns

1. **No commit exists yet.** The branch is prepared but all changes are working-tree state. A commit requires explicit approval.
2. **Shared-document merge order.** `AGENTS.md` and `README.md` are also touched by the booking-fee commits. If booking-fee merges first, rebase this branch and recheck both files. If AI merges first, booking-fee must rebase.
3. **Full CI / access job not run.** The live Compose/Nginx route check and Gitleaks/Trivy/Syft were not executed in this preparation step.
4. **Model availability.** The local `.env` selects `gemini-3.8-flash`, which showed provider-side 504s during the audit. The code/example default is the verified `gemini-3.5-flash`; production should choose a stable model deliberately.
5. **`main` remains AI-free.** Do not merge this branch into `main`; a separate product decision and curated release are required.
6. **In-process MVP state.** Conversations and rate limits are not durable or multi-replica safe.

## 9. Recommended PR title

```text
feat(ai): add Gemini-backed SwiftDrop Delivery Assistant
```

## 10. Recommended PR description

```markdown
## Summary

Adds the SwiftDrop AI Delivery Assistant end to end:

- Gemini-backed Go service (`apps/services/ai-assistant`) with optional JWT
  identity, a bounded read-only tool registry, embedded knowledge retrieval,
  safe fallbacks and process-local conversations/rate limiting.
- Exactly four approved tools: `calculate_delivery_fee`,
  `get_public_parcel_status`, `get_my_parcel_history` and
  `get_my_active_deliveries`.
- Next.js floating chat widget, API client and Playwright coverage.
- `POST /api/v1/ai/chat` through Nginx, with an `ai-assistant` Compose service
  on internal port 8087.
- OpenAPI contract, implementation contract, architecture/README updates and
  environment-variable documentation.
- CI path filters, Docker image support, route checks and Nginx static tests.

## Security

- The Gemini key is injected only into the assistant container; it is not in the
  web image, `NEXT_PUBLIC_*`, build args or logs.
- Identity and roles come only from verified JWT claims. The model cannot supply
  user IDs, roles or ownership.
- Private tools forward only the caller's token and fail closed for anonymous
  callers.
- Public tracking and delivery results are allowlist-sanitized.
- Run `govulncheck` and the normal CI security jobs.

## Verification

- `go build ./...`, `go vet ./...`, `go test -count=1 ./...`
- `go test -race ./apps/services/ai-assistant/...`
- Nx `ai-assistant` build/test/lint
- Web unit tests, lint, typecheck, build
- AI Playwright 19/19
- `docker compose config --quiet`
- Live Gemini: basic chat, function calling, thought-signature round trip,
  pricing and public tracking (controlled upstreams); private tools covered by
  automated tests.

## Not included / follow-ups

- The three `fix/booking-fee-post-merge-hardening` commits are not part of this
  branch; `AGENTS.md`/`README.md` need reconciliation if that work merges first.
- `main` is AI-free; this PR targets `development` only.
- In-process conversations and rate limits are MVP limitations.
```

## 11. Exact next commands for the user

After reviewing the worktree and approving a commit:

```sh
cd /var/folders/s6/rkrnyxh51_ggggbvy5blmc2c0000gn/T/opencode/swiftdrop-ai-feature

# Final review
git status
git diff
git diff --check

# Confirm no .env or unrelated audit files are staged
git add .env.example .github/workflows/ci.yml AGENTS.md Makefile README.md \
  apps/services/ai-assistant apps/web/src/app/layout.tsx \
  apps/web/src/lib/api-errors.ts apps/web/src/components/ai-chat \
  apps/web/src/lib/ai-chat-api.ts apps/web/src/lib/ai-chat-api.spec.ts \
  apps/web/src/lib/use-ai-chat.ts apps/web-e2e/src/ai-chat.spec.ts \
  contracts/openapi/README.md contracts/openapi/ai-assistant.yaml \
  docker-compose.yml docs/architecture.md docs/ai-assistant.md \
  docs/AI_ASSISTANT_FINAL_AUDIT.md docs/AI_ASSISTANT_PR_PREPARATION.md \
  go.mod go.sum infra/nginx/default.conf scripts/check-routes.sh \
  tests/nginxconfig/nginx_config_test.go tests/scripts/check_routes_test.go \
  tests/scripts/ci_path_filter_test.go

git diff --cached --stat
git commit -m "feat(ai): add Gemini-backed SwiftDrop Delivery Assistant"
git push -u origin feat/swiftdrop-ai-assistant

gh pr create --base development --head feat/swiftdrop-ai-assistant \
  --title "feat(ai): add Gemini-backed SwiftDrop Delivery Assistant" \
  --body-file docs/AI_ASSISTANT_PR_PREPARATION.md
```

Do not push, merge or open the PR until the commit and PR description are approved. Do not merge this branch into `main`.
