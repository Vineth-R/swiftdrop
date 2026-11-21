import type { Metadata } from 'next';

import { RequireRole } from '../../../../components/layout/require-role';
import { PricingView } from '../../../../components/pricing/pricing-view';

export const metadata: Metadata = { title: 'Pricing — SwiftDrop' };

/** Administrators only: anyone else sees the 403 page inside the shell. */
export default function AdminPricingPage() {
  return (
    <RequireRole roles={['ADMIN']} audience="administrators">
      <PricingView />
    </RequireRole>
  );
}
