import type { Metadata } from 'next';

import { LoginPanel } from '../../../components/auth/panels';
import { AuthLayout } from '../../../components/layout/auth-layout';
import { ButtonLink } from '../../../components/ui/button';
import { RouteLine } from '../../../components/ui/route-line';

export const metadata: Metadata = { title: 'You are signed out — SwiftDrop' };

export default function SignedOutPage() {
  return (
    <AuthLayout panel={<LoginPanel />} showRoute={false}>
      <div className="flex flex-col gap-6">
        <RouteLine state="idle" />
        <div className="flex flex-col gap-2">
          <h1 className="font-display text-h2">You are signed out</h1>
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
