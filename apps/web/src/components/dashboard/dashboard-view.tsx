'use client';

import { useRouter } from 'next/navigation';
import { useEffect } from 'react';

import { useSession } from '@/lib/session';
import { CourierDashboard } from '../courier/courier-dashboard';
import { Spinner } from '../ui/spinner';
import { AdminDashboard } from './admin-dashboard';
import { CustomerDashboard } from './customer-dashboard';
import { MerchantDashboard } from './merchant-dashboard';

/**
 * `/dashboard` is one route for every role, so this view chooses the role's own
 * home. A dispatcher has no dashboard: their home is the unassigned list, and
 * they are sent there if they open this address.
 */
export function DashboardView() {
  const { session } = useSession();
  const router = useRouter();
  const role = session.status === 'authenticated' ? session.user.role : null;

  useEffect(() => {
    if (role === 'DISPATCHER') router.replace('/unassigned');
  }, [role, router]);

  if (session.status !== 'authenticated') return null;
  const { user } = session;

  switch (user.role) {
    case 'COURIER':
      return <CourierDashboard />;
    case 'CUSTOMER':
      return <CustomerDashboard name={user.fullName} />;
    case 'MERCHANT':
      return <MerchantDashboard name={user.fullName} />;
    case 'ADMIN':
      return <AdminDashboard name={user.fullName} />;
    default:
      return <Spinner tone="standalone" label="Opening your page" />;
  }
}
