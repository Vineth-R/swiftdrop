import type { Metadata } from 'next';

import { BusinessProfileView } from '../../../../components/merchant/profile-view';
import { RequireRole } from '../../../../components/layout/require-role';

export const metadata: Metadata = { title: 'Business profile — SwiftDrop' };

export default function MerchantProfilePage() {
  return (
    <RequireRole roles={['MERCHANT']} audience="approved merchants">
      <BusinessProfileView />
    </RequireRole>
  );
}
