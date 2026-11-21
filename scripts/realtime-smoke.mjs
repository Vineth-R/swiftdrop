#!/usr/bin/env node
// Realtime smoke test (M10.3).
// Injects one synthetic outbox row and follows it through
//   parcel relay -> RabbitMQ -> realtime consumer -> WebSocket client.
// It does not run the courier status-update transaction; that is covered elsewhere.
//
//   node scripts/realtime-smoke.mjs
//
// Needs Node 22+ (global WebSocket), Docker Compose, and a running stack whose
// realtime WebSocket port is reachable at WS_URL.
// Env:
//   WS_URL       default ws://localhost:8085/ws
//   TRACKING     default: a random SDK number, so no real parcel is touched
//   TIMEOUT_MS   default 15000 (positive integer, at most 2147483647)
// The postgres user and database are read from the running postgres container,
// not from the host shell, so a Compose .env override is honoured.
import { randomInt, randomUUID } from 'node:crypto';
import { spawnSync } from 'node:child_process';

const TRACKING_PATTERN = /^SD[KN][A-Z0-9]{8}$/;
const ALPHABET = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789';
const PUBLIC_DATA_KEYS = [
  'previousStatus',
  'status',
  'trackingNumber',
  'updatedAt',
];
const PRIVATE_NOTE = 'PRIVATE_NOTE_DO_NOT_FORWARD';
const MAX_TIMEOUT_MS = 2_147_483_647;

let socket;
let timer;
let finished = false;

function randomTracking() {
  let tail = '';
  for (let i = 0; i < 8; i += 1) tail += ALPHABET[randomInt(ALPHABET.length)];
  return `SDK${tail}`;
}

function finish(code, message) {
  if (finished) return;
  finished = true;
  clearTimeout(timer);
  if (
    socket &&
    (socket.readyState === WebSocket.CONNECTING ||
      socket.readyState === WebSocket.OPEN)
  ) {
    try {
      socket.close();
    } catch {
      // The process is exiting; a failed close must not mask the result.
    }
  }
  if (message) {
    if (code === 0) console.log(message);
    else console.error(`FAIL: ${message}`);
  }
  process.exit(code);
}

function fail(message) {
  finish(1, message);
}

const url = process.env.WS_URL ?? 'ws://localhost:8085/ws';
const tracking = process.env.TRACKING ?? randomTracking();
const timeoutMs = Number(process.env.TIMEOUT_MS ?? 15000);

if (!TRACKING_PATTERN.test(tracking)) {
  fail(`TRACKING must match ${TRACKING_PATTERN}`);
}
if (
  !Number.isInteger(timeoutMs) ||
  timeoutMs <= 0 ||
  timeoutMs > MAX_TIMEOUT_MS
) {
  fail(
    `TIMEOUT_MS must be a positive integer no greater than ${MAX_TIMEOUT_MS}`,
  );
}

const eventId = randomUUID();

function dockerCompose(args, options = {}) {
  const run = spawnSync('docker', ['compose', ...args], {
    encoding: 'utf8',
    timeout: 10000,
    ...options,
  });
  if (run.error) fail(`could not run docker compose\n${run.error.message}`);
  return run;
}

// Read a setting from the postgres container itself. Compose injects the
// configured values there, so this works even when .env is not exported.
function postgresSetting(name) {
  const script = `printf %s "$POSTGRES_${name}"`;
  const run = dockerCompose(['exec', '-T', 'postgres', 'sh', '-c', script]);
  if (run.status !== 0) {
    fail(
      `could not read POSTGRES_${name} from the postgres container\n${run.stderr || run.stdout}`,
    );
  }
  const value = run.stdout.trim();
  if (!value) fail(`the postgres container did not set POSTGRES_${name}`);
  return value;
}

const dbUser = postgresSetting('USER');
const dbName = postgresSetting('DB');

function psql(sql, variables = {}) {
  const args = [
    'exec',
    '-T',
    'postgres',
    'psql',
    '-X',
    '-q',
    '-At',
    '-U',
    dbUser,
    '-d',
    dbName,
    '-v',
    'ON_ERROR_STOP=1',
  ];
  for (const [key, value] of Object.entries(variables)) {
    args.push('-v', `${key}=${value}`);
  }
  return dockerCompose(args, { input: sql });
}

