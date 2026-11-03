'use client';

import type { ReactNode } from 'react';

import { useSession } from '../../lib/session';
import { AppShell } from './app-shell';
import { CourierShell } from './courier-shell';

/**
 * Picks the frame for the signed-in role. Mount it inside SignedInOnly, which
 * guarantees there is a user. Customers, merchants, dispatchers and admins share
 * AppShell; couriers get the mobile-first CourierShell.
 */
export function RoleShell({ children }: { children: ReactNode }) {
  const { session } = useSession();
  if (session.status !== 'authenticated') return null;

  const { user } = session;
  return user.role === 'COURIER' ? (
    <CourierShell user={user}>{children}</CourierShell>
  ) : (
    <AppShell user={user}>{children}</AppShell>
  );
}
