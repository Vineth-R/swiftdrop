import type { Metadata } from 'next';
import Link from 'next/link';
import { Suspense } from 'react';

import { LoginForm } from '../../../components/auth/login-form';
import { LoginPanel } from '../../../components/auth/panels';
import {
  AuthHeading,
  AuthLayout,
} from '../../../components/layout/auth-layout';

export const metadata: Metadata = { title: 'Log in — SwiftDrop' };

export default function LoginPage() {
  return (
    <AuthLayout panel={<LoginPanel />} showRoute={false}>
      <AuthHeading title="Log in">
        No account?{' '}
        <Link
          href="/register"
          className="text-interactive underline underline-offset-4 hover:text-interactive-hover"
        >
          Create one
        </Link>
        .
      </AuthHeading>
      <Suspense>
        <LoginForm />
      </Suspense>
    </AuthLayout>
  );
}
