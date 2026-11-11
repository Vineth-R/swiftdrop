import type { Metadata } from 'next';

import { MerchantOrdersView } from '../../../../components/merchant/orders-view';
import { RequireRole } from '../../../../components/layout/require-role';

export const metadata: Metadata = { title: 'Orders — SwiftDrop' };

export default function MerchantOrdersPage() {
  return (
    <RequireRole roles={['MERCHANT']} audience="merchants">
      <MerchantOrdersView />
    </RequireRole>
  );
}
