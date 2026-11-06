'use client';

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react';

import { ApiError, onSessionExpired } from './api';
import { getMe, login, logout, type User } from './auth-api';

/**
 * loading: the cookie is being checked (the splash shows).
 * unavailable: the check could not reach the API, which is not the same as
 *   being signed out, so the person is never bounced to the login page.
 */
export type SessionState =
  | { status: 'loading' }
  | { status: 'authenticated'; user: User }
  | { status: 'unauthenticated' }
  | { status: 'unavailable' };

type SessionContextValue = {
  session: SessionState;
  signIn: (email: string, password: string) => Promise<User>;
  /** Ends the session on the server. Navigate away when it settles. */
  signOut: () => Promise<void>;
  /** For a user the page just created or signed in by other means. */
  setUser: (user: User) => void;
  /** Check the session again, for example after the API was unreachable. */
  recheck: () => void;
};

const SessionContext = createContext<SessionContextValue | null>(null);

export function SessionProvider({ children }: { children: ReactNode }) {
  const [session, setSession] = useState<SessionState>({ status: 'loading' });
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    getMe()
      .then((user) => {
        if (!controller.signal.aborted)
          setSession({ status: 'authenticated', user });
      })
      .catch((error: unknown) => {
        if (controller.signal.aborted) return;
        const isSignedOut =
          error instanceof ApiError &&
          (error.code === 'UNAUTHENTICATED' ||
            error.code === 'ACCOUNT_DEACTIVATED');
        setSession({ status: isSignedOut ? 'unauthenticated' : 'unavailable' });
      });
    return () => controller.abort();
  }, [attempt]);

  useEffect(
    () => onSessionExpired(() => setSession({ status: 'unauthenticated' })),
    [],
  );

  const signIn = useCallback(async (email: string, password: string) => {
    const user = await login(email, password);
    setSession({ status: 'authenticated', user });
    return user;
  }, []);

  const signOut = useCallback(async () => {
    // The caller navigates away afterwards. The state is left alone here on
    // purpose: marking the session empty first would let the signed-in guard
    // redirect to the login page before the caller can show "signed out".
    await logout();
  }, []);

  const setUser = useCallback(
    (user: User) => setSession({ status: 'authenticated', user }),
    [],
  );

  const recheck = useCallback(() => {
    setSession({ status: 'loading' });
    setAttempt((count) => count + 1);
  }, []);

  const value = useMemo(
    () => ({ session, signIn, signOut, setUser, recheck }),
    [session, signIn, signOut, setUser, recheck],
  );
  return <SessionContext value={value}>{children}</SessionContext>;
}

export function useSession(): SessionContextValue {
  const value = useContext(SessionContext);
  if (!value)
    throw new Error('useSession must be used inside <SessionProvider>');
  return value;
}
