#!/usr/bin/env node
// Realtime verification (M10): checks the WebSocket service against API v1.1
// section 9 on a running stack and prints a results table.
//
//   node scripts/realtime-verify.mjs
//
// It checks who may follow what (anonymous, customer, dispatcher, a bad
// token), the subscription limit, the message-rate limit, the size limit, the
// browser origin check, and that a status update carries only public fields.
// The idle timeout and the token-expiry close take a minute or more, so they
// are not run here; the Go tests cover them (see the notes printed at the end).
//
// Needs Node 22+ (global WebSocket), a running stack, and these variables set
// in the same terminal (never in a file):
//   REAL_STACK_DISPATCHER_EMAIL / REAL_STACK_DISPATCHER_PASSWORD
//   REAL_STACK_CUSTOMER_EMAIL   / REAL_STACK_CUSTOMER_PASSWORD
// Optional:
//   WS_URL        default ws://localhost:8080/ws (through the gateway)
//   API_URL       default http://localhost:8080 (where /api/v1/auth/login is)
//   GOOD_ORIGIN   default http://localhost:8080 (an origin the service allows)
//   BAD_ORIGIN    default http://evil.example
//   SKIP_PRIVACY  set to 1 to skip the privacy check, which runs
//                 scripts/realtime-smoke.mjs (it needs Docker Compose)
// Exit code 0 when every check passed, 1 otherwise.
import { randomBytes, randomInt } from 'node:crypto';
import { spawnSync } from 'node:child_process';
import http from 'node:http';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const WS_URL = process.env.WS_URL || 'ws://localhost:8080/ws';
const API_URL = (process.env.API_URL || 'http://localhost:8080').replace(
  /\/$/,
  '',
);
const GOOD_ORIGIN = process.env.GOOD_ORIGIN || 'http://localhost:8080';
const BAD_ORIGIN = process.env.BAD_ORIGIN || 'http://evil.example';
const ALPHABET = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789';
const STEP_MS = 8000;

const results = [];

function randomTracking() {
  let tail = '';
  for (let i = 0; i < 8; i += 1) tail += ALPHABET[randomInt(ALPHABET.length)];
  return `SDK${tail}`;
}

function record(area, check, expected, actual, pass) {
  results.push({ area, check, expected, actual, pass });
  console.log(`${pass ? 'PASS' : 'FAIL'}  ${area}: ${check} (${actual})`);
}

async function login(emailVar, passwordVar) {
  const email = process.env[emailVar];
  const password = process.env[passwordVar];
  if (!email || !password) return null;
  for (let attempt = 0; attempt < 6; attempt += 1) {
    const response = await fetch(`${API_URL}/api/v1/auth/login`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-Token-Mode': 'body',
      },
      body: JSON.stringify({ email, password }),
    });
    if (response.status === 429) {
      await new Promise((resolve) => setTimeout(resolve, 5000));
      continue;
    }
    if (!response.ok)
      throw new Error(`login for ${email} returned ${response.status}`);
    const body = await response.json();
    const token = body?.data?.accessToken;
    if (!token) throw new Error('login returned no access token');
    return token;
  }
  throw new Error(`login for ${email} stayed rate limited`);
}

/**
 * Opens a socket and keeps what happens: messages, and how it closed.
 * wait(predicate) resolves when the predicate holds, or after STEP_MS.
 */
function connect(token) {
  const url = token ? `${WS_URL}?token=${encodeURIComponent(token)}` : WS_URL;
  const socket = new WebSocket(url);
  const state = { messages: [], closed: null, opened: false };
  socket.addEventListener('open', () => {
    state.opened = true;
  });
  socket.addEventListener('message', (event) => {
    try {
      state.messages.push(JSON.parse(String(event.data)));
    } catch {
      state.messages.push({ type: 'unparseable' });
    }
  });
  socket.addEventListener('close', (event) => {
    state.closed = { code: event.code, reason: event.reason };
  });
  socket.addEventListener('error', () => {
    // A refused handshake also ends in a close event; nothing else to do.
  });

  const send = (value) => socket.send(JSON.stringify(value));
  const wait = async (predicate, ms = STEP_MS) => {
    const deadline = Date.now() + ms;
    while (Date.now() < deadline) {
      if (predicate(state)) return true;
      await new Promise((resolve) => setTimeout(resolve, 50));
    }
    return predicate(state);
  };
  const close = () => {
    try {
      socket.close();
    } catch {
      // Already closed.
    }
  };
  return { socket, state, send, wait, close };
}

const isOpen = (state) => state.opened;
const hasType = (type) => (state) =>
  state.messages.some((message) => message.type === type);
const wasClosed = (state) => state.closed !== null;

