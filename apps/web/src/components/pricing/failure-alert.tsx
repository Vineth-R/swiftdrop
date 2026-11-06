'use client';

import { Lock } from 'lucide-react';

import type { ApiError } from '../../lib/api';
import { RETRY_AFTER_FALLBACK_SECONDS } from '../../lib/constants';
import { useCountdown } from '../../lib/use-countdown';
import { serverFieldMessage } from '../../lib/validation';
import { RateLimitedAlert } from '../auth/form-alerts';
import { Alert } from '../ui/alert';
import { Button } from '../ui/button';
import { ReferenceLine } from '../ui/reference-line';

/**
 * Our wording for the fields a 422 names. Anything the form does not show comes
 * back empty, so the caller can fall back to a FailureAlert.
 */
export function fieldErrors<Field extends string>(
  error: ApiError,
  fields: readonly Field[],
): Partial<Record<Field, string>> {
  const found: Partial<Record<Field, string>> = {};
  for (const issue of error.issues) {
    const field = fields.find((name) => name === issue.field);
    const message = field && serverFieldMessage(field);
    if (field && message) found[field] = message;
  }
  return found;
}

function RateLimited({ error }: { error: ApiError }) {
  const remaining = useCountdown(
    error.retryAfter ?? RETRY_AFTER_FALLBACK_SECONDS,
  );
  return <RateLimitedAlert remaining={remaining} />;
}

/** Why a list or an action failed, in words that never repeat the server's. */
export function FailureAlert({
  error,
  onRetry,
}: {
  error: ApiError;
  onRetry?: () => void;
}) {
  const retry = onRetry && (
    <Button variant="secondary" size="lg" onClick={onRetry}>
      Try again
    </Button>
  );
  switch (error.code) {
    case 'FORBIDDEN':
      return (
        <Alert
          tone="warning"
          icon={Lock}
          title="You do not have access to this"
        >
          Only administrators can manage pricing. If you need access, ask an
          administrator.
        </Alert>
      );
    case 'RATE_LIMITED':
      return <RateLimited error={error} />;
    case 'NOT_FOUND':
      return (
        <Alert
          tone="warning"
          title="This item no longer exists"
          actions={retry}
        >
          Reload the list to see how it looks now.
        </Alert>
      );
    case 'ZONE_NOT_FOUND':
      return (
        <Alert tone="warning" title="One of these zones no longer exists">
          Reload the page and choose again.
        </Alert>
      );
    case 'RULE_EXISTS':
      return (
        <Alert tone="warning" title="This route already has an active price">
          Deactivate that rule first.
        </Alert>
      );
    case 'BAND_OVERLAP':
      return (
        <Alert tone="warning" title="This band overlaps another active band">
          Change or deactivate the other band first.
        </Alert>
      );
    case 'VALIDATION_FAILED':
      return (
        <Alert tone="warning" title="We could not save that">
          Check the details and try again.
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
