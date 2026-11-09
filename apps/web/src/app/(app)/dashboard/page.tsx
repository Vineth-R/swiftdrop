import type { Metadata } from 'next';

import { CourierDashboard } from '@/components/courier/courier-dashboard';

export const metadata: Metadata = { title: 'Dashboard — SwiftDrop' };

export default function DashboardPage() {
  return <CourierDashboard />;
}
