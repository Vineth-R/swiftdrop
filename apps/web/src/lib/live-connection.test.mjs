/**
 * Tests for the live updates connection. They use a fake socket and a fake
 * clock, so they add no dependency and need no server:
 *
 *   node --test "apps/web/src/lib/*.test.mjs"
 */
import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import './api-test-support.mjs';

const { parcelStatuses } = await import('./tokens.ts');
const {
  backoffDelay,
  createLiveConnection,
  HEARTBEAT_INTERVAL_MS,
  SUBSCRIBE_TIMEOUT_MS,
} = await import('./live-connection.ts');

class FakeSocket {
  constructor(url) {
    this.url = url;
    this.sent = [];
    this.closedWith = null;
    this.onopen = null;
    this.onmessage = null;
    this.onclose = null;
    this.onerror = null;
  }
  send(data) {
    this.sent.push(JSON.parse(data));
  }
  close(code) {
    this.closedWith = code;
  }
  open() {
    this.onopen?.();
  }
  receive(message) {
    const data =
      typeof message === 'string' ? message : JSON.stringify(message);
    this.onmessage?.({ data });
  }
  drop(code = 1006) {
    this.onclose?.({ code });
  }
}

const TRACKING = 'SDK7M2Q9XP4';

function setup(subscription = { trackingNumber: TRACKING }, extra = {}) {
  const env = {
    sockets: [],
    timers: [],
    states: [],
    updates: [],
    syncs: 0,
  };
  env.connection = createLiveConnection({
    url: 'ws://test/ws',
    subscription,
    onUpdate: (update) => env.updates.push(update),
    onSync: () => {
      env.syncs += 1;
    },
    onState: (state) => env.states.push(state),
    createSocket: (url) => {
      const socket = new FakeSocket(url);
      env.sockets.push(socket);
      return socket;
    },
    schedule: (callback, ms) => {
      const entry = { callback, ms, cancelled: false, fired: false };
      env.timers.push(entry);
      return entry;
    },
    cancel: (entry) => {
      entry.cancelled = true;
    },
    random: () => 0.5,
    ...extra,
  });
  return env;
}

/** Opens a socket and confirms the subscription, so only the heartbeat runs. */
function confirm(env, index = 0) {
  env.sockets[index].open();
  env.sockets[index].receive({ type: 'subscribed', trackingNumber: TRACKING });
}

function activeTimers(env) {
  return env.timers.filter((timer) => !timer.cancelled && !timer.fired);
}

function activeAt(env, ms) {
  return activeTimers(env).filter((timer) => timer.ms === ms);
}

/** Runs the next reconnect that is waiting and returns its delay. */
function fire(env) {
  const [entry] = activeTimers(env);
  assert.ok(entry, 'a reconnect should be waiting');
  entry.fired = true;
  entry.callback();
  return entry.ms;
}

/** Runs the one timer with this delay, so heartbeat and timeout stay separate. */
function fireDelay(env, ms) {
  const waiting = activeAt(env, ms);
  assert.equal(waiting.length, 1, `one ${ms}ms timer should be waiting`);
  const [entry] = waiting;
  entry.fired = true;
  entry.callback();
  return entry.ms;
}

function update(overrides = {}) {
  return {
    type: 'parcel.status.updated',
    data: {
      trackingNumber: TRACKING,
      previousStatus: 'IN_TRANSIT',
      status: 'OUT_FOR_DELIVERY',
      updatedAt: '2026-11-02T15:40:00Z',
      ...overrides,
    },
  };
}

describe('subscribing', () => {
  it('subscribes to one parcel when the socket opens', () => {
    const env = setup();
    env.sockets[0].open();
    assert.deepEqual(env.sockets[0].sent, [
      { type: 'subscribe', trackingNumber: TRACKING },
    ]);
    assert.deepEqual(env.states, ['connecting']);
  });

  it('subscribes to everything with scope all', () => {
    const env = setup({ scope: 'all' });
    env.sockets[0].open();
    assert.deepEqual(env.sockets[0].sent, [
      { type: 'subscribe', scope: 'all' },
    ]);
  });

  it('is live and asks the screen to refetch once the server confirms', () => {
    const env = setup();
    confirm(env);
    assert.deepEqual(env.states, ['connecting', 'live']);
    assert.equal(env.syncs, 1);
  });
});

