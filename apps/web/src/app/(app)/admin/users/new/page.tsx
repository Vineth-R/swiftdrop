import type { Metadata } from 'next';

import { CreateUserView } from '../../../../../components/admin/create-user-view';
import { RequireRole } from '../../../../../components/layout/require-role';

export const metadata: Metadata = { title: 'Create user — SwiftDrop' };

/** Administrators only: anyone else sees the 403 page inside the shell. */
export default function CreateUserPage() {
  return (
    <RequireRole roles={['ADMIN']} audience="administrators">
      <CreateUserView />
    </RequireRole>
  );
}