// Returns created_at as UTC text. The relay uses it as the event's occurred_at.
function insertOutboxRow() {
  const payload = JSON.stringify({
    parcel_id: randomUUID(), // required by the event contract; no owner fields, so no email is attempted
    tracking_number: tracking,
    previous_status: 'CREATED',
    status: 'PICKED_UP',
    note: PRIVATE_NOTE, // contract-allowed, but never part of the public projection
  });
  const run = psql(
    `INSERT INTO parcel.outbox_events (id, event_type, routing_key, payload)
VALUES (:'event_id'::uuid, 'parcel.status_changed', 'parcel.status.changed', :'payload'::jsonb)
RETURNING to_char(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"');`,
    { event_id: eventId, payload },
  );
  if (run.status !== 0)
    fail(`could not insert the outbox row\n${run.stderr || run.stdout}`);
  const createdAt = run.stdout.trim();
  if (!createdAt) fail('the insert did not return the outbox created_at');
  return createdAt;
}

function cleanUp() {
  // The row is PUBLISHED, so deleting this exact row cannot affect broker
  // messages other consumers already received.
  const run = psql(
    `DELETE FROM parcel.outbox_events WHERE id = :'event_id'::uuid RETURNING id;`,
    { event_id: eventId },
  );
  if (run.status !== 0) {
    console.error(
      `warning: could not delete the synthetic outbox row\n${run.stderr || run.stdout}`,
    );
  } else if (run.stdout.trim() !== eventId) {
    console.error('warning: the synthetic outbox row was already gone');
  }
}

// The relay marks the row PUBLISHED after the broker confirms the publish. The
// WebSocket event can arrive first, so wait for the expected state before
// deleting the row.
function verifyPublishedAndCleanUp(attempt = 0) {
  const select = psql(
    `SELECT status FROM parcel.outbox_events WHERE id = :'event_id'::uuid;`,
    { event_id: eventId },
  );
  if (select.status !== 0) {
    fail(
      `could not read the outbox row status\n${select.stderr || select.stdout}`,
    );
  }
  const status = select.stdout.trim();
  if (status === 'PUBLISHED') {
    cleanUp();
    finish(
      0,
      `PASS: update received with only the four public fields; outbox row PUBLISHED (tracking ${tracking})`,
    );
    return;
  }
  if (!status) fail('the outbox row disappeared before it was published');
  if (attempt >= 20) fail(`outbox row status is ${status}, want PUBLISHED`);
  setTimeout(() => verifyPublishedAndCleanUp(attempt + 1), 100);
}

let createdAt = '';

function onStatusUpdate(message) {
  const data = message.data ?? {};
  const keys = Object.keys(data).sort();
  if (JSON.stringify(keys) !== JSON.stringify(PUBLIC_DATA_KEYS)) {
    fail(`unexpected fields: ${keys.join(', ')}`);
  }
  if (JSON.stringify(message).includes(PRIVATE_NOTE)) {
    fail('private note leaked to the socket');
  }
  if (data.trackingNumber !== tracking) {
    fail(`wrong trackingNumber: ${data.trackingNumber}`);
  }
  if (data.previousStatus !== 'CREATED') {
    fail(`wrong previousStatus: ${data.previousStatus}`);
  }
  if (data.status !== 'PICKED_UP') {
    fail(`wrong status: ${data.status}`);
  }
  const updatedAt = Date.parse(data.updatedAt);
  const outboxAt = Date.parse(createdAt);
  if (!Number.isFinite(updatedAt) || !Number.isFinite(outboxAt)) {
    fail(
      `could not parse timestamps: updatedAt=${data.updatedAt} outbox=${createdAt}`,
    );
  }
  const drift = Math.abs(updatedAt - outboxAt);
  if (drift > 2000) {
    fail(
      `updatedAt ${data.updatedAt} does not match the outbox time ${createdAt}`,
    );
  }

  clearTimeout(timer); // the update arrived; the remaining wait is for PUBLISHED
  verifyPublishedAndCleanUp();
}

console.log(`tracking: ${tracking}`);

socket = new WebSocket(url);
timer = setTimeout(() => fail(`no update within ${timeoutMs} ms`), timeoutMs);

socket.onerror = () =>
  fail(`cannot connect to ${url}. Is the realtime port published?`);
socket.onclose = () => {
  if (!finished)
    fail('the server closed the connection before an update arrived');
};
socket.onopen = () => {
  socket.send(JSON.stringify({ type: 'subscribe', trackingNumber: tracking }));
};
socket.onmessage = (event) => {
  let message;
  try {
    message = JSON.parse(String(event.data));
  } catch (error) {
    fail(`invalid JSON from the server: ${error.message}`);
    return;
  }
  console.log('<-', JSON.stringify(message));
  if (message.type === 'subscribed' && !createdAt) {
    createdAt = insertOutboxRow(); // no replay, so insert only after the subscription is confirmed
    return;
  }
  if (message.type === 'error') {
    fail(`server error: ${message.code ?? JSON.stringify(message)}`);
    return;
  }
  if (message.type === 'parcel.status.updated') onStatusUpdate(message);
};