/** A raw upgrade request, so the Origin header can be chosen. */
function upgradeStatus(origin) {
  return new Promise((resolve) => {
    const target = new URL(WS_URL.replace(/^ws/, 'http'));
    const request = http.request({
      host: target.hostname,
      port: target.port || (target.protocol === 'https:' ? 443 : 80),
      path: target.pathname,
      headers: {
        Connection: 'Upgrade',
        Upgrade: 'websocket',
        'Sec-WebSocket-Version': '13',
        'Sec-WebSocket-Key': randomBytes(16).toString('base64'),
        Origin: origin,
      },
    });
    const done = (value) => {
      request.destroy();
      resolve(value);
    };
    request.on('upgrade', (response, socket) => {
      socket.destroy();
      resolve(response.statusCode);
    });
    request.on('response', (response) => done(response.statusCode));
    request.on('error', () => done('connection error'));
    request.setTimeout(STEP_MS, () => done('timeout'));
    request.end();
  });
}

async function main() {
  console.log(`Realtime verification against ${WS_URL}\n`);

  const dispatcherToken = await login(
    'REAL_STACK_DISPATCHER_EMAIL',
    'REAL_STACK_DISPATCHER_PASSWORD',
  );
  const customerToken = await login(
    'REAL_STACK_CUSTOMER_EMAIL',
    'REAL_STACK_CUSTOMER_PASSWORD',
  );

  // ---- Authorization -------------------------------------------------------
  {
    const c = connect(null);
    await c.wait(isOpen);
    c.send({ type: 'subscribe', trackingNumber: randomTracking() });
    const got = await c.wait(hasType('subscribed'));
    record(
      'Authorization',
      'anonymous visitor follows one parcel',
      'subscribed',
      got ? 'subscribed' : 'no confirmation',
      got,
    );
    c.close();
  }
  {
    const c = connect(null);
    await c.wait(isOpen);
    c.send({ type: 'subscribe', scope: 'all' });
    await c.wait(wasClosed);
    const code = c.state.closed?.code;
    record(
      'Authorization',
      'anonymous cannot follow everything',
      'close 4401',
      `close ${code ?? 'none'}`,
      code === 4401,
    );
    c.close();
  }
  {
    const c = connect('not-a-real-token');
    await c.wait(wasClosed);
    const code = c.state.closed?.code;
    record(
      'Authorization',
      'invalid token is refused',
      'close 4401',
      `close ${code ?? 'none'}`,
      code === 4401,
    );
    c.close();
  }
  if (customerToken) {
    const c = connect(customerToken);
    await c.wait(isOpen);
    c.send({ type: 'subscribe', scope: 'all' });
    await c.wait(wasClosed);
    const code = c.state.closed?.code;
    const sawError = c.state.messages.some(
      (message) => message.type === 'error' && message.code === 'FORBIDDEN',
    );
    record(
      'Authorization',
      'customer cannot follow everything',
      'FORBIDDEN, close 4403',
      `${sawError ? 'FORBIDDEN' : 'no error message'}, close ${code ?? 'none'}`,
      code === 4403 && sawError,
    );
    c.close();
  } else {
    record(
      'Authorization',
      'customer cannot follow everything',
      'close 4403',
      'REAL_STACK_CUSTOMER_* not set',
      false,
    );
  }
  if (dispatcherToken) {
    const c = connect(dispatcherToken);
    await c.wait(isOpen);
    c.send({ type: 'subscribe', scope: 'all' });
    const got = await c.wait((state) =>
      state.messages.some(
        (message) => message.type === 'subscribed' && message.scope === 'all',
      ),
    );
    record(
      'Authorization',
      'dispatcher follows everything',
      'subscribed (scope all)',
      got ? 'subscribed (scope all)' : 'no confirmation',
      got,
    );
    c.close();
  } else {
    record(
      'Authorization',
      'dispatcher follows everything',
      'subscribed (scope all)',
      'REAL_STACK_DISPATCHER_* not set',
      false,
    );
  }

  // ---- Limits --------------------------------------------------------------
  {
    const c = connect(null);
    await c.wait(isOpen);
    for (let i = 0; i < 20; i += 1) {
      c.send({ type: 'subscribe', trackingNumber: randomTracking() });
    }
    const twenty = await c.wait(
      (state) =>
        state.messages.filter((message) => message.type === 'subscribed')
          .length === 20,
    );
    record(
      'Limits',
      '20 subscriptions are allowed',
      '20 subscribed',
      twenty ? '20 subscribed' : 'fewer than 20 confirmed',
      twenty,
    );
    c.send({ type: 'subscribe', trackingNumber: randomTracking() });
    await c.wait(wasClosed);
    const code = c.state.closed?.code;
    record(
      'Limits',
      'the 21st subscription is refused',
      'close 4429',
      `close ${code ?? 'none'}`,
      code === 4429,
    );
    c.close();
  }
  {
    // Unsubscribing frees a slot.
    const c = connect(null);
    await c.wait(isOpen);
    const first = randomTracking();
    c.send({ type: 'subscribe', trackingNumber: first });
    for (let i = 0; i < 19; i += 1) {
      c.send({ type: 'subscribe', trackingNumber: randomTracking() });
    }
    await c.wait(
      (state) =>
        state.messages.filter((message) => message.type === 'subscribed')
          .length === 20,
    );
    c.send({ type: 'unsubscribe', trackingNumber: first });
    c.send({ type: 'subscribe', trackingNumber: randomTracking() });
    const got = await c.wait(
      (state) =>
        state.messages.filter((message) => message.type === 'subscribed')
          .length === 21,
    );
    record(
      'Limits',
      'unsubscribing frees a slot',
      'a new subscription is accepted',
      got ? 'accepted' : 'not accepted',
      got && c.state.closed === null,
    );
    c.close();
  }
  {
    const c = connect(null);
    await c.wait(isOpen);
    for (let i = 0; i < 70; i += 1) c.send({ type: 'ping' });
    await c.wait(wasClosed);
    const code = c.state.closed?.code;
    record(
      'Limits',
      'more than 60 messages in 10 seconds are refused',
      'close 4429',
      `close ${code ?? 'none'}`,
      code === 4429,
    );
    c.close();
  }
  {
    const c = connect(null);
    await c.wait(isOpen);
    c.socket.send(JSON.stringify({ type: 'ping', pad: 'x'.repeat(2000) }));
    await c.wait(wasClosed);
    const code = c.state.closed?.code;
    record(
      'Limits',
      'a message over 1024 bytes is refused',
      'close 1009',
      `close ${code ?? 'none'}`,
      code === 1009,
    );
    c.close();
  }

  // ---- Messages ------------------------------------------------------------
  {
    const c = connect(null);
    await c.wait(isOpen);
    c.socket.send('this is not json');
    c.send({ type: 'subscribe', trackingNumber: 'not-a-tracking-number' });
    const refused = await c.wait(
      (state) =>
        state.messages.filter(
          (message) =>
            message.type === 'error' && message.code === 'INVALID_MESSAGE',
        ).length >= 2,
    );
    c.send({ type: 'subscribe', trackingNumber: randomTracking() });
    const stillWorks = await c.wait(hasType('subscribed'));
    record(
      'Messages',
      'bad messages get INVALID_MESSAGE and the connection stays',
      '2 errors, then a working subscribe',
      `${refused ? '2 errors' : 'missing error'}, ${stillWorks ? 'subscribe ok' : 'subscribe failed'}`,
      refused && stillWorks && c.state.closed === null,
    );
    c.close();
  }

  // ---- Browser origin ------------------------------------------------------
  {
    const bad = await upgradeStatus(BAD_ORIGIN);
    record(
      'Origin',
      `origin ${BAD_ORIGIN} is refused`,
      'HTTP 403',
      `HTTP ${bad}`,
      bad === 403,
    );
    const good = await upgradeStatus(GOOD_ORIGIN);
    record(
      'Origin',
      `origin ${GOOD_ORIGIN} is accepted`,
      'HTTP 101',
      `HTTP ${good}`,
      good === 101,
    );
  }

  // ---- Privacy -------------------------------------------------------------
  if (process.env.SKIP_PRIVACY === '1') {
    console.log('SKIP  Privacy: SKIP_PRIVACY=1');
  } else {
    const here = dirname(fileURLToPath(import.meta.url));
    const smoke = spawnSync(
      process.execPath,
      [join(here, 'realtime-smoke.mjs')],
      { env: { ...process.env, WS_URL }, encoding: 'utf8', timeout: 60_000 },
    );
    const lastLine = `${smoke.stdout || ''}${smoke.stderr || ''}`
      .trim()
      .split(/\r?\n/)
      .pop();
    record(
      'Privacy',
      'an update with private fields in the event carries only the four public ones',
      'realtime-smoke passes',
      smoke.status === 0 ? 'passed' : `failed: ${lastLine}`,
      smoke.status === 0,
    );
  }

  // ---- Summary -------------------------------------------------------------
  console.log('\n| Area | Check | Expected | Actual | Result |');
  console.log('|---|---|---|---|---|');
  for (const row of results) {
    console.log(
      `| ${row.area} | ${row.check} | ${row.expected} | ${row.actual} | ${row.pass ? 'PASS' : 'FAIL'} |`,
    );
  }
  console.log(
    '\nNot run here (take 60 seconds or more); covered by Go tests in apps/services/realtime/internal/ws/handler_test.go:',
  );
  console.log(
    '- idle timeout drops a silent client: TestSilentClientIsDroppedByTheIdleTimeout',
  );
  console.log('- token expiry closes with 4401: TestTokenEndsTheConnection');
  console.log(
    '- server shutdown closes with 1001: TestShutdownClosesConnections',
  );

  const failed = results.filter((row) => !row.pass).length;
  console.log(
    `\n${results.length - failed} of ${results.length} checks passed.`,
  );
  process.exit(failed === 0 ? 0 : 1);
}

main().catch((error) => {
  console.error(`FAIL: ${error instanceof Error ? error.message : error}`);
  process.exit(1);
});
