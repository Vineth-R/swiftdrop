'use client';

import { ApiError } from '../../lib/api';
import { ServerFaultAlert } from '../auth/form-alerts';
import { Alert } from '../ui/alert';

export function ParcelProblem({ error }: { error: unknown }) {
  if (error instanceof ApiError) {
    if (error.code === 'UNAUTHENTICATED') return null;
    const messages: Partial<Record<typeof error.code, string>> = {
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
      RATE_LIMITED:
        'Too many requests. Please wait a moment before trying again.',
      VALIDATION_FAILED: 'Check the delivery information and try again.',
    };
    const message = messages[error.code];
    if (message)
      return (
        <Alert tone="danger" title="Request could not be completed">
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
