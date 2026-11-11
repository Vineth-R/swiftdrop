'use client';

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react';

import { ApiError, onSessionExpired } from './api';
import { getMe, login, logout, type User } from './auth-api';
import { emitSessionIdentity } from './session-events';

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
  /**
   * True when a signed-in session ended while its page was open. The state
   * stays `authenticated` so the page keeps showing; the signed-in gate puts
   * the "session ended" dialog over it instead of replacing it with the login
   * page. It is never true for someone who had no session to begin with.
   */
  isExpired: boolean;
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
  const [isExpired, setIsExpired] = useState(false);
  const [attempt, setAttempt] = useState(0);
  // The expiry listener below is registered once, so it reads the state here.
  const current = useRef(session);
  current.current = session;

  useEffect(() => {
    const controller = new AbortController();
    getMe()
      .then((user) => {
        if (controller.signal.aborted) return;
        emitSessionIdentity(user.id);
        setSession({ status: 'authenticated', user });
      })
      .catch((error: unknown) => {
        if (controller.signal.aborted) return;
        const isSignedOut =
          error instanceof ApiError &&
          (error.code === 'UNAUTHENTICATED' ||
            error.code === 'ACCOUNT_DEACTIVATED');
        if (isSignedOut) emitSessionIdentity(null);
        setSession({ status: isSignedOut ? 'unauthenticated' : 'unavailable' });
      });
    return () => controller.abort();
  }, [attempt]);

  useEffect(
    () =>
      onSessionExpired(() => {
        emitSessionIdentity(null);
        // Someone who was signed in keeps their page and is told, in a dialog.
        // Someone who never was (a cold visit) goes to the login page at once.
        if (current.current.status === 'authenticated') {
          setIsExpired(true);
        } else {
          setSession({ status: 'unauthenticated' });
        }
      }),
    [],
  );

  const signIn = useCallback(async (email: string, password: string) => {
    const user = await login(email, password);
    emitSessionIdentity(user.id);
    setIsExpired(false);
    setSession({ status: 'authenticated', user });
    return user;
  }, []);

  const signOut = useCallback(async () => {
    // The caller navigates away afterwards. The state is left alone here on
    // purpose: marking the session empty first would let the signed-in guard
    // redirect to the login page before the caller can show "signed out".
    await logout();
    emitSessionIdentity(null);
  }, []);

  const setUser = useCallback((user: User) => {
    emitSessionIdentity(user.id);
    setIsExpired(false);
    setSession({ status: 'authenticated', user });
  }, []);

  const recheck = useCallback(() => {
    setIsExpired(false);
    setSession({ status: 'loading' });
    setAttempt((count) => count + 1);
  }, []);

  const value = useMemo(
    () => ({ session, isExpired, signIn, signOut, setUser, recheck }),
    [session, isExpired, signIn, signOut, setUser, recheck],
  );
  return <SessionContext value={value}>{children}</SessionContext>;
}

export function useSession(): SessionContextValue {
  const value = useContext(SessionContext);
  if (!value)
    throw new Error('useSession must be used inside <SessionProvider>');
  return value;
}
