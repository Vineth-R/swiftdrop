'use client';

import { DashboardPlaceholder } from '../auth/dashboard-placeholder';
import { useSession } from '../../lib/session';

import { CourierTodayView } from './courier-today-view';

/** Keeps the shared dashboard intact for every role except its new courier path. */
export function CourierDashboard() {
  const { session } = useSession();

  if (session.status !== 'authenticated') return null;
  return session.user.role === 'COURIER' ? (
    <CourierTodayView />
  ) : (
    <DashboardPlaceholder />
  );
}
