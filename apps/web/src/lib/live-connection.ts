import { parcelStatuses, type ParcelStatus } from './tokens';

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
 * - sends the documented client keepalive (`{ type: "ping" }`) at a fixed
 *   interval so the server's idle timeout does not close an active connection;
 * - reconnects after a drop with a growing delay and some randomness;
 * - stops for good when the server says the caller is not allowed (4403 or a
 *   FORBIDDEN message), or when a limit is exceeded (4429);
 * - on 4401 (token expired) asks for a new session, then reconnects.
 *
 * Heartbeat note: API v1.1 section 9 defines only the client message
 * `{ "type": "ping" }`. It does not define a server JSON ping frame or a client
 * `pong` frame, so this module sends the documented client ping and does not
 * invent a pong message. Protocol-level WebSocket pings are answered by the
 * browser automatically.
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
 * after a drop. denied: not allowed, no more attempts. limited: the server
 * closed the connection for exceeding a subscription or message limit, so no
 * more attempts are made automatically.
 */
export type LiveState =
  'connecting' | 'live' | 'reconnecting' | 'denied' | 'limited';

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
  onSync: () => void | Promise<unknown>;
  onState: (state: LiveState) => void;
  /** Called on close code 4401, before the next attempt. */
  onAuthExpired?: () => Promise<unknown>;
  createSocket?: (url: string) => SocketLike;
  schedule?: (callback: () => void, delayMs: number) => unknown;
  cancel?: (handle: unknown) => void;
  random?: () => number;
};

const STATUS_SET: ReadonlySet<string> = new Set(parcelStatuses);

export const BACKOFF_BASE_MS = 1000;
export const BACKOFF_MAX_MS = 30_000;

/**
 * The documented 60-second idle timeout leaves room for one missed ping, so a
 * ping every 25 seconds keeps an otherwise idle connection alive.
 */
export const HEARTBEAT_INTERVAL_MS = 25_000;

/** How long to wait for the server's `subscribed` confirmation after opening. */
export const SUBSCRIBE_TIMEOUT_MS = 10_000;

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
  return typeof value === 'string' && STATUS_SET.has(value);
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

/** An assignment notice carries only the tracking number and event time. */
type AssignmentNotice = { trackingNumber: string; updatedAt: string };

function toAssignment(value: unknown): AssignmentNotice | null {
  if (typeof value !== 'object' || value === null) return null;
  const { trackingNumber, updatedAt } = value as Record<string, unknown>;
  if (typeof trackingNumber !== 'string' || typeof updatedAt !== 'string') {
    return null;
  }
  return { trackingNumber, updatedAt };
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
  let reconnectTimer: unknown = null;
  let heartbeatTimer: unknown = null;
  let subscribeTimer: unknown = null;
  let attempt = 0;
  let closed = false;
  let confirmed = false;
  let state: LiveState | null = null;

  function setState(next: LiveState) {
    if (state === next) return;
    state = next;
    onState(next);
  }

  function matchesTracking(trackingNumber: string): boolean {
    return (
      'scope' in subscription ||
      trackingNumber === subscription.trackingNumber
    );
  }

  function matches(update: StatusUpdate): boolean {
    return matchesTracking(update.trackingNumber);
  }

  function clearReconnect() {
    if (reconnectTimer === null) return;
    cancel(reconnectTimer);
    reconnectTimer = null;
  }

  function clearHeartbeat() {
    if (heartbeatTimer === null) return;
    cancel(heartbeatTimer);
    heartbeatTimer = null;
  }

  function clearSubscribeTimeout() {
    if (subscribeTimer === null) return;
    cancel(subscribeTimer);
    subscribeTimer = null;
  }

  function detach() {
    clearHeartbeat();
    clearSubscribeTimeout();
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
    clearReconnect();
    clearHeartbeat();
    clearSubscribeTimeout();
    detach();
  }

  function terminate(next: 'denied' | 'limited') {
    stop();
    setState(next);
  }

  function safeSync() {
    try {
      void Promise.resolve(onSync()).catch(() => undefined);
    } catch {
      // A screen's refetch must not break the socket.
    }
  }

  function startHeartbeat(current: SocketLike) {
    if (heartbeatTimer !== null) return;
    heartbeatTimer = schedule(() => {
      heartbeatTimer = null;
      if (closed || socket !== current) return;
      try {
        current.send(JSON.stringify({ type: 'ping' }));
      } catch {
        return;
      }
      startHeartbeat(current);
    }, HEARTBEAT_INTERVAL_MS);
  }

  function startSubscribeTimeout(current: SocketLike) {
    clearSubscribeTimeout();
    subscribeTimer = schedule(() => {
      subscribeTimer = null;
      if (closed || confirmed || socket !== current) return;
      setState('reconnecting');
      detach();
      scheduleReconnect();
    }, SUBSCRIBE_TIMEOUT_MS);
  }

  function scheduleReconnect() {
    if (closed) return;
    clearReconnect();
    reconnectTimer = schedule(connect, backoffDelay(attempt, random));
    attempt += 1;
  }

  function handleMessage(data: unknown) {
    const message = parse(data);
    if (!message) return;

    if (message.type === 'subscribed') {
      // A duplicate confirmation must not refetch the screen again.
      if (confirmed) return;
      confirmed = true;
      attempt = 0;
      clearSubscribeTimeout();
      setState('live');
      safeSync();
      return;
    }
    if (message.type === 'parcel.status.updated') {
      const update = toUpdate(message.data);
      if (update && matches(update)) onUpdate(update);
      return;
    }
    if (message.type === 'parcel.assigned.updated') {
      // Dispatch notices carry no assignment data on purpose. Only a scope:all
      // subscription may receive one; refetch the authoritative REST state.
      if (!('scope' in subscription)) return;
      if (toAssignment(message.data)) safeSync();
      return;
    }
    if (message.type === 'error' && message.code === 'FORBIDDEN')
      terminate('denied');
  }

  function handleClose(code: number) {
    if (closed) return;
    if (code === 4403) {
      terminate('denied');
      return;
    }
    if (code === 4429) {
      terminate('limited');
      return;
    }
    detach();
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
    reconnectTimer = null;
    if (closed) return;
    clearHeartbeat();
    clearSubscribeTimeout();
    confirmed = false;

    let current: SocketLike;
    try {
      current = createSocket(url);
    } catch {
      setState('reconnecting');
      scheduleReconnect();
      return;
    }

    socket = current;
    current.onopen = () => {
      if (socket !== current) return;
      try {
        current.send(JSON.stringify({ type: 'subscribe', ...subscription }));
      } catch {
        handleClose(1006);
        return;
      }
      startHeartbeat(current);
      if (!confirmed) startSubscribeTimeout(current);
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