describe('messages', () => {
  it('passes updates on and ignores other parcels', () => {
    const env = setup();
    env.sockets[0].open();
    env.sockets[0].receive(update());
    env.sockets[0].receive(update({ trackingNumber: 'SDK00000000' }));
    assert.equal(env.updates.length, 1);
    assert.equal(env.updates[0].trackingNumber, TRACKING);
    assert.equal(env.updates[0].status, 'OUT_FOR_DELIVERY');
  });

  it('passes every parcel on with scope all', () => {
    const env = setup({ scope: 'all' });
    env.sockets[0].open();
    env.sockets[0].receive(update());
    env.sockets[0].receive(update({ trackingNumber: 'SDN4T8W2LC6' }));
    assert.equal(env.updates.length, 2);
  });

  it('refetches on an assignment notice for scope all', () => {
    const env = setup({ scope: 'all' });
    confirm(env);
    assert.equal(env.syncs, 1);
    env.sockets[0].receive({
      type: 'parcel.assigned.updated',
      data: { trackingNumber: TRACKING, updatedAt: '2026-11-02T15:40:00Z' },
    });
    assert.equal(env.syncs, 2);
    assert.deepEqual(env.updates, []);
  });

  it('ignores an assignment notice on a per-parcel subscription', () => {
    const env = setup();
    confirm(env);
    env.sockets[0].receive({
      type: 'parcel.assigned.updated',
      data: { trackingNumber: TRACKING, updatedAt: '2026-11-02T15:40:00Z' },
    });
    assert.equal(env.syncs, 1);
  });

  it('ignores a malformed assignment notice', () => {
    const env = setup({ scope: 'all' });
    confirm(env);
    env.sockets[0].receive({
      type: 'parcel.assigned.updated',
      data: { trackingNumber: TRACKING },
    });
    assert.equal(env.syncs, 1);
  });

  it('ignores messages that are not valid', () => {
    const env = setup();
    env.sockets[0].open();
    env.sockets[0].receive('not json');
    env.sockets[0].receive({ type: 'something.else' });
    env.sockets[0].receive(update({ status: 'LOST' }));
    env.sockets[0].receive({
      type: 'parcel.status.updated',
      data: { trackingNumber: TRACKING },
    });
    assert.deepEqual(env.updates, []);
  });

  it('accepts every status from the shared parcelStatuses list', () => {
    const env = setup();
    confirm(env);
    for (const status of parcelStatuses) {
      env.sockets[0].receive(update({ status }));
    }
    assert.deepEqual(
      env.updates.map((entry) => entry.status),
      [...parcelStatuses],
    );
  });

  it('rejects a status that is not in the shared list', () => {
    const env = setup();
    confirm(env);
    env.sockets[0].receive(update({ status: 'LOST' }));
    assert.deepEqual(env.updates, []);
  });

  it('keeps a valid previous status and turns an invalid one into null', () => {
    const env = setup();
    confirm(env);
    env.sockets[0].receive(update({ previousStatus: 'PICKED_UP' }));
    env.sockets[0].receive(update({ previousStatus: 'LOST' }));
    assert.equal(env.updates[0].previousStatus, 'PICKED_UP');
    assert.equal(env.updates[1].previousStatus, null);
  });

  it('refetches once when the server confirms twice', () => {
    const env = setup();
    confirm(env);
    env.sockets[0].receive({ type: 'subscribed', trackingNumber: TRACKING });
    assert.equal(env.syncs, 1);
  });
});

