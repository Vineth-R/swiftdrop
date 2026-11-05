import type { Metadata } from 'next';

import { EditUserView } from '../../../../../../components/admin/edit-user-view';
import { RequireRole } from '../../../../../../components/layout/require-role';

export const metadata: Metadata = { title: 'Edit user — SwiftDrop' };

/** Administrators only: anyone else sees the 403 page inside the shell. */
export default async function EditUserPage({
  params,
}: {
  params: Promise<{ userId: string }>;
}) {
  const { userId } = await params;
  return (
    <RequireRole roles={['ADMIN']} audience="administrators">
      <EditUserView userId={userId} />
    </RequireRole>
  );
}
