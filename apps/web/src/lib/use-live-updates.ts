'use client';

import { useEffect, useRef, useState } from 'react';

import { refreshSession } from './api';
import {
  createLiveConnection,
  type LiveState,
  type StatusUpdate,
} from './live-connection';

export type { LiveState, StatusUpdate } from './live-connection';

export type UseLiveUpdatesOptions = {
  /** Follow one parcel. Public pages may do this without signing in. */
  trackingNumber?: string;
  /**
   * Follow every parcel. This is a server-authorized capability for dispatchers
   * and administrators only. A merchant must never use it: merchant live views
   * must be scoped by the server to that merchant's own parcels.
   */
  scope?: 'all';
  /** Set to false to stay disconnected, for example while still loading. */
  enabled?: boolean;
  /** A parcel changed status. Update what the screen shows. */
  onUpdate: (update: StatusUpdate) => void;
  /**
   * Fetch the current state again. Called after every confirmed subscription,
   * the first one and after each reconnect, because the socket can miss events.
   */
  onSync?: () => void | Promise<unknown>;
};

/**
 * Live status updates for one parcel or for all parcels (API v1.1 section 9).
 *
 *   const { state } = useLiveUpdates({
 *     trackingNumber,
 *     onUpdate: (update) => setStatus(update.status),
 *     onSync: refetch,
 *   });
 *
 * `state` is 'idle' while disabled, then 'connecting', 'live', 'reconnecting',
 * 'denied' or 'limited'. Show it in words, not only a colour ("Live",
 * "Reconnecting"). 'limited' means the server closed the connection for
 * exceeding a subscription or message limit; it is not retried automatically.
 * Behind the gateway the address is the page's own host. For `nx dev`, set
 * NEXT_PUBLIC_WS_URL=ws://localhost:8080/ws in an untracked apps/web/.env.local
 * (see apps/web/.env.example).
 */
export function useLiveUpdates({
  trackingNumber,
  scope,
  enabled = true,
  onUpdate,
  onSync,
}: UseLiveUpdatesOptions): { state: LiveState | 'idle' } {
  const [state, setState] = useState<LiveState>('connecting');
  const updateRef = useRef(onUpdate);
  const syncRef = useRef(onSync);

  // Keep the latest callbacks without reconnecting when they change.
  useEffect(() => {
    updateRef.current = onUpdate;
    syncRef.current = onSync;
  });

  const active = enabled && (scope === 'all' || Boolean(trackingNumber));

  useEffect(() => {
    if (!active) return;
    const connection = createLiveConnection({
      url: socketUrl(),
      subscription:
        scope === 'all'
          ? { scope: 'all' }
          : { trackingNumber: trackingNumber ?? '' },
      onUpdate: (update) => updateRef.current(update),
      onSync: () => syncRef.current?.(),
      onState: setState,
      // A 4401 means the access token expired. Renew it directly; the next
      // handshake then carries the rotated cookie.
      onAuthExpired: refreshSession,
    });
    return () => connection.close();
  }, [active, scope, trackingNumber]);

  return { state: active ? state : 'idle' };
}

function socketUrl(): string {
  const override = process.env.NEXT_PUBLIC_WS_URL;
  if (override) return override;
  const scheme = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  return `${scheme}//${window.location.host}/ws`;
}
