import type { ApiIssue } from './api-errors';

/**
 * Where a merchant owner's application stands, as the login tells them after a
 * correct password. A 403 MERCHANT_ACCOUNT_INACTIVE carries it in `details`
 * (API v1.1 section 2.2): `status`, then `reason` and `supportEmail` when there
 * are any.
 */
export type MerchantStanding = {
  status: 'PENDING' | 'REJECTED' | 'SUSPENDED';
  /** The administrator's own words. Empty when none was given. */
  reason?: string;
  supportEmail?: string;
};

const STATUSES: MerchantStanding['status'][] = [
  'PENDING',
  'REJECTED',
  'SUSPENDED',
];

const PLAIN_EMAIL = /^[^\s@]+@[^\s@]+$/;

/**
 * Reads the details of a refused merchant login. Returns undefined when the API
 * sent none, which is the plain refusal (an account an administrator switched
 * off, or a lookup that could not be made).
 */
export function merchantStandingFromIssues(
  issues: ApiIssue[],
): MerchantStanding | undefined {
  const value = (field: string) =>
    issues.find((item) => item.field === field)?.issue.trim() || undefined;

  const status = STATUSES.find((candidate) => candidate === value('status'));
  if (!status) return undefined;

  const supportEmail = value('supportEmail');
  return {
    status,
    reason: value('reason'),
    // It goes into a mailto link, so anything that is not a plain address is dropped.
    supportEmail:
      supportEmail && PLAIN_EMAIL.test(supportEmail) ? supportEmail : undefined,
  };
}
