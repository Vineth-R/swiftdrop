import type { Ref } from 'react';

import type { MerchantStanding } from '@/lib/merchant-standing';
import { Alert } from '@/components/ui/alert';

const copy = {
  PENDING: {
    tone: 'warning',
    title: 'Application under review',
    body: 'An administrator is still reviewing your business application. You can sign in once it is approved, and we will email you the decision.',
  },
  REJECTED: {
    tone: 'danger',
    title: 'Application not approved',
    body: 'An administrator did not approve this application, so this account cannot sign in.',
  },
  SUSPENDED: {
    tone: 'danger',
    title: 'Merchant account suspended',
    body: 'An administrator suspended this account, so it cannot sign in.',
  },
} as const;

/**
 * Tells a merchant owner where their application stands, after a correct
 * password. The reason is the administrator's own text for this owner, shown as
 * plain text; it is never an API error message.
 */
export function MerchantStatusAlert({
  standing,
  alertRef,
}: {
  standing: MerchantStanding;
  alertRef: Ref<HTMLDivElement>;
}) {
  const text = copy[standing.status];
  const hasDecision = standing.status !== 'PENDING';

  return (
    <Alert ref={alertRef} tone={text.tone} title={text.title}>
      <p>{text.body}</p>
      {hasDecision ? (
        <div className="mt-3">
          <p className="font-semibold text-text">
            Reason from the administrator
          </p>
          <p className="mt-1 whitespace-pre-line break-words">
            {standing.reason ?? 'No reason was given.'}
          </p>
        </div>
      ) : null}
      {hasDecision ? (
        <p className="mt-3">
          {standing.supportEmail ? (
            <>
              To ask about this decision, write to{' '}
              <a
                href={`mailto:${standing.supportEmail}`}
                className="font-semibold text-interactive underline underline-offset-4"
              >
                {standing.supportEmail}
              </a>
              .
            </>
          ) : (
            'To ask about this decision, contact SwiftDrop support.'
          )}
        </p>
      ) : null}
    </Alert>
  );
}
