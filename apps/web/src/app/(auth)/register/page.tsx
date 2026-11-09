import type { Metadata } from 'next';
import Link from 'next/link';
import { Suspense } from 'react';

import { RegisterPanel } from '@/components/auth/panels';
import { RegisterForm } from '@/components/auth/register-form';
import { AuthHeading, AuthLayout } from '@/components/layout/auth-layout';

export const metadata: Metadata = { title: 'Create your account — SwiftDrop' };

export default function RegisterPage() {
  return (
    <AuthLayout panel={<RegisterPanel />} width="wide">
      <AuthHeading eyebrow="New account" title="Create your account">
        Already have one?{' '}
        <Link
          href="/login"
          className="text-interactive underline underline-offset-4 hover:text-interactive-hover"
        >
          Log in
        </Link>
        .
      </AuthHeading>
      <p className="text-body-sm text-text-muted">Registering a business? <Link href="/merchant/register" className="text-interactive underline underline-offset-4">Create a merchant account</Link></p>
      <Suspense>
        <RegisterForm />
      </Suspense>
    </AuthLayout>
  );
}
