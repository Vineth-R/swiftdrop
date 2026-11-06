'use client';

import { Lock } from 'lucide-react';

import type { ApiError } from '../../lib/api';
import { RETRY_AFTER_FALLBACK_SECONDS } from '../../lib/constants';
import { useCountdown } from '../../lib/use-countdown';
import { RateLimitedAlert } from '../auth/form-alerts';
import { Alert } from '../ui/alert';
import { Button } from '../ui/button';
import { ReferenceLine } from '../ui/reference-line';

function RateLimited({ error }: { error: ApiError }) {
  const remaining = useCountdown(
    error.retryAfter ?? RETRY_AFTER_FALLBACK_SECONDS,
  );
  return <RateLimitedAlert remaining={remaining} />;
}

/** Error alert for dispatch screens and modals with user-friendly messages. */
export function DispatchAlert({
  error,
  onRetry,
}: {
  error: ApiError;
  onRetry?: () => void;
}) {
  const retry = onRetry && (
    <Button variant="secondary" size="md" onClick={onRetry}>
      Try again
    </Button>
  );

  switch (error.code) {
    case 'FORBIDDEN':
      return (
        <Alert
          tone="warning"
          icon={Lock}
          title="Access restricted"
          actions={retry}
        >
          Only dispatchers and administrators can perform this dispatch
          action.
        </Alert>
      );

    case 'RATE_LIMITED':
      return <RateLimited error={error} />;

    case 'PARCEL_CLOSED':
      return (
        <Alert
          tone="warning"
          title="Parcel already closed"
          actions={retry}
        >
          This parcel is already delivered or closed and cannot be assigned or reassigned.
        </Alert>
      );

    case 'COURIER_NOT_ELIGIBLE':
      return (
        <Alert
          tone="warning"
          title="Courier not eligible"
          actions={retry}
        >
          This courier is currently unavailable or ineligible for assignment. Please select another courier.
        </Alert>
      );

    case 'NOT_FOUND':
      return (
        <Alert
          tone="warning"
          title="Parcel not found"
          actions={retry}
        >
          This parcel no longer exists. Refresh the list to view current records.
        </Alert>
      );

    case 'VALIDATION_FAILED':
      return (
        <Alert
          tone="warning"
          title="Validation failed"
          actions={retry}
        >
          Please review the entered information and try again.
        </Alert>
      );

    default:
      return (
        <Alert
          tone="danger"
          title={
            error.code === 'NETWORK'
              ? 'We could not reach SwiftDrop'
              : 'Something broke on our side'
          }
          actions={retry}
        >
          {error.code === 'NETWORK'
            ? 'Check your connection and try again.'
            : 'Please try again in a moment.'}
          {error.isServerFault && (
            <ReferenceLine requestId={error.requestId} className="mt-2 block" />
          )}
        </Alert>
      );
  }
}
