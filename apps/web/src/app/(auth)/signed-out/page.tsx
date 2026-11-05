import type { Metadata } from 'next';

import { SignedOutPanel } from '@/components/auth/panels';
import { AuthLayout } from '@/components/layout/auth-layout';
import { ButtonLink } from '@/components/ui/button';

export const metadata: Metadata = { title: 'You are signed out — SwiftDrop' };

export default function SignedOutPage() {
  return (
    <AuthLayout panel={<SignedOutPanel />}>
      <div className="flex flex-col gap-6">
        <div className="flex flex-col gap-2">
          <span className="text-overline uppercase text-text-faint">
            Signed out
          </span>
          <h1 className="font-display text-h2 md:text-h1">
            You are signed out
          </h1>
          <p className="text-body text-text-muted">
            This session has been closed on this device. Your parcels keep
            moving.
          </p>
        </div>
        <div className="flex flex-col gap-3">
          <ButtonLink href="/login" size="lg" isFullWidth>
            Log in again
          </ButtonLink>
          <ButtonLink href="/" variant="secondary" size="lg" isFullWidth>
            Back to home
          </ButtonLink>
        </div>
      </div>
    </AuthLayout>
  );
}
