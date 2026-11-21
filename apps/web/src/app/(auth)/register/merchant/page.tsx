import type { Metadata } from 'next';
import Link from 'next/link';

import { RegisterPanel } from '@/components/auth/panels';
import { MerchantRegisterForm } from '@/components/merchant/register-form';
import { AuthHeading, AuthLayout } from '@/components/layout/auth-layout';

export const metadata: Metadata = {
  title: 'Merchant registration — SwiftDrop',
};

export default function MerchantRegisterPage() {
  return (
    <AuthLayout panel={<RegisterPanel />} width="wide">
      <AuthHeading eyebrow="Merchant account" title="Register your business">
        Already registered?{' '}
        <Link
          href="/login"
          className="text-interactive underline underline-offset-4 hover:text-interactive-hover"
        >
          Log in
        </Link>
        . Sending a personal parcel?{' '}
        <Link
          href="/register/customer"
          className="text-interactive underline underline-offset-4 hover:text-interactive-hover"
        >
          Create a customer account
        </Link>
        .
      </AuthHeading>
      <MerchantRegisterForm />
    </AuthLayout>
  );
}
