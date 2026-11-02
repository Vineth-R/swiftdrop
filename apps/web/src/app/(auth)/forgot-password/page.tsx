import type { Metadata } from 'next';

import { RecoveryPanel } from '../../../components/auth/panels';
import { RequestResetForm } from '../../../components/auth/request-reset';
import { AuthLayout } from '../../../components/layout/auth-layout';

export const metadata: Metadata = {
  title: 'Forgot your password? — SwiftDrop',
};

export default function ForgotPasswordPage() {
  return (
    <AuthLayout panel={<RecoveryPanel />}>
      <RequestResetForm />
    </AuthLayout>
  );
}