describe('heartbeat', () => {
  it('sends the documented ping at the interval and reschedules once', () => {
    const env = setup();
    confirm(env);
    assert.equal(activeAt(env, HEARTBEAT_INTERVAL_MS).length, 1);

    fireDelay(env, HEARTBEAT_INTERVAL_MS);
    assert.deepEqual(env.sockets[0].sent.at(-1), { type: 'ping' });
    assert.equal(
      env.sockets[0].sent.filter((message) => message.type === 'ping').length,
      1,
    );
    assert.equal(activeAt(env, HEARTBEAT_INTERVAL_MS).length, 1);

    fireDelay(env, HEARTBEAT_INTERVAL_MS);
    assert.equal(
      env.sockets[0].sent.filter((message) => message.type === 'ping').length,
      2,
    );
    assert.equal(activeAt(env, HEARTBEAT_INTERVAL_MS).length, 1);
  });

  it('does not send a ping after close', () => {
    const env = setup();
    confirm(env);
    env.connection.close();
    assert.equal(activeAt(env, HEARTBEAT_INTERVAL_MS).length, 0);
    assert.equal(
      env.sockets[0].sent.filter((message) => message.type === 'ping').length,
      0,
    );
  });

  it('cancels on a drop and starts one on the new socket', () => {
    const env = setup();
    confirm(env);
    env.sockets[0].drop();
    assert.equal(activeAt(env, HEARTBEAT_INTERVAL_MS).length, 0);
    assert.equal(activeAt(env, SUBSCRIBE_TIMEOUT_MS).length, 0);

    fire(env);
    env.sockets[1].open();
    assert.equal(activeAt(env, HEARTBEAT_INTERVAL_MS).length, 1);
    assert.equal(activeAt(env, SUBSCRIBE_TIMEOUT_MS).length, 1);

    env.sockets[1].receive({ type: 'subscribed', trackingNumber: TRACKING });
    assert.equal(activeAt(env, SUBSCRIBE_TIMEOUT_MS).length, 0);
    fireDelay(env, HEARTBEAT_INTERVAL_MS);
    assert.equal(
      env.sockets[1].sent.filter((message) => message.type === 'ping').length,
      1,
    );
    assert.equal(
      env.sockets[0].sent.filter((message) => message.type === 'ping').length,
      0,
    );
  });

  it('is not started when the socket cannot be created', () => {
    const sockets = [];
    let calls = 0;
    const env = setup(
      { trackingNumber: TRACKING },
      {
        createSocket: (url) => {
          calls += 1;
          if (calls === 1) throw new Error('blocked');
          const socket = new FakeSocket(url);
          sockets.push(socket);
          return socket;
        },
      },
    );

    assert.equal(env.states.at(-1), 'reconnecting');
    assert.equal(activeAt(env, HEARTBEAT_INTERVAL_MS).length, 0);
    assert.equal(activeAt(env, SUBSCRIBE_TIMEOUT_MS).length, 0);
    const [waiting] = activeTimers(env);
    assert.equal(waiting.ms, 1000);

    fire(env);
    assert.equal(calls, 2);
    sockets[0].open();
    assert.deepEqual(sockets[0].sent, [
      { type: 'subscribe', trackingNumber: TRACKING },
    ]);
  });
});

describe('subscription timeout', () => {
  it('reconnects when the server never confirms the subscription', () => {
    const env = setup();
    env.sockets[0].open();
    assert.equal(activeAt(env, SUBSCRIBE_TIMEOUT_MS).length, 1);

    fireDelay(env, SUBSCRIBE_TIMEOUT_MS);
    assert.equal(env.states.at(-1), 'reconnecting');
    assert.equal(env.sockets[0].closedWith, 1000);
    assert.equal(activeAt(env, SUBSCRIBE_TIMEOUT_MS).length, 0);
    assert.equal(activeAt(env, HEARTBEAT_INTERVAL_MS).length, 0);
    assert.equal(fire(env), 1000);
  });

  it('cancels the timeout once the server confirms', () => {
    const env = setup();
    confirm(env);
    assert.equal(activeAt(env, SUBSCRIBE_TIMEOUT_MS).length, 0);
    assert.equal(activeAt(env, HEARTBEAT_INTERVAL_MS).length, 1);
  });
});

describe('reconnecting', () => {
  it('waits longer each time and starts again after a confirmed subscription', () => {
    const env = setup();
    env.sockets[0].open();
    env.sockets[0].drop();
    assert.equal(fire(env), 1000);
    env.sockets[1].drop();
    assert.equal(fire(env), 2000);
    env.sockets[2].open();
    env.sockets[2].receive({ type: 'subscribed', trackingNumber: TRACKING });
    env.sockets[2].drop();
    assert.equal(fire(env), 1000);
    assert.deepEqual(env.states, [
      'connecting',
      'reconnecting',
      'live',
      'reconnecting',
    ]);
  });

  it('asks the screen to refetch again after a reconnect', () => {
    const env = setup();
    confirm(env);
    env.sockets[0].drop();
    fire(env);
    env.sockets[1].open();
    env.sockets[1].receive({ type: 'subscribed', trackingNumber: TRACKING });
    assert.equal(env.syncs, 2);
    assert.equal(env.states.at(-1), 'live');
  });

  it('asks for a new session on 4401, then reconnects', async () => {
    const calls = [];
    const env = setup(
      { trackingNumber: TRACKING },
      {
        onAuthExpired: async () => {
          calls.push('renew');
        },
      },
    );
    env.sockets[0].open();
    env.sockets[0].drop(4401);
    await new Promise((resolve) => setImmediate(resolve));
    assert.deepEqual(calls, ['renew']);
    assert.equal(fire(env), 1000);
  });

  it('reconnects after a failed renewal attempt', async () => {
    const env = setup(
      { trackingNumber: TRACKING },
      {
        onAuthExpired: () => Promise.reject(new Error('no session')),
      },
    );
    env.sockets[0].open();
    env.sockets[0].drop(4401);
    await new Promise((resolve) => setImmediate(resolve));
    assert.equal(fire(env), 1000);
  });

  it('does not reconnect if closed while renewing the session', async () => {
    let resolveAuth;
    const auth = new Promise((resolve) => {
      resolveAuth = resolve;
    });
    const env = setup(
      { trackingNumber: TRACKING },
      { onAuthExpired: () => auth },
    );
    env.sockets[0].open();
    env.sockets[0].drop(4401);
    env.connection.close();

    resolveAuth();
    await new Promise((resolve) => setImmediate(resolve));

    assert.equal(activeTimers(env).length, 0);
    assert.equal(env.sockets.length, 1);
  });
});

