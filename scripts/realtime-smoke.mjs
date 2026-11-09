#!/usr/bin/env node
// Realtime smoke test (M10.3).
// Injects one synthetic outbox row and follows it through
//   parcel relay -> RabbitMQ -> realtime consumer -> WebSocket client.
// It does not run the courier status-update transaction; that is covered elsewhere.
//
//   node scripts/realtime-smoke.mjs
//
// Needs Node 22+ (global WebSocket) and a running stack with the realtime port published.
// Env:
//   WS_URL       default ws://localhost:8085/ws
//   TRACKING     default: a random SDK number, so no real parcel is touched
//   TIMEOUT_MS   default 15000
//   POSTGRES_USER, POSTGRES_DB   default swiftdrop (same variables Compose uses)
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

function randomTracking() {
  let tail = '';
  for (let i = 0; i < 8; i += 1) tail += ALPHABET[randomInt(ALPHABET.length)];
  return `SDK${tail}`;
}

function fail(message) {
  console.error(`FAIL: ${message}`);
  process.exit(1);
}

const url = process.env.WS_URL ?? 'ws://localhost:8085/ws';
const tracking = process.env.TRACKING ?? randomTracking();
const timeoutMs = Number(process.env.TIMEOUT_MS ?? 15000);
const dbUser = process.env.POSTGRES_USER || 'swiftdrop';
const dbName = process.env.POSTGRES_DB || 'swiftdrop';

if (!TRACKING_PATTERN.test(tracking))
  fail(`TRACKING must match ${TRACKING_PATTERN}`);
if (!Number.isFinite(timeoutMs) || timeoutMs <= 0)
  fail('TIMEOUT_MS must be a positive number');
if (!/^[A-Za-z0-9_]+$/.test(dbUser) || !/^[A-Za-z0-9_]+$/.test(dbName)) {
  fail(
    'POSTGRES_USER and POSTGRES_DB may contain only letters, digits and underscores',
  );
}

const eventId = randomUUID();

function psql(sql) {
  const run = spawnSync(
    'docker',
    [
      'compose',
      'exec',
      '-T',
      'postgres',
      'psql',
      '-U',
      dbUser,
      '-d',
      dbName,
      '-q',
      '-At',
      '-v',
      'ON_ERROR_STOP=1',
    ],
    { input: sql, encoding: 'utf8' },
  );
  return run;
}

// Returns created_at as UTC text. The relay uses it as the event's occurred_at.
function insertOutboxRow() {
  const payload = JSON.stringify({
    parcel_id: randomUUID(), // required by the event contract; no owner fields, so no email is attempted
    tracking_number: tracking,
    previous_status: 'CREATED',
    status: 'PICKED_UP',
    // Not in the contract. The consumer must drop it; the test fails if it reaches the socket.
    recipient_phone: '+94770000000',
  });
  const run =
    psql(`INSERT INTO parcel.outbox_events (id, event_type, routing_key, payload)
VALUES ('${eventId}', 'parcel.status_changed', 'parcel.status.changed', '${payload}'::jsonb)
RETURNING to_char(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"');`);
  if (run.status !== 0)
    fail(`could not insert the outbox row\n${run.stderr || run.stdout}`);
  return run.stdout.trim();
}

function cleanUp() {
  // Best effort: the relay has already published the row, and the 14-day cleanup would remove it anyway.
  psql(
    `DELETE FROM parcel.outbox_events WHERE id = '${eventId}' AND status = 'PUBLISHED';`,
  );
}

let createdAt = '';
let passed = false;
const socket = new WebSocket(url);
const timer = setTimeout(
  () => fail(`no update within ${timeoutMs} ms`),
  timeoutMs,
);

socket.onerror = () =>
  fail(`cannot connect to ${url}. Is the realtime port published?`);
socket.onclose = () => {
  if (!passed)
    fail('the server closed the connection before an update arrived');
};
socket.onopen = () =>
  socket.send(JSON.stringify({ type: 'subscribe', trackingNumber: tracking }));
socket.onmessage = (event) => {
  const message = JSON.parse(String(event.data));
  console.log('<-', JSON.stringify(message));
  if (message.type === 'subscribed' && !createdAt) {
    createdAt = insertOutboxRow(); // no replay, so insert only after the subscription is confirmed
    return;
  }
  if (message.type === 'error')
    fail(`server error: ${message.code ?? JSON.stringify(message)}`);
  if (message.type !== 'parcel.status.updated') return;

  const data = message.data ?? {};
  const keys = Object.keys(data).sort();
  if (JSON.stringify(keys) !== JSON.stringify(PUBLIC_DATA_KEYS))
    fail(`unexpected fields: ${keys.join(', ')}`);
  if (JSON.stringify(message).includes('94770000000'))
    fail('private data leaked to the socket');
  if (data.trackingNumber !== tracking)
    fail(`wrong tracking number: ${data.trackingNumber}`);
  if (data.previousStatus !== 'CREATED')
    fail(`wrong previousStatus: ${data.previousStatus}`);
  if (data.status !== 'PICKED_UP') fail(`wrong status: ${data.status}`);
  const drift = Math.abs(Date.parse(data.updatedAt) - Date.parse(createdAt));
  if (!Number.isFinite(drift) || drift > 2000)
    fail(
      `updatedAt ${data.updatedAt} does not match the outbox time ${createdAt}`,
    );

  passed = true;
  clearTimeout(timer);
  cleanUp();
  console.log('PASS: update received with only the four public fields');
  socket.close();
  process.exit(0);
};
