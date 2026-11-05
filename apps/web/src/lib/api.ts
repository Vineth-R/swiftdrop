/**
 * The one way the browser talks to the SwiftDrop API.
 *
 * Calls go to /api/v1 on the same origin, through the gateway (or the dev proxy
 * in next.config.js). The session lives in HttpOnly cookies that the browser
 * attaches itself, so no code here ever reads or stores a token.
 *
 * Every failure becomes one ApiError whose `code` picks the screen state and
 * the copy. The server's own message is kept for logs and never shown.
 *
 * CSRF posture: requests use same-origin credentials, SameSite=Lax cookies and
 * a JSON content type, which a cross-site form cannot send without a preflight.
 * That narrows the exposure but is not a complete defence. There is no CSRF
 * token until the API contract and the backend provide one.
 *
 * Transport, error shaping and session-expiry events live in http.ts,
 * api-errors.ts and session-events.ts; this module orchestrates them and keeps
 * the public surface the screens already import.
 */

import { ApiError, readError, type ApiIssue } from './api-errors';
import { send, type RequestOptions } from './http';
import { emitSessionExpired } from './session-events';

export { ApiError, type ApiErrorCode, type ApiIssue } from './api-errors';
export { onSessionExpired } from './session-events';

type ApiRequestOptions = RequestOptions & {
  /**
   * Renew the session once and retry when the access token has expired.
   * On by default; the auth endpoints themselves never do it.
   */
  renewSession?: boolean;
};

/** The `meta` of a success response. A list endpoint adds `pagination`. */
export type ApiMeta = {
  requestId?: string;
  timestamp?: string;
  pagination?: {
    page: number;
    pageSize: number;
    totalItems: number;
    totalPages: number;
  };
};

type Envelope<T> = { data: T; meta?: ApiMeta };
type ErrorEnvelope = {
  error?: { code?: string; message?: string; details?: ApiIssue[] };
  meta?: { requestId?: string };
};

let renewal: Promise<boolean> | null = null;

/** One renewal at a time, however many requests found the token expired. */
function renewSession(): Promise<boolean> {
  renewal ??= send('/auth/refresh', { method: 'POST' })
    .then((response) => response.ok)
    .catch(() => false)
    .finally(() => {
      renewal = null;
    });
  return renewal;
}

/**
 * A 401 means "sign in again", except when the API says INVALID_CREDENTIALS:
 * that is a wrong current password on the change-password form, and the person
 * is still signed in. Renewing the session for it would sign them out.
 */
async function isSessionEnded(response: Response): Promise<boolean> {
  if (response.status !== 401) return false;
  try {
    const body = (await response.clone().json()) as ErrorEnvelope;
    return body.error?.code !== 'INVALID_CREDENTIALS';
  } catch {
    return true;
  }
}

async function call<T>(
  path: string,
  options: ApiRequestOptions,
): Promise<Envelope<T>> {
  const mayRenew = options.renewSession ?? !path.startsWith('/auth/');

  let response = await send(path, options);
  if (mayRenew && (await isSessionEnded(response)) && (await renewSession())) {
    response = await send(path, options);
  }
  if (mayRenew && (await isSessionEnded(response))) {
    emitSessionExpired();
  }

  if (!response.ok) throw await readError(response);
  if (response.status === 204) return { data: undefined as T };

  try {
    return (await response.json()) as Envelope<T>;
  } catch {
    throw new ApiError({ code: 'INTERNAL', status: response.status });
  }
}

/** Calls the API and returns the `data` of the success envelope. */
export async function api<T = void>(
  path: string,
  options: ApiRequestOptions = {},
): Promise<T> {
  return (await call<T>(path, options)).data;
}

/**
 * Like api(), for list endpoints: it also returns the envelope's `meta`, which
 * carries `pagination` (page, pageSize, totalItems, totalPages).
 */
export async function apiWithMeta<T>(
  path: string,
  options: ApiRequestOptions = {},
): Promise<{ data: T; meta: ApiMeta }> {
  const { data, meta } = await call<T>(path, options);
  return { data, meta: meta ?? {} };
}
