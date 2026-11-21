import type { Metadata } from 'next';
import Link from 'next/link';

import { RegisterPanel } from '@/components/auth/panels';
import { AuthHeading, AuthLayout } from '@/components/layout/auth-layout';
import { ButtonLink } from '@/components/ui/button';

export const metadata: Metadata = { title: 'Create your account — SwiftDrop' };

export default function RegisterPage() {
  return (
    <AuthLayout panel={<RegisterPanel />} width="wide">
      <AuthHeading eyebrow="New account" title="How will you use SwiftDrop?">
        Already have one?{' '}
        <Link
          href="/login"
          className="text-interactive underline underline-offset-4 hover:text-interactive-hover"
        >
          Log in
        </Link>
        .
      </AuthHeading>

      <div className="flex flex-col gap-4">
        <section className="rounded-card border border-line bg-surface p-5">
          <p className="text-overline uppercase text-text-muted">Customer</p>
          <h2 className="mt-2 font-display text-h3">Send and track parcels</h2>
          <p className="mt-2 text-body-sm text-text-muted">
            Create a personal account to request deliveries, track parcels and
            manage your shipments.
          </p>
          <ButtonLink
            href="/register/customer"
            size="lg"
            isFullWidth
            className="mt-5"
          >
            Register as Customer
          </ButtonLink>
        </section>

        <section className="rounded-card border border-line bg-surface p-5">
          <p className="text-overline uppercase text-text-muted">Merchant</p>
          <h2 className="mt-2 font-display text-h3">
            Deliver for your business
          </h2>
          <p className="mt-2 text-body-sm text-text-muted">
            Register your business with SwiftDrop, integrate your systems and
            manage deliveries through our merchant platform.
          </p>
          <ButtonLink
            href="/register/merchant"
            variant="secondary"
            size="lg"
            isFullWidth
            className="mt-5"
          >
            Register as Merchant
          </ButtonLink>
        </section>
      </div>
    </AuthLayout>
  );
}
