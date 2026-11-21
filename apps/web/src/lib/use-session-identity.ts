'use client';

import { useSyncExternalStore } from 'react';

import {
  getServerSessionIdentityVersion,
  getServerSessionRole,
  getSessionIdentityVersion,
  getSessionRole,
  subscribeSessionIdentity,
} from './session-events';
import type { Role } from './tokens';

/**
 * A version number that changes only when the signed-in identity changes.
 * Components use it as a React key to discard per-identity state.
 */
export function useSessionIdentityVersion(): number {
  return useSyncExternalStore(
    subscribeSessionIdentity,
    getSessionIdentityVersion,
    getServerSessionIdentityVersion,
  );
}

/**
 * The verified role for the current session, or null for a guest. It comes from
 * the same trusted `GET /users/me` response the rest of the app uses; it is only
 * used to tailor suggestions and copy, never to authorize a request.
 */
export function useSessionRole(): Role | null {
  return useSyncExternalStore(
    subscribeSessionIdentity,
    getSessionRole,
    getServerSessionRole,
  );
}
