import type { ParcelStatus } from './tokens';

/**
 * The live updates connection (API v1.1 section 9), without React so it can be
 * tested with a fake socket. Screens use `useLiveUpdates`, which wraps this.
 *
 * What it does:
 * - opens the socket and subscribes to one parcel or to everything (`scope: all`);
 * - passes each status update on to `onUpdate`;
 * - calls `onSync` after every confirmed subscription, first and after every
 *   reconnect, so the screen fetches the current state again (the socket is
 *   never the source of truth);
 * - reconnects after a drop with a growing delay and some randomness;
 * - stops for good when the server says the caller is not allowed (4403 or a
 *   FORBIDDEN message);
 * - on 4401 (token expired) asks for a new session, then reconnects.
 */

export type LiveSubscription = { trackingNumber: string } | { scope: 'all' };

export type StatusUpdate = {
  trackingNumber: string;
  previousStatus: ParcelStatus | null;
  status: ParcelStatus;
  /** An ISO timestamp from the server. */
  updatedAt: string;
};

/**
 * connecting: first attempt. live: subscribed. reconnecting: waiting to retry
 * after a drop. denied: not allowed, no more attempts.
 */
export type LiveState = 'connecting' | 'live' | 'reconnecting' | 'denied';

/** The part of a WebSocket this module uses, so a test can supply a fake. */
export type SocketLike = {
  send(data: string): void;
  close(code?: number): void;
  onopen: (() => void) | null;
  onmessage: ((event: { data: unknown }) => void) | null;
  onclose: ((event: { code: number }) => void) | null;
  onerror: (() => void) | null;
};

export type LiveOptions = {
  url: string;
  subscription: LiveSubscription;
  onUpdate: (update: StatusUpdate) => void;
  /** Fetch the current state again. */
  onSync: () => void;
  onState: (state: LiveState) => void;
  /** Called on close code 4401, before the next attempt. */
  onAuthExpired?: () => Promise<unknown>;
  createSocket?: (url: string) => SocketLike;
  schedule?: (callback: () => void, delayMs: number) => unknown;
  cancel?: (handle: unknown) => void;
  random?: () => number;
};

const STATUSES: readonly string[] = [
  'CREATED',
  'PICKED_UP',
  'IN_TRANSIT',
  'OUT_FOR_DELIVERY',
  'DELIVERED',
  'FAILED',
];

export const BACKOFF_BASE_MS = 1000;
export const BACKOFF_MAX_MS = 30_000;

/** The wait before attempt number `attempt` (0 is the first retry), in milliseconds. */
export function backoffDelay(
  attempt: number,
  random: () => number = Math.random,
): number {
  const base = BACKOFF_BASE_MS * 2 ** attempt;
  const jittered = Math.round(base * (0.8 + random() * 0.4));
  return Math.min(BACKOFF_MAX_MS, jittered);
}

function isStatus(value: unknown): value is ParcelStatus {
  return typeof value === 'string' && STATUSES.includes(value);
}

type Message = { type: string; code?: unknown; data?: unknown };

function parse(data: unknown): Message | null {
  if (typeof data !== 'string') return null;
  try {
    const value: unknown = JSON.parse(data);
    if (typeof value !== 'object' || value === null) return null;
    const message = value as Record<string, unknown>;
    if (typeof message.type !== 'string') return null;
    return { type: message.type, code: message.code, data: message.data };
  } catch {
    return null;
  }
}

function toUpdate(value: unknown): StatusUpdate | null {
  if (typeof value !== 'object' || value === null) return null;
  const { trackingNumber, previousStatus, status, updatedAt } = value as Record<
    string,
    unknown
  >;
  if (typeof trackingNumber !== 'string' || typeof updatedAt !== 'string') {
    return null;
  }
  if (!isStatus(status)) return null;
  return {
    trackingNumber,
    previousStatus: isStatus(previousStatus) ? previousStatus : null,
    status,
    updatedAt,
  };
}

/** A real WebSocket has the same shape, apart from its event types. */
function defaultCreateSocket(url: string): SocketLike {
  return new WebSocket(url) as unknown as SocketLike;
}

export function createLiveConnection(options: LiveOptions): {
  close: () => void;
} {
  const {
    url,
    subscription,
    onUpdate,
    onSync,
    onState,
    onAuthExpired,
    createSocket = defaultCreateSocket,
    schedule = (callback, delayMs) => setTimeout(callback, delayMs),
    cancel = (handle) => clearTimeout(handle as ReturnType<typeof setTimeout>),
    random = Math.random,
  } = options;

  let socket: SocketLike | null = null;
  let timer: unknown = null;
  let attempt = 0;
  let closed = false;
  let state: LiveState | null = null;

  function setState(next: LiveState) {
    if (state === next) return;
    state = next;
    onState(next);
  }

  function matches(update: StatusUpdate): boolean {
    return (
      'scope' in subscription ||
      update.trackingNumber === subscription.trackingNumber
    );
  }

  function detach() {
    const current = socket;
    socket = null;
    if (!current) return;
    current.onopen = null;
    current.onmessage = null;
    current.onclose = null;
    current.onerror = null;
    current.close(1000);
  }

  function stop() {
    closed = true;
    if (timer !== null) {
      cancel(timer);
      timer = null;
    }
    detach();
  }

  function deny() {
    stop();
    setState('denied');
  }

  function scheduleReconnect() {
    if (closed) return;
    timer = schedule(connect, backoffDelay(attempt, random));
    attempt += 1;
  }

  function handleMessage(data: unknown) {
    const message = parse(data);
    if (!message) return;

    if (message.type === 'subscribed') {
      attempt = 0;
      setState('live');
      onSync();
      return;
    }
    if (message.type === 'parcel.status.updated') {
      const update = toUpdate(message.data);
      if (update && matches(update)) onUpdate(update);
      return;
    }
    if (message.type === 'error' && message.code === 'FORBIDDEN') deny();
  }

  function handleClose(code: number) {
    if (code === 4403) {
      deny();
      return;
    }
    setState('reconnecting');
    if (code === 4401 && onAuthExpired) {
      Promise.resolve()
        .then(onAuthExpired)
        .catch(() => undefined)
        .then(scheduleReconnect);
      return;
    }
    scheduleReconnect();
  }

  function connect() {
    timer = null;
    if (closed) return;
    const current = createSocket(url);
    socket = current;
    current.onopen = () => {
      if (socket !== current) return;
      current.send(JSON.stringify({ type: 'subscribe', ...subscription }));
    };
    current.onmessage = (event) => {
      if (socket === current) handleMessage(event.data);
    };
    current.onclose = (event) => {
      if (socket === current) handleClose(event.code);
    };
    // A close event always follows an error, so there is nothing to do here.
    current.onerror = () => undefined;
  }

  setState('connecting');
  connect();

  return { close: stop };
}
