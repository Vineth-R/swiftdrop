import type { Metadata } from 'next';

import { DashboardPlaceholder } from '../../../components/auth/dashboard-placeholder';

export const metadata: Metadata = { title: 'Dashboard — SwiftDrop' };

export default function DashboardPage() {
  return <DashboardPlaceholder />;
}
