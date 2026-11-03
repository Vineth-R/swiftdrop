/**
 * The one way the browser talks to the SwiftDrop API.
 *
 * Calls go to /api/v1 on the same origin, through the gateway (or the dev proxy
 * in next.config.js). The session lives in HttpOnly cookies that the browser
 * attaches itself, so no code here ever reads or stores a token.
 *
 * Every failure becomes one ApiError whose `code` picks the screen state and
 * the copy. The server's own message is kept for logs and never shown.
 */

export type ApiIssue = { field: string; issue: string };

/** What the screens switch on. The API's own names are mapped onto these. */
export type ApiErrorCode =
  | 'INVALID_CREDENTIALS'
  | 'ACCOUNT_LOCKED'
  | 'ACCOUNT_DEACTIVATED'
  | 'RATE_LIMITED'
  | 'EMAIL_IN_USE'
  | 'PASSWORD_NOT_SET'
  | 'RESET_TOKEN_INVALID'
  | 'VALIDATION_FAILED'
  | 'UNAUTHENTICATED'
  | 'FORBIDDEN'
  | 'NOT_FOUND'
  | 'NETWORK'
  | 'INTERNAL';

const codeFromApi: Record<string, ApiErrorCode> = {
  INVALID_CREDENTIALS: 'INVALID_CREDENTIALS',
  ACCOUNT_LOCKED: 'ACCOUNT_LOCKED',
  ACCOUNT_DISABLED: 'ACCOUNT_DEACTIVATED',
  RATE_LIMITED: 'RATE_LIMITED',
  EMAIL_ALREADY_EXISTS: 'EMAIL_IN_USE',
  PASSWORD_NOT_SET: 'PASSWORD_NOT_SET',
  INVALID_OR_EXPIRED_RESET_TOKEN: 'RESET_TOKEN_INVALID',
  VALIDATION_ERROR: 'VALIDATION_FAILED',
  UNAUTHENTICATED: 'UNAUTHENTICATED',
  TOKEN_EXPIRED: 'UNAUTHENTICATED',
  REFRESH_TOKEN_INVALID: 'UNAUTHENTICATED',
  FORBIDDEN: 'FORBIDDEN',
  NOT_FOUND: 'NOT_FOUND',
};

function codeFromStatus(status: number): ApiErrorCode {
  if (status === 401) return 'UNAUTHENTICATED';
  if (status === 403) return 'FORBIDDEN';
  if (status === 404) return 'NOT_FOUND';
  if (status === 422) return 'VALIDATION_FAILED';
  if (status === 429) return 'RATE_LIMITED';
  return 'INTERNAL';
}

export class ApiError extends Error {
  readonly code: ApiErrorCode;
  readonly status: number;
  /** The API's request ID. Show it as a "Reference" for server faults only. */
  readonly requestId?: string;
  /** Seconds to wait, from the Retry-After header of a 429. */
  readonly retryAfter?: number;
  /** Field problems from a 422, already keyed by the API's field names. */
  readonly issues: ApiIssue[];

  constructor(init: {
    code: ApiErrorCode;
    status: number;
    message?: string;
    requestId?: string;
    retryAfter?: number;
    issues?: ApiIssue[];
  }) {
    super(init.message ?? init.code);
    this.name = 'ApiError';
    this.code = init.code;
    this.status = init.status;
    this.requestId = init.requestId;
    this.retryAfter = init.retryAfter;
    this.issues = init.issues ?? [];
  }

  /** A fault on our side or in the network: the only case that shows a Reference line. */
  get isServerFault(): boolean {
    return this.code === 'INTERNAL' || this.code === 'NETWORK';
  }
}

type RequestOptions = {
  method?: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE';
  body?: unknown;
  signal?: AbortSignal;
  /**
   * Renew the session once and retry when the access token has expired.
   * On by default; the auth endpoints themselves never do it.
   */
  renewSession?: boolean;
};

const BASE = '/api/v1';

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

async function send(path: string, options: RequestOptions): Promise<Response> {
  try {
    return await fetch(`${BASE}${path}`, {
      method: options.method ?? 'GET',
      headers: {
        Accept: 'application/json',
        ...(options.body === undefined
          ? {}
          : { 'Content-Type': 'application/json' }),
      },
      body:
        options.body === undefined ? undefined : JSON.stringify(options.body),
      credentials: 'same-origin',
      cache: 'no-store',
      signal: options.signal,
    });
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError')
      throw error;
    throw new ApiError({ code: 'NETWORK', status: 0 });
  }
}

async function readError(response: Response): Promise<ApiError> {
  let body: ErrorEnvelope = {};
  try {
    body = (await response.json()) as ErrorEnvelope;
  } catch {
    // An HTML error page from the gateway, or an empty body.
  }
  const apiCode = body.error?.code;
  const retryHeader = Number(response.headers.get('Retry-After'));
  return new ApiError({
    code:
      (apiCode && codeFromApi[apiCode]) ||
      (response.status >= 500 ? 'INTERNAL' : codeFromStatus(response.status)),
    status: response.status,
    message: body.error?.message,
    requestId:
      body.meta?.requestId ?? response.headers.get('X-Request-Id') ?? undefined,
    retryAfter:
      Number.isFinite(retryHeader) && retryHeader > 0 ? retryHeader : undefined,
    issues: Array.isArray(body.error?.details) ? body.error.details : [],
  });
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

const sessionListeners = new Set<() => void>();

/** Called when a signed-in request could not be saved by renewing the session. */
export function onSessionExpired(listener: () => void): () => void {
  sessionListeners.add(listener);
  return () => sessionListeners.delete(listener);
}

async function call<T>(
  path: string,
  options: RequestOptions,
): Promise<Envelope<T>> {
  const mayRenew = options.renewSession ?? !path.startsWith('/auth/');

  let response = await send(path, options);
  if (mayRenew && (await isSessionEnded(response)) && (await renewSession())) {
    response = await send(path, options);
  }
  if (mayRenew && (await isSessionEnded(response))) {
    sessionListeners.forEach((listener) => listener());
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
  options: RequestOptions = {},
): Promise<T> {
  return (await call<T>(path, options)).data;
}

/**
 * Like api(), for list endpoints: it also returns the envelope's `meta`, which
 * carries `pagination` (page, pageSize, totalItems, totalPages).
 */
export async function apiWithMeta<T>(
  path: string,
  options: RequestOptions = {},
): Promise<{ data: T; meta: ApiMeta }> {
  const { data, meta } = await call<T>(path, options);
  return { data, meta: meta ?? {} };
}
