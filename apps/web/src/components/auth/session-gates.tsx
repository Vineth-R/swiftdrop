'use client';

import { usePathname, useRouter, useSearchParams } from 'next/navigation';
import { useEffect, useRef, type ReactNode } from 'react';

import { homeFor } from '@/lib/nav';
import { safeNext } from '@/lib/redirect';
import { useSession, type SessionState } from '@/lib/session';
import { Splash } from './splash';

/**
 * Wraps pages that need a signed-in user. The splash shows while the cookie is
 * checked; a signed-out visitor is sent to the login page and brought back to
 * this path afterwards.
 */
export function SignedInOnly({ children }: { children: ReactNode }) {
  const { session, recheck } = useSession();
  const router = useRouter();
  const pathname = usePathname();

  useEffect(() => {
    if (session.status === 'unauthenticated') {
      router.replace(`/login?next=${encodeURIComponent(pathname)}`);
    }
  }, [session.status, router, pathname]);

  if (session.status === 'authenticated') return <>{children}</>;
  return (
    <Splash
      state={session.status === 'unavailable' ? 'unavailable' : 'checking'}
      onRetry={recheck}
    />
  );
}

/**
 * Wraps the login, register and reset pages. Someone who opens one of them
 * while already signed in goes straight to where they were headed. Signing in
 * on the page itself is not redirected here: each form decides where it goes.
 */
export function SignedOutOnly({ children }: { children: ReactNode }) {
  const { session } = useSession();
  const router = useRouter();
  const params = useSearchParams();
  const firstResult = useRef<SessionState['status'] | null>(null);

  useEffect(() => {
    if (firstResult.current !== null || session.status === 'loading') return;
    firstResult.current = session.status;
    if (session.status === 'authenticated') {
      router.replace(safeNext(params.get('next'), homeFor(session.user.role)));
    }
  }, [session, router, params]);

  return <>{children}</>;
}
