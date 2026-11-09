'use client';

import { DashboardPlaceholder } from '@/components/auth/dashboard-placeholder';
import { MerchantDashboardView } from '@/components/merchant/dashboard-view';
import { useSession } from '@/lib/session';

function DashboardContent() {
  const { session } = useSession();
  if (session.status === 'authenticated' && session.user.role === 'MERCHANT') {
    return <MerchantDashboardView />;
  }
  return <DashboardPlaceholder />;
}

export default function DashboardPage() {
  return <DashboardContent />;
}
