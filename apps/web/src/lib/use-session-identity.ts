'use client';

import { useSyncExternalStore } from 'react';

import {
  getServerSessionIdentityVersion,
  getSessionIdentityVersion,
  subscribeSessionIdentity,
} from './session-events';

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
