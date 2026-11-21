import type { Metadata } from 'next';

import { RequireRole } from '../../../../components/layout/require-role';
import { ZonesView } from '../../../../components/pricing/zones-view';

export const metadata: Metadata = { title: 'Zones — SwiftDrop' };

/** Administrators only: anyone else sees the 403 page inside the shell. */
export default function AdminZonesPage() {
  return (
    <RequireRole roles={['ADMIN']} audience="administrators">
      <ZonesView />
    </RequireRole>
  );
}
