import { ApiError, type ApiIssue } from './api-errors';
import { RETRY_AFTER_FALLBACK_SECONDS } from './constants';

/**
 * The common shapes an API failure can take on screen. Domain-specific codes
 * (invalid credentials, a locked account, an expired reset token) are handled
 * by the form that owns them before this classifier is called.
 */
export type ApiProblem =
  | { kind: 'rate-limited'; seconds: number }
  | { kind: 'validation'; issues: ApiIssue[] }
  | { kind: 'network' }
  | { kind: 'server'; requestId?: string };

/**
 * Maps a thrown value onto the presentation the screens share. Unknown values
 * and unknown codes become a server fault with no reference, matching the
 * previous per-form behaviour. Request IDs are shown only for our own faults.
 */
export function classifyApiError(error: unknown): ApiProblem {
  if (!(error instanceof ApiError)) return { kind: 'server' };

  switch (error.code) {
    case 'RATE_LIMITED':
      return {
        kind: 'rate-limited',
        seconds: error.retryAfter ?? RETRY_AFTER_FALLBACK_SECONDS,
      };
    case 'VALIDATION_FAILED':
      return { kind: 'validation', issues: error.issues };
    case 'NETWORK':
      return { kind: 'network' };
    case 'INTERNAL':
      return { kind: 'server', requestId: error.requestId };
    default:
      return { kind: 'server' };
  }
}
