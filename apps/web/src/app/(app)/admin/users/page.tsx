import type { Metadata } from 'next';

import { PageHeader } from '../../../../components/layout/page-header';
import { RequireRole } from '../../../../components/layout/require-role';

export const metadata: Metadata = { title: 'Users — SwiftDrop' };

/**
 * A stand-in that shows the pattern for a page only some roles may open: wrap
 * the content in RequireRole, and anyone else sees the 403 page inside the
 * shell. Replace the body with the real user list (API 2.10 to 2.14).
 */
export default function AdminUsersPage() {
  return (
    <RequireRole roles={['ADMIN']} audience="administrators">
      <PageHeader
        title="Users"
        description="Accounts for couriers, dispatchers and administrators. This screen is not built yet."
      />
    </RequireRole>
  );
}
