#!/usr/bin/env node
// Realtime end-to-end smoke test (M10.3).
//   outbox row -> parcel relay -> RabbitMQ -> realtime consumer -> WebSocket client
// Needs Node 22+ (global WebSocket) and a running stack with the realtime port published.
//   node scripts/realtime-smoke.mjs
// Env: WS_URL (default ws://localhost:8085/ws), TRACKING (default SDK7M2Q9XP4), TIMEOUT_MS (default 15000)
import { randomUUID } from 'node:crypto';
import { spawnSync } from 'node:child_process';

const url = process.env.WS_URL ?? 'ws://localhost:8085/ws';
const tracking = process.env.TRACKING ?? 'SDK7M2Q9XP4';
const timeoutMs = Number(process.env.TIMEOUT_MS ?? 15000);
const PUBLIC_DATA_KEYS = [
  'previousStatus',
  'status',
  'trackingNumber',
  'updatedAt',
];

function fail(message) {
  console.error(`FAIL: ${message}`);
  process.exit(1);
}

function insertOutboxRow() {
  const payload = JSON.stringify({
    tracking_number: tracking,
    previous_status: 'CREATED',
    status: 'PICKED_UP',
    // Must be dropped by the consumer. If it reaches the socket, the test fails.
    recipient_phone: '+94770000000',
  });
  const sql = `INSERT INTO parcel.outbox_events (id, event_type, routing_key, payload)
VALUES ('${randomUUID()}', 'parcel.status_changed', 'parcel.status.changed', '${payload}'::jsonb);`;
  const run = spawnSync(
    'docker',
    [
      'compose',
      'exec',
      '-T',
      'postgres',
      'psql',
      '-U',
      'swiftdrop',
      '-d',
      'swiftdrop',
      '-v',
      'ON_ERROR_STOP=1',
    ],
    { input: sql, encoding: 'utf8' },
  );
  if (run.status !== 0)
    fail(`could not insert the outbox row\n${run.stderr || run.stdout}`);
}

const socket = new WebSocket(url);
const timer = setTimeout(
  () => fail(`no update within ${timeoutMs} ms`),
  timeoutMs,
);
let inserted = false;

socket.onerror = () =>
  fail(`cannot connect to ${url}. Is the realtime port published?`);
socket.onopen = () =>
  socket.send(JSON.stringify({ type: 'subscribe', trackingNumber: tracking }));
socket.onmessage = (event) => {
  const message = JSON.parse(String(event.data));
  console.log('<-', JSON.stringify(message));
  if (message.type === 'subscribed' && !inserted) {
    inserted = true; // the service has no replay, so insert only after the subscription is confirmed
    insertOutboxRow();
    return;
  }
  if (message.type === 'error')
    fail(`server error: ${message.code ?? JSON.stringify(message)}`);
  if (message.type !== 'parcel.status.updated') return;

  const keys = Object.keys(message.data ?? {}).sort();
  if (JSON.stringify(keys) !== JSON.stringify(PUBLIC_DATA_KEYS))
    fail(`unexpected fields: ${keys.join(', ')}`);
  if (JSON.stringify(message).includes('94770000000'))
    fail('private data leaked to the socket');
  if (
    message.data.trackingNumber !== tracking ||
    message.data.status !== 'PICKED_UP'
  )
    fail('wrong update');
  clearTimeout(timer);
  console.log('PASS: update received with only the four public fields');
  socket.close();
  process.exit(0);
};
