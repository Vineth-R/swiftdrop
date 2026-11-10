import type { Metadata } from 'next';

import { PageHeader } from '@/components/layout/page-header';
import { Section } from '@/components/layout/section';
import { Alert } from '@/components/ui/alert';
import { ButtonLink } from '@/components/ui/button';

export const metadata: Metadata = { title: 'Merchant application — SwiftDrop' };

export default function MerchantPendingPage() {
  return (
    <>
      <PageHeader
        title="Merchant application"
        description="Your business application is reviewed by a SwiftDrop administrator."
      />
      <Section title="What happens next">
        <div className="flex max-w-2xl flex-col gap-4">
          <Alert tone="info" title="Login is disabled until approval">
            Merchant accounts cannot sign in until an administrator approves the
            application. This page is informational only and does not show
            application details.
          </Alert>
          <ol className="flex flex-col gap-3 text-body text-text-muted">
            <li>1. We received your application and its confirmation email.</li>
            <li>2. An administrator reviews your business details.</li>
            <li>3. We email you when the decision is made.</li>
            <li>
              4. After approval, sign in and manage your API keys and profile.
            </li>
          </ol>
          <p className="text-body-sm text-text-muted">
            If you received a rejection or suspension email, it explains the
            reason and how to contact support. If you need help, reply to that
            email or write to SwiftDrop support.
          </p>
          <div className="flex flex-wrap gap-3">
            <ButtonLink href="/login">Go to log in</ButtonLink>
            <ButtonLink href="/register/merchant" variant="secondary">
              Register another business
            </ButtonLink>
          </div>
        </div>
      </Section>
    </>
  );
}
