/**
 * Tests for the live updates connection. They use a fake socket and a fake
 * clock, so they add no dependency and need no server:
 *
 *   node --test "apps/web/src/lib/*.test.mjs"
 */
import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { backoffDelay, createLiveConnection } from './live-connection.ts';

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

/** Runs the next reconnect that is waiting and returns its delay. */
function fire(env) {
  const entry = env.timers.find((timer) => !timer.cancelled && !timer.fired);
  assert.ok(entry, 'a reconnect should be waiting');
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
    env.sockets[0].open();
    env.sockets[0].receive({ type: 'subscribed', trackingNumber: TRACKING });
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
    env.sockets[0].open();
    env.sockets[0].receive({ type: 'subscribed', trackingNumber: TRACKING });
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
});

describe('stopping', () => {
  it('stops for good on close code 4403', () => {
    const env = setup();
    env.sockets[0].open();
    env.sockets[0].drop(4403);
    assert.equal(env.states.at(-1), 'denied');
    assert.equal(env.timers.length, 0);
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
    assert.equal(env.timers.length, 0);
  });

  it('does not reconnect after close', () => {
    const env = setup();
    env.sockets[0].open();
    env.connection.close();
    assert.equal(env.sockets[0].closedWith, 1000);
    env.sockets[0].drop();
    assert.equal(env.timers.length, 0);
  });

  it('cancels a waiting reconnect on close', () => {
    const env = setup();
    env.sockets[0].open();
    env.sockets[0].drop();
    env.connection.close();
    assert.equal(env.timers[0].cancelled, true);
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
