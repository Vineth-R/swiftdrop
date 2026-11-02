import type { Metadata } from 'next';
import { Suspense } from 'react';

import { RecoveryPanel } from '../../../components/auth/panels';
import { ResetPasswordForm } from '../../../components/auth/reset-form';
import { AuthLayout } from '../../../components/layout/auth-layout';

export const metadata: Metadata = {
  title: 'Set a new password — SwiftDrop',
  // The address carries the one-time reset token, so it must not leak in a Referer header.
  referrer: 'no-referrer',
};

export default function ResetPasswordPage() {
  return (
    <AuthLayout panel={<RecoveryPanel />}>
      <Suspense>
        <ResetPasswordForm />
      </Suspense>
    </AuthLayout>
  );
}
