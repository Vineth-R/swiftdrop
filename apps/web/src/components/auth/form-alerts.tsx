import { Clock } from 'lucide-react';
import type { Ref } from 'react';

import { Alert } from '@/components/ui/alert';
import { ReferenceLine } from '@/components/ui/reference-line';

/** A 429. The typed values stay; only the button waits. */
export function RateLimitedAlert({
  remaining,
  alertRef,
  isPublic,
}: {
  remaining: number;
  alertRef?: Ref<HTMLDivElement>;
  /** On a page with no login, leaves out the line about the account. */
  isPublic?: boolean;
}) {
  const reassurance = isPublic ? '' : ' Nothing is wrong with your account.';
  return (
    <Alert
      ref={alertRef}
      tone="warning"
      icon={Clock}
      title="Too many attempts from this device"
    >
      {remaining > 0 ? (
        <>
          Wait{' '}
          <span className="font-mono font-semibold text-text">
            {remaining} seconds
          </span>{' '}
          and try again.{reassurance}
        </>
      ) : (
        `You can try again now.${reassurance}`
      )}
    </Alert>
  );
}

/**
 * A fault on our side or in the network. The Reference line is for faults on
 * our side only: on a 4xx it would read as blame.
 */
export function ServerFaultAlert({
  isNetwork,
  requestId,
  alertRef,
}: {
  isNetwork: boolean;
  requestId?: string;
  alertRef?: Ref<HTMLDivElement>;
}) {
  return (
    <Alert
      ref={alertRef}
      tone="danger"
      title={
        isNetwork
          ? 'We could not reach SwiftDrop'
          : 'Something broke on our side'
      }
    >
      {isNetwork
        ? 'Check your connection and try again. What you typed is still here.'
        : 'Please try again in a moment. What you typed is still here.'}
      <ReferenceLine requestId={requestId} className="mt-2 block" />
    </Alert>
  );
}
