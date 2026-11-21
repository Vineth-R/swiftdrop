'use client';

import { ApiError, type ApiErrorCode } from '../../lib/api';
import { RETRY_AFTER_FALLBACK_SECONDS } from '../../lib/constants';
import { useCountdown } from '../../lib/use-countdown';
import { RateLimitedAlert, ServerFaultAlert } from '../auth/form-alerts';
import { Alert } from '../ui/alert';

/** Our wording for each code a parcel screen can meet. */
export const parcelMessages: Partial<Record<ApiErrorCode, string>> = {
  FORBIDDEN: 'This action is not available for your account.',
  NOT_FOUND:
    'This parcel is not available. Check the link or return to My parcels.',
  ROUTE_NOT_SERVICEABLE:
    'Delivery is not available between these zones. Choose another route.',
  ZONE_NOT_FOUND:
    'A selected zone is no longer available. Choose another zone.',
  WEIGHT_NOT_PRICED:
    'A delivery fee is not available for this weight. Change the weight and try again.',
  CONFLICT:
    'This request conflicts with an earlier delivery. Check My parcels before trying again.',
  VALIDATION_FAILED: 'Check the delivery information and try again.',
};

/**
 * A route or weight with no price is a normal answer about the delivery, not a
 * fault, so it is a warning (rule 10: say what happened, then what to do).
 */
const pricingCodes: ApiErrorCode[] = [
  'ROUTE_NOT_SERVICEABLE',
  'ZONE_NOT_FOUND',
  'WEIGHT_NOT_PRICED',
];

/** Seconds to wait after a 429, or null for any other error. */
export function rateLimitSeconds(error: unknown): number | null {
  return error instanceof ApiError && error.code === 'RATE_LIMITED'
    ? (error.retryAfter ?? RETRY_AFTER_FALLBACK_SECONDS)
    : null;
}

function RateLimited({ error }: { error: ApiError }) {
  const remaining = useCountdown(rateLimitSeconds(error));
  return <RateLimitedAlert remaining={remaining} />;
}

export function ParcelProblem({ error }: { error: unknown }) {
  if (error instanceof ApiError) {
    if (error.code === 'UNAUTHENTICATED') return null;
    if (error.code === 'RATE_LIMITED') return <RateLimited error={error} />;
    const message = parcelMessages[error.code];
    if (message)
      return (
        <Alert
          tone={pricingCodes.includes(error.code) ? 'warning' : 'danger'}
          title="Request could not be completed"
        >
          {message}
        </Alert>
      );
    return (
      <ServerFaultAlert
        isNetwork={error.code === 'NETWORK'}
        requestId={error.code === 'INTERNAL' ? error.requestId : undefined}
      />
    );
  }
  return <ServerFaultAlert isNetwork={false} />;
}
