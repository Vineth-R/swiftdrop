import type { Metadata } from 'next';

import { AuditLogsView } from '../../../../components/admin/audit-logs-view';
import { RequireRole } from '../../../../components/layout/require-role';

export const metadata: Metadata = { title: 'Audit logs — SwiftDrop' };

/** ADMIN-only read access to identity, merchant and pricing audit records. */
export default function AdminAuditLogsPage() {
  return (
    <RequireRole roles={['ADMIN']} audience="administrators">
      <AuditLogsView />
    </RequireRole>
  );
}
