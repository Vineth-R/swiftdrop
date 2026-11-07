# Member 5 real-service journeys

These opt-in tests call real Identity, Pricing, Parcel and PostgreSQL through the
local gateway. They never intercept requests or return mocked replies. Run only
against a disposable local stack: they create accounts and retain parcels with
append-only history. They refuse non-loopback origins and require
`M5_LIVE_DISPOSABLE=1`.

Failures omit Playwright request call logs and credential headers, which can
contain local session cookies. Assertions still fail normally. The independent
redaction regression tests require no running services:

```sh
node --test tests/integration/parcelflow/safe-errors.test.mjs
```

Install locked dependencies, start/migrate the documented local stack with
`make up-build`, and start the web app if Compose does not provide it.

When testing with Mailpit, do not inherit real SMTP credentials from `.env`.
On Windows, assigning an empty process environment value removes the override,
so Compose can fall back to `.env`. The verification stack used an ignored
`tmp/m5/mailpit-override.yml` with literal empty credentials:

```yaml
services:
  identity:
    environment:
      SMTP_USER: ''
      SMTP_PASSWORD: ''
  notification-worker:
    environment:
      SMTP_USER: ''
      SMTP_PASSWORD: ''
```

Use both files for that isolated test stack, for example
`docker compose -f docker-compose.yml -f tmp/m5/mailpit-override.yml up -d --wait`.
Keep TLS protections enabled and leave the original `.env` unchanged.

```sh
M5_LIVE_DISPOSABLE=1 M5_LIVE_BASE_URL=http://localhost:8080 \
  node --test tests/integration/parcelflow/api-journey.mjs
M5_LIVE_DISPOSABLE=1 M5_LIVE_BASE_URL=http://localhost:3000 \
  node --test tests/integration/parcelflow/customer-journey.mjs
```

PowerShell:

```powershell
$env:M5_LIVE_DISPOSABLE = '1'
$env:M5_LIVE_BASE_URL = 'http://localhost:8080'
node --test tests/integration/parcelflow/api-journey.mjs
$env:M5_LIVE_BASE_URL = 'http://localhost:3000'
node --test tests/integration/parcelflow/customer-journey.mjs
```

The API journey verifies eight simultaneous identical requests produce one
parcel, stored fees, list/history/receipt, cross-customer 404s, authentication and
gateway rejection of internal routes. To change a real pricing rule through the
ADMIN API, also supply local `M5_LIVE_ADMIN_EMAIL` and `M5_LIVE_ADMIN_PASSWORD`.
It checks the changed estimate and `PRICE_CHANGED`, original replay/detail/PDF
fees, then restores the original rule in `finally`. Without those credentials it
explicitly reports pricing mutation as **NOT TESTED**.

The browser journey registers through the UI, obtains a real estimate, completes
the wizard, opens My Parcels and detail/timeline, downloads the PDF, checks the
360px layout/public tracking privacy, and verifies another customer's protected
reads return 404. Screenshots, PDFs and API outcomes go under ignored
`test-output/m5-live` (override `M5_LIVE_EVIDENCE`).

Install Playwright Chromium or set `PLAYWRIGHT_CHANNEL=chrome` for an installed
Chrome. A Windows host whose GPU/compositor stalls headless Chrome may set
`M5_BROWSER_DISABLE_GPU=1`; this affects only the opt-in test browser.

The existing `tests/access` suite still needs Compose for fixture SQL. These
scripts complement it; they do not prove a Docker run, M6 relay publication,
notification delivery, merchant order calling or courier lifecycle writes.
