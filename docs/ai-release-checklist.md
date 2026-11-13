# AI assistant release checklist

The SwiftDrop Delivery Assistant is part of the M1 release. It is not excluded
from `release/m1-complete` or from production image publication.

## Required configuration

| Variable                               | Required             | Notes                                                                                                            |
| -------------------------------------- | -------------------- | ---------------------------------------------------------------------------------------------------------------- |
| `AI_ENABLED`                           | yes                  | `true` in the release.                                                                                           |
| `AI_PROVIDER`                          | yes                  | `gemini`; `mock` is development-only.                                                                            |
| `GEMINI_API_KEY`                       | yes for real answers | Injected only into the `ai-assistant` container. Never in the web image, a build arg, `NEXT_PUBLIC_*` or source. |
| `AI_MODEL`                             | yes                  | Must be verified for the deployment key.                                                                         |
| `JWT_ACCESS_SECRET`                    | yes                  | Must match Identity and Parcel so private tools work.                                                            |
| `COOKIE_NAME`                          | yes                  | Must match Identity/Parcel (`sd_access` by default).                                                             |
| `PRICING_BASE_URL` / `PARCEL_BASE_URL` | yes                  | Internal service addresses and timeouts.                                                                         |
| `HTTP_TRUSTED_PROXIES`                 | yes                  | Limited to the gateway network.                                                                                  |
| `AI_RATE_LIMIT` / `AI_RATE_WINDOW`     | recommended          | Defaults to 15 per minute.                                                                                       |
| `AI_MAX_CONCURRENT`                    | recommended          | Process-local concurrency bound.                                                                                 |

## Behaviour and safety

- The assistant is reachable only at `POST /api/v1/ai/chat`; its health and
  readiness probes stay internal.
- Guests receive an HttpOnly guest session; they may use public tools only.
- CUSTOMER, DISPATCHER and ADMIN may use the private parcel tools. MERCHANT,
  COURIER and guests are denied.
- The model never receives a user ID, role or token as an authorization
  mechanism; the verified caller context decides access.
- The assistant never accesses the database and exposes no write tools.
- Tracking-number lookups use the ownership-scoped Parcel API, never the
  anonymous public projection.
- A missing key leaves the process running and returns `503 AI_UNAVAILABLE`
  instead of taking down the gateway.

## Release verification

1. `docker compose config` contains `ai-assistant` and the AI route.
2. `make build` builds `ai-assistant` for `linux/amd64`.
3. `docker compose up -d --wait` reports `ai-assistant` healthy.
4. `scripts/check-routes.sh` reaches `/api/v1/ai/chat` (empty message returns
   upstream 422) and confirms `/api/v1/ai/health` and `/ready` are 404 through
   the gateway.
5. The frontend build includes the chat widget.
6. With a real key, a signed-in CUSTOMER can ask about their own parcel and
   cannot retrieve another customer's information.
7. With no key, the chat returns a friendly `503 AI_UNAVAILABLE`.
8. `GEMINI_API_KEY` is absent from Gitleaks findings, images and logs.

## Known limitations

- Conversation storage, rate limiting and concurrency are process-local; a
  multi-replica deployment needs shared state.
- Gemini latency and provider availability affect answer quality.
- Live Gemini verification requires a billable key and is credential-gated.
