'use client';

import { PageHeader } from '../layout/page-header';
import { Section } from '../layout/section';
import { ApprovalNotice, LoadState, useMerchant } from './shared';
import { WebhookDeliveries } from './webhook-deliveries';
import { WebhookSettings } from './webhook-settings';

/** The merchant's webhook: where events go, a test, and what was delivered. */
export function WebhookView() {
  const profile = useMerchant();
  const merchant = profile.merchant;
  const isReady = !profile.loading && !profile.error && merchant;

  return (
    <>
      <PageHeader
        title="Webhook"
        description="Get a signed message on your server each time one of your parcels changes status."
      />
      <LoadState {...profile} />
      {isReady && merchant.status !== 'APPROVED' && (
        <ApprovalNotice merchant={merchant} />
      )}
      {isReady && merchant.status === 'APPROVED' && (
        <>
          <Section title="Settings">
            <WebhookSettings />
          </Section>
          <Section
            title="Delivery log"
            description="Every attempt, including retries. A failed delivery is tried up to six times."
          >
            <WebhookDeliveries />
          </Section>
        </>
      )}
    </>
  );
}
