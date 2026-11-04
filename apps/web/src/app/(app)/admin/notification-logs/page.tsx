import type { Metadata } from 'next';

import { NotificationLogsView } from '../../../../components/notification-logs/notification-logs-view';
import { RequireRole } from '../../../../components/layout/require-role';

export const metadata: Metadata = { title: 'Notification logs — SwiftDrop' };

/** ADMIN-only read access to the notification worker's delivery records. */
export default function AdminNotificationLogsPage() {
  return (
    <RequireRole roles={['ADMIN']} audience="administrators">
      <NotificationLogsView />
    </RequireRole>
  );
}