describe('refetch failures', () => {
  it('does not let a rejected refetch break the connection', async () => {
    let syncs = 0;
    let unhandled = false;
    const onUnhandled = () => {
      unhandled = true;
    };
    process.on('unhandledRejection', onUnhandled);

    const env = setup(
      { trackingNumber: TRACKING },
      {
        onSync: () => {
          syncs += 1;
          return Promise.reject(new Error('refetch failed'));
        },
      },
    );
    confirm(env);
    await new Promise((resolve) => setImmediate(resolve));
    process.off('unhandledRejection', onUnhandled);

    assert.equal(unhandled, false);
    assert.equal(syncs, 1);
    env.sockets[0].receive(update());
    assert.equal(env.updates.length, 1);
  });

  it('does not let a throwing refetch break the connection', () => {
    const env = setup(
      { trackingNumber: TRACKING },
      {
        onSync: () => {
          throw new Error('refetch failed');
        },
      },
    );
    confirm(env);
    assert.equal(env.states.at(-1), 'live');
    env.sockets[0].receive(update());
    assert.equal(env.updates.length, 1);
  });
});

describe('stopping', () => {
  it('stops for good on close code 4403', () => {
    const env = setup();
    env.sockets[0].open();
    env.sockets[0].drop(4403);
    assert.equal(env.states.at(-1), 'denied');
    assert.equal(activeTimers(env).length, 0);
  });

  it('stops in the limited state on close code 4429 without retrying', () => {
    const env = setup();
    env.sockets[0].open();
    env.sockets[0].drop(4429);
    assert.equal(env.states.at(-1), 'limited');
    assert.equal(env.states.includes('reconnecting'), false);
    assert.equal(activeTimers(env).length, 0);
    assert.equal(env.sockets.length, 1);
  });

  it('stops when the server says FORBIDDEN', () => {
    const env = setup({ scope: 'all' });
    env.sockets[0].open();
    env.sockets[0].receive({
      type: 'error',
      code: 'FORBIDDEN',
      message: 'Not allowed to subscribe to this scope.',
    });
    assert.equal(env.states.at(-1), 'denied');
    assert.equal(env.sockets[0].closedWith, 1000);
    assert.equal(activeTimers(env).length, 0);
  });

  it('does not reconnect after close', () => {
    const env = setup();
    env.sockets[0].open();
    env.connection.close();
    assert.equal(env.sockets[0].closedWith, 1000);
    env.sockets[0].drop();
    assert.equal(activeTimers(env).length, 0);
  });

  it('cancels a waiting reconnect on close', () => {
    const env = setup();
    env.sockets[0].open();
    env.sockets[0].drop();
    const [reconnect] = activeTimers(env);
    assert.equal(reconnect.ms, 1000);

    env.connection.close();
    assert.equal(reconnect.cancelled, true);
    assert.equal(env.sockets.length, 1);
  });
});

describe('backoffDelay', () => {
  it('doubles up to thirty seconds', () => {
    const delays = [0, 1, 2, 3, 4, 5, 6].map((n) => backoffDelay(n, () => 0.5));
    assert.deepEqual(delays, [1000, 2000, 4000, 8000, 16000, 30000, 30000]);
  });

  it('adds up to 20 percent either way', () => {
    assert.equal(
      backoffDelay(0, () => 0),
      800,
    );
    assert.equal(
      backoffDelay(0, () => 1),
      1200,
    );
  });
});
