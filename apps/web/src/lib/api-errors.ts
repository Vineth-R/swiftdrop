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
  | 'CANNOT_DEACTIVATE_SELF'
  | 'ZONE_NOT_FOUND'
  | 'ROUTE_NOT_SERVICEABLE'
  | 'WEIGHT_NOT_PRICED'
  | 'ZONE_CODE_EXISTS'
  | 'RULE_EXISTS'
  | 'BAND_OVERLAP'
  | 'TRACKING_NOT_FOUND'
  | 'CONFLICT'
  | 'PRICE_CHANGED'
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
  CANNOT_DEACTIVATE_SELF: 'CANNOT_DEACTIVATE_SELF',
  ZONE_NOT_FOUND: 'ZONE_NOT_FOUND',
  ROUTE_NOT_SERVICEABLE: 'ROUTE_NOT_SERVICEABLE',
  WEIGHT_NOT_PRICED: 'WEIGHT_NOT_PRICED',
  ZONE_CODE_EXISTS: 'ZONE_CODE_EXISTS',
  RULE_EXISTS: 'RULE_EXISTS',
  BAND_OVERLAP: 'BAND_OVERLAP',
  TRACKING_NOT_FOUND: 'TRACKING_NOT_FOUND',
  CONFLICT: 'CONFLICT',
  IDEMPOTENCY_CONFLICT: 'CONFLICT',
  PRICE_CHANGED: 'PRICE_CHANGED',
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
  /** Current authoritative LKR total, only for PRICE_CHANGED reconfirmation. */
  readonly currentFee?: number;

  constructor(init: {
    code: ApiErrorCode;
    status: number;
    message?: string;
    requestId?: string;
    retryAfter?: number;
    issues?: ApiIssue[];
    currentFee?: number;
  }) {
    super(init.message ?? init.code);
    this.name = 'ApiError';
    this.code = init.code;
    this.status = init.status;
    this.requestId = init.requestId;
    this.retryAfter = init.retryAfter;
    this.issues = init.issues ?? [];
    this.currentFee = init.currentFee;
  }

  /** A fault on our side or in the network: the only case that shows a Reference line. */
  get isServerFault(): boolean {
    return this.code === 'INTERNAL' || this.code === 'NETWORK';
  }
}

export type ErrorEnvelope = {
  error?: {
    code?: string;
    message?: string;
    details?: ApiIssue[];
    currentFee?: number;
  };
  meta?: { requestId?: string };
};

/** Turns a failed response into the one error type the screens understand. */
export async function readError(response: Response): Promise<ApiError> {
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
    currentFee:
      apiCode === 'PRICE_CHANGED' &&
      typeof body.error?.currentFee === 'number' &&
      Number.isFinite(body.error.currentFee) &&
      body.error.currentFee >= 0
        ? body.error.currentFee
        : undefined,
  });
}
