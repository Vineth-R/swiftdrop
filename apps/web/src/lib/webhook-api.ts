import { api, apiWithMeta } from './api';

/**
 * The merchant's webhook calls and types. Field names follow API v1.1
 * sections 8.12 to 8.15 and contracts/openapi/merchant.yaml.
 */

/** The only event type API v1.1 defines for a webhook. */
export const WEBHOOK_EVENT = 'parcel.status_changed';

/** A delivery is tried at most this many times (API v1.1 section 10.3). */
export const MAX_ATTEMPTS = 6;

/** The longest address the API accepts. */
export const WEBHOOK_URL_MAX = 500;

export type WebhookSettings = {
  url: string;
  eventTypes: string[];
  isActive: boolean;
  updatedAt: string;
};

/** What a save returns. `signingSecret` appears only when it was just created or rotated. */
export type WebhookSettingsSaved = WebhookSettings & { signingSecret?: string };

export type WebhookSettingsInput = {
  url: string;
  eventTypes: string[];
  isActive: boolean;
  rotateSecret: boolean;
};

export type WebhookTestResult = {
  delivered: boolean;
  /** The merchant endpoint's HTTP status, or null when no response arrived. */
  httpStatus: number | null;
  durationMs: number;
};

export type WebhookDeliveryAttempt = {
  eventId: string;
  trackingNumber: string;
  attemptNumber: number;
  httpStatus: number | null;
  success: boolean;
  nextRetryAt: string | null;
  deadLettered: boolean;
  createdAt: string;
};

export type DeliveryQuery = {
  page: number;
  /** 'true' or 'false' to filter by outcome; '' for both. */
  success: '' | 'true' | 'false';
  /** Exact tracking-number match; '' for all. */
  trackingNumber: string;
};

/** 8.12. A merchant with no webhook gets a 404, which the screen treats as "not set up yet". */
export function getWebhook(signal?: AbortSignal) {
  return api<WebhookSettings>('/merchants/me/webhook', { signal });
}

/** 8.13. Creates or replaces the settings. */
export function saveWebhook(body: WebhookSettingsInput) {
  return api<WebhookSettingsSaved>('/merchants/me/webhook', {
    method: 'PUT',
    body,
  });
}

/** 8.14. Sends one signed sample event to the saved address. */
export function testWebhook() {
  return api<WebhookTestResult>('/merchants/me/webhook/test', {
    method: 'POST',
  });
}

/** 8.15. One page of delivery attempts, newest first. */
export function listWebhookDeliveries(
  query: DeliveryQuery,
  pageSize: number,
  signal?: AbortSignal,
) {
  const params = new URLSearchParams({
    page: String(query.page),
    pageSize: String(pageSize),
  });
  if (query.success) params.set('success', query.success);
  if (query.trackingNumber) params.set('trackingNumber', query.trackingNumber);
  return apiWithMeta<WebhookDeliveryAttempt[]>(
    `/merchants/me/webhook/deliveries?${params}`,
    { signal },
  );
}

/**
 * A quick check before the request, so the obvious mistakes are caught at the
 * field. The API stays the authority: it also rejects private and loopback
 * addresses, which only the server can resolve.
 */
export function validateWebhookUrl(value: string): string | undefined {
  const text = value.trim();
  if (!text) return 'Enter the address SwiftDrop should send events to.';
  if (text.length > WEBHOOK_URL_MAX) return 'Use 500 characters or fewer.';

  let url: URL;
  try {
    url = new URL(text);
  } catch {
    return 'Enter a full address, for example https://shop.example.com/hooks.';
  }
  if (url.protocol !== 'https:') return 'The address must start with https://.';
  if (url.username || url.password) {
    return 'Remove the user name and password from the address.';
  }
  return undefined;
}

export type AttemptOutcome = 'delivered' | 'retrying' | 'gave-up' | 'failed';

/** How one attempt ended, in the words the delivery log uses. */
export function attemptOutcome(
  attempt: WebhookDeliveryAttempt,
): AttemptOutcome {
  if (attempt.success) return 'delivered';
  if (attempt.nextRetryAt) return 'retrying';
  if (attempt.deadLettered) return 'gave-up';
  return 'failed';
}
