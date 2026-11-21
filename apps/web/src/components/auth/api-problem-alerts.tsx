import type { Ref } from 'react';

import type { ApiProblem } from '@/lib/api-problem';
import { RateLimitedAlert, ServerFaultAlert } from './form-alerts';

type ApiProblemAlertsProps = {
  problem: ApiProblem | null;
  alertRef?: Ref<HTMLDivElement>;
  /** Seconds left on a rate limit, from useCountdown. */
  remaining?: number;
};

/**
 * The alerts the auth screens share. Validation is rendered as a server fault
 * only when the form could not place the issue on a field; field messages stay
 * with their form.
 */
export function ApiProblemAlerts({
  problem,
  alertRef,
  remaining = 0,
}: ApiProblemAlertsProps) {
  if (!problem) return null;

  switch (problem.kind) {
    case 'rate-limited':
      return <RateLimitedAlert alertRef={alertRef} remaining={remaining} />;
    case 'network':
      return <ServerFaultAlert alertRef={alertRef} isNetwork />;
    case 'server':
      return (
        <ServerFaultAlert
          alertRef={alertRef}
          isNetwork={false}
          requestId={problem.requestId}
        />
      );
    case 'validation':
      // API v1.1 defines only field-level `details` for validation errors and
      // no approved generic validation copy. Forms map known fields before
      // delegating, so keep the previous server-fault presentation rather than
      // inventing a new message.
      return <ServerFaultAlert alertRef={alertRef} isNetwork={false} />;
  }
}
