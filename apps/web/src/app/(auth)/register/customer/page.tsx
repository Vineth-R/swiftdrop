import type { Metadata } from 'next';
import Link from 'next/link';
import { Suspense } from 'react';

import { RegisterPanel } from '@/components/auth/panels';
import { RegisterForm } from '@/components/auth/register-form';
import { AuthHeading, AuthLayout } from '@/components/layout/auth-layout';

export const metadata: Metadata = {
  title: 'Customer registration — SwiftDrop',
};

export default function CustomerRegisterPage() {
  return (
    <AuthLayout panel={<RegisterPanel />} width="wide">
      <AuthHeading eyebrow="Customer account" title="Create your account">
        Already have one?{' '}
        <Link
          href="/login"
          className="text-interactive underline underline-offset-4 hover:text-interactive-hover"
        >
          Log in
        </Link>
        . Registering a business?{' '}
        <Link
          href="/register/merchant"
          className="text-interactive underline underline-offset-4 hover:text-interactive-hover"
        >
          Use the merchant form
        </Link>
        .
      </AuthHeading>
      <Suspense>
        <RegisterForm />
      </Suspense>
    </AuthLayout>
  );
}
