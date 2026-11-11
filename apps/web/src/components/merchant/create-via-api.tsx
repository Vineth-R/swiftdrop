'use client';

import { Check, Copy, ExternalLink, KeyRound } from 'lucide-react';
import { useState } from 'react';

import { Button, ButtonLink } from '../ui/button';
import { Section } from '../layout/section';

const API_DOCS_URL =
  'https://github.com/Vineth-R/swiftdrop/blob/development/docs/API_Documentation_v1.1.md#816-post-merchantorders';

const requestBody = `{
  "externalOrderId": "SZ-100245",
  "recipient": {
    "name": "Amali Fernando",
    "phone": "+94712345678",
    "address": "45 Lake Drive",
    "city": "Kandy",
    "zoneId": "9b2f..."
  },
  "weightKg": 1.5,
  "sizeCategory": "SMALL",
  "description": "Phone case"
}`;

const successBody = `{
  "data": {
    "orderId": "o7c6...",
    "externalOrderId": "SZ-100245",
    "trackingNumber": "SDN4T8W2LC6",
    "status": "CREATED",
    "fee": {
      "currency": "LKR",
      "baseFee": 450.00,
      "weightCharge": 100.00,
      "sizeSurcharge": 0.00,
      "total": 550.00
    },
    "createdAt": "2026-11-04T10:00:00Z"
  }
}`;

const curlExample = `curl -X POST https://your-swiftdrop-host/api/v1/merchant/orders \\
  -H "X-API-Key: $SWIFTDROP_API_KEY" \\
  -H "Idempotency-Key: order-100245" \\
  -H "Content-Type: application/json" \\
  -d '${requestBody.replaceAll('\n', '')}'`;

function CodeBlock({ label, code }: { label: string; code: string }) {
  return (
    <div className="flex min-w-0 flex-col gap-2">
      <p className="text-overline uppercase text-text-muted">{label}</p>
      <pre className="max-h-96 overflow-auto rounded-control border border-line bg-surface-2 p-4 font-mono text-caption leading-relaxed text-text">
        <code>{code}</code>
      </pre>
    </div>
  );
}

export function CreateViaAPI() {
  const [copied, setCopied] = useState(false);

  async function copyCurl() {
    try {
      await navigator.clipboard.writeText(curlExample);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      setCopied(false);
    }
  }

  return (
    <Section
      title="Create orders from your system"
      description="Merchant orders are created server-to-server with an API key. The key stays on your server and is never sent to the browser."
    >
      <div className="flex flex-col gap-6 rounded-card border border-line bg-surface p-5 md:p-6">
        <dl className="grid gap-4 sm:grid-cols-[auto_1fr]">
          <dt className="text-caption text-text-muted">Endpoint</dt>
          <dd className="break-all font-mono text-body-sm">
            <span className="mr-2 inline-flex rounded-chip border border-line bg-surface-2 px-2 py-1 text-overline">
              POST
            </span>
            /api/v1/merchant/orders
          </dd>
          <dt className="text-caption text-text-muted">Required header</dt>
          <dd className="break-all font-mono text-body-sm">
            X-API-Key: sdk_live_...
          </dd>
          <dt className="text-caption text-text-muted">Optional header</dt>
          <dd className="break-all font-mono text-body-sm">
            Idempotency-Key: up to 100 characters
          </dd>
          <dt className="text-caption text-text-muted">Content type</dt>
          <dd className="font-mono text-body-sm">application/json</dd>
        </dl>

        <div className="grid gap-6 xl:grid-cols-2">
          <CodeBlock label="Request body" code={requestBody} />
          <CodeBlock label="201 Created" code={successBody} />
        </div>

        <div className="flex flex-col gap-3">
          <p className="text-overline uppercase text-text-muted">Errors</p>
          <ul className="grid gap-2 text-body-sm text-text-muted sm:grid-cols-2">
            <li>
              <code className="font-mono text-text">401 INVALID_API_KEY</code> —
              missing, unknown or revoked key.
            </li>
            <li>
              <code className="font-mono text-text">
                403 MERCHANT_NOT_APPROVED
              </code>{' '}
              — the merchant is not approved.
            </li>
            <li>
              <code className="font-mono text-text">409 DUPLICATE_ORDER</code> —
              same reference or idempotency key, different payload.
            </li>
            <li>
              <code className="font-mono text-text">422 VALIDATION_ERROR</code>{' '}
              — invalid fields or an unpriced route.
            </li>
            <li>
              <code className="font-mono text-text">429 RATE_LIMITED</code> —
              more than 120 requests per minute for one key.
            </li>
            <li>
              <code className="font-mono text-text">
                503 SERVICE_UNAVAILABLE
              </code>{' '}
              — Parcel or Pricing is unavailable.
            </li>
          </ul>
        </div>

        <div className="flex flex-wrap items-center gap-3">
          <Button
            variant="secondary"
            onClick={copyCurl}
            iconLeading={
              copied ? (
                <Check size={16} aria-hidden="true" />
              ) : (
                <Copy size={16} aria-hidden="true" />
              )
            }
          >
            {copied ? 'Copied' : 'Copy cURL'}
          </Button>
          <ButtonLink href="/api-keys" variant="secondary">
            <KeyRound size={16} aria-hidden="true" />
            Manage API keys
          </ButtonLink>
          <a
            className="inline-flex min-h-11 items-center gap-2 text-body-sm font-semibold text-interactive underline underline-offset-4"
            href={API_DOCS_URL}
            target="_blank"
            rel="noreferrer"
          >
            API documentation
            <ExternalLink size={14} aria-hidden="true" />
          </a>
        </div>

        <p className="text-caption text-text-muted">
          Duplicate handling: the same external order reference or idempotency
          key with the same payload returns the original order. Reusing it with
          a different payload returns 409. Every order receives an SDN tracking
          number and an immutable fee snapshot.
        </p>
      </div>
    </Section>
  );
}
