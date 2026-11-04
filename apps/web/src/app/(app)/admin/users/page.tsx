import type { Metadata } from 'next';

import { UsersView } from '../../../../components/admin/users-view';
import { RequireRole } from '../../../../components/layout/require-role';

export const metadata: Metadata = { title: 'Users — SwiftDrop' };

/** Administrators only: anyone else sees the 403 page inside the shell. */
export default function AdminUsersPage() {
  return (
    <RequireRole roles={['ADMIN']} audience="administrators">
      <UsersView />
    </RequireRole>
  );
}
