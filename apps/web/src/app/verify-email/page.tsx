import type { Metadata } from 'next';
import { Suspense } from 'react';

import { VerifyEmailView } from '@/components/auth/verify-email';
import { AuthHeading, AuthLayout } from '@/components/layout/auth-layout';
import { RegisterPanel } from '@/components/auth/panels';

export const metadata: Metadata = { title: 'Verify your email — SwiftDrop' };

export default function VerifyEmailPage() {
  return (
    <AuthLayout panel={<RegisterPanel />}>
      <AuthHeading eyebrow="Email verification" title="Verify your email">
        Confirming your address activates your SwiftDrop account.
      </AuthHeading>
      <Suspense>
        <VerifyEmailView />
      </Suspense>
    </AuthLayout>
  );
}
