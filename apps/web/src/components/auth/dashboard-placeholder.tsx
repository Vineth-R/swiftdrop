'use client';

import { useRouter } from 'next/navigation';
import { useState } from 'react';

import { useSession } from '../../lib/session';
import { Button } from '../ui/button';
import { Logo } from '../ui/logo';

/**
 * Stands in for every role's dashboard until the app shell exists. It proves
 * the sign-in, the role and sign-out all work end to end. Replace it with the
 * real dashboards; keep the session gate in app/(app)/layout.tsx.
 */
export function DashboardPlaceholder() {
  const { session, signOut } = useSession();
  const router = useRouter();
  const [isLeaving, setIsLeaving] = useState(false);

  if (session.status !== 'authenticated') return null;
  const { user } = session;

  async function leave() {
    setIsLeaving(true);
    try {
      await signOut();
    } catch {
      // The server could not be reached; this browser still leaves the signed-in screens.
    }
    router.replace('/signed-out');
  }

  return (
    <main className="mx-auto flex min-h-screen max-w-content flex-col gap-8 px-4 py-10 md:px-8">
      <Logo />
      <div className="flex flex-col gap-2">
        <h1 className="font-display text-h1">Dashboard</h1>
        <p className="text-body text-text-muted">
          Signed in as{' '}
          <span className="font-semibold text-text">{user.fullName}</span>{' '}
          <span className="font-mono text-body-sm">({user.email})</span> with
          the role <span className="font-mono text-body-sm">{user.role}</span>.
        </p>
        <p className="text-body-sm text-text-faint">
          This is a placeholder until each role gets its own dashboard.
        </p>
      </div>
      <div>
        <Button variant="secondary" isLoading={isLeaving} onClick={leave}>
          {isLeaving ? 'Logging out' : 'Log out'}
        </Button>
      </div>
    </main>
  );
}
