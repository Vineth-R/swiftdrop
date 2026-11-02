import { Clock } from 'lucide-react';
import type { Ref } from 'react';

import { Alert } from '../ui/alert';

/** A 429. The typed values stay; only the button waits. */
export function RateLimitedAlert({
  remaining,
  alertRef,
}: {
  remaining: number;
  alertRef?: Ref<HTMLDivElement>;
}) {
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
          and try again. Nothing is wrong with your account.
        </>
      ) : (
        'You can try again now. Nothing is wrong with your account.'
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
      {requestId && (
        <span className="mt-2 block select-all font-mono text-overline text-text-faint">
          Reference {requestId}
        </span>
      )}
    </Alert>
  );
}
