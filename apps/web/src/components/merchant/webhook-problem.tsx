'use client';

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type Ref,
  type RefObject,
} from 'react';

import { ApiError } from '../../lib/api';
import { useSession } from '../../lib/session';
import { useCountdown } from '../../lib/use-countdown';
import { RateLimitedAlert, ServerFaultAlert } from '../auth/form-alerts';
import { Alert } from '../ui/alert';

/** What went wrong with a webhook call, in terms the screen has words for. */
export type Problem =
  | { kind: 'rate-limited'; seconds: number }
  | { kind: 'server'; isNetwork: boolean; requestId?: string }
  | { kind: 'not-approved' }
  | { kind: 'not-configured' };

/**
 * Holds the one problem a panel is showing and sorts an error into it. A lost
 * session needs no message (the shell handles it) and a deactivated account
 * asks the session to check itself. Focus moves to the alert when it appears.
 */
export function useWebhookProblem() {
  const { recheck } = useSession();
  const [problem, setProblem] = useState<Problem | null>(null);
  const alertRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (problem) alertRef.current?.focus();
  }, [problem]);

  const clear = useCallback(() => setProblem(null), []);

  const fail = useCallback(
    (error: unknown) => {
      if (!(error instanceof ApiError)) {
        setProblem({ kind: 'server', isNetwork: false });
      } else if (error.code === 'UNAUTHENTICATED') {
        // The session gate is already sending the person to log in.
      } else if (error.code === 'ACCOUNT_DEACTIVATED') {
        recheck();
      } else if (error.code === 'MERCHANT_NOT_APPROVED') {
        setProblem({ kind: 'not-approved' });
      } else if (error.code === 'FORBIDDEN') {
        setProblem({ kind: 'not-approved' });
      } else if (error.code === 'NOT_FOUND') {
        setProblem({ kind: 'not-configured' });
      } else if (error.code === 'RATE_LIMITED') {
        setProblem({ kind: 'rate-limited', seconds: error.retryAfter ?? 60 });
      } else {
        setProblem({
          kind: 'server',
          isNetwork: error.code === 'NETWORK',
          requestId: error.code === 'INTERNAL' ? error.requestId : undefined,
        });
      }
    },
    [recheck],
  );

  return { problem, fail, clear, alertRef };
}

function RateLimited({
  seconds,
  alertRef,
}: {
  seconds: number;
  alertRef: Ref<HTMLDivElement>;
}) {
  const remaining = useCountdown(seconds);
  return <RateLimitedAlert alertRef={alertRef} remaining={remaining} />;
}

/** The same notice for a panel that has no other state yet. */
export function WebhookNotApproved({
  alertRef,
}: {
  alertRef?: Ref<HTMLDivElement>;
}) {
  return (
    <Alert ref={alertRef} tone="warning" title="Webhook is not available yet">
      Webhook settings open once an administrator approves your business.
    </Alert>
  );
}

/** Shows a Problem. Our own words only: nothing the server said is printed. */
export function WebhookProblem({
  problem,
  alertRef,
}: {
  problem: Problem;
  alertRef: RefObject<HTMLDivElement | null>;
}) {
  switch (problem.kind) {
    case 'rate-limited':
      return <RateLimited seconds={problem.seconds} alertRef={alertRef} />;
    case 'server':
      return (
        <ServerFaultAlert
          alertRef={alertRef}
          isNetwork={problem.isNetwork}
          requestId={problem.requestId}
        />
      );
    case 'not-approved':
      return <WebhookNotApproved alertRef={alertRef} />;
    case 'not-configured':
      return (
        <Alert ref={alertRef} tone="info" title="Save your webhook first">
          Add an address above and save it. Then you can send a test event.
        </Alert>
      );
  }
}
