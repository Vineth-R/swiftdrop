import type { Metadata } from 'next';

import { MerchantsView } from '../../../../components/admin/merchants-view';
import { RequireRole } from '../../../../components/layout/require-role';

export const metadata: Metadata = { title: 'Merchants — SwiftDrop' };

export default function AdminMerchantsPage() {
  return (
    <RequireRole roles={['ADMIN']} audience="administrators">
      <MerchantsView />
    </RequireRole>
  );
}
