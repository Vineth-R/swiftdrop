import assert from 'node:assert/strict';
import { afterEach, test } from 'node:test';

import { fail, failNetwork, ok, restoreFetch } from './api-test-support.mjs';

const { getTracking, isTrackingNumber, normalizeTrackingNumber } =
  await import('./tracking-api.ts');

const tracking = {
  trackingNumber: 'SDK7M2Q9XP4',
  status: 'OUT_FOR_DELIVERY',
  pickupCity: 'Colombo',
  dropoffCity: 'Kandy',
  lastUpdatedAt: '2026-11-02T15:40:00Z',
  timeline: [
    { status: 'CREATED', at: '2026-11-02T10:00:00Z' },
    { status: 'OUT_FOR_DELIVERY', at: '2026-11-02T15:40:00Z' },
  ],
};

afterEach(restoreFetch);

test('asks for the number and returns the tracking', async () => {
  const calls = ok(tracking);
  const { signal } = new AbortController();
  assert.deepEqual(await getTracking('SDK7M2Q9XP4', signal), tracking);
  assert.equal(calls.length, 1);
  assert.equal(calls[0].method, 'GET');
  assert.equal(calls[0].url, '/api/v1/tracking/SDK7M2Q9XP4');
  assert.equal(calls[0].signal, signal);
});

// The service only accepts capitals, so typed or pasted numbers are cleaned up.
for (const typed of ['sdk7m2q9xp4', '  SDK7M2Q9XP4  ', '\tsdk7M2q9xP4\n']) {
  test(`cleans up ${JSON.stringify(typed)}`, async () => {
    assert.equal(normalizeTrackingNumber(typed), 'SDK7M2Q9XP4');
    const calls = ok(tracking);
    await getTracking(typed);
    assert.equal(calls[0].url, '/api/v1/tracking/SDK7M2Q9XP4');
  });
}

test('isTrackingNumber accepts only what the service accepts', () => {
  for (const good of ['SDK7M2Q9XP4', 'SDN1A2B3C4D', 'SDK00000000']) {
    assert.equal(isTrackingNumber(good), true, good);
  }
  for (const bad of [
    '',
    'sdk7m2q9xp4',
    'SDX7M2Q9XP4',
    'SDK7M2Q9XP',
    'SDK7M2Q9XP44',
    'SDK7M2Q9X-4',
    ' SDK7M2Q9XP4',
    'SDK7M2Q9XP4\n',
  ]) {
    assert.equal(isTrackingNumber(bad), false, JSON.stringify(bad));
  }
});

test('a number cannot change the path', async () => {
  const calls = ok(tracking);
  await getTracking('../admin/users');
  assert.equal(calls[0].url, '/api/v1/tracking/..%2FADMIN%2FUSERS');
});

test('keeps only public fields, whatever the server sends', async () => {
  ok({
    ...tracking,
    senderName: 'Nimal Perera',
    recipientPhone: '+94771234567',
    dropoffAddress: '45 Lake Drive',
    priceAmount: 550,
    timeline: tracking.timeline.map((event) => ({
      ...event,
      note: 'Left with Amali Fernando',
      actorUserId: 'user-1',
    })),
  });
  assert.deepEqual(await getTracking('SDK7M2Q9XP4'), tracking);
});

for (const [name, status, apiCode, code] of [
  ['unknown number', 404, 'TRACKING_NOT_FOUND', 'TRACKING_NOT_FOUND'],
  ['wrong format', 422, 'VALIDATION_ERROR', 'VALIDATION_FAILED'],
  ['service down', 503, 'SERVICE_UNAVAILABLE', 'INTERNAL'],
]) {
  test(`${name}: ${status} ${apiCode} becomes ${code}`, async () => {
    fail(status, apiCode);
    await assert.rejects(getTracking('SDK7M2Q9XP4'), { code, status });
  });
}

test('too many requests: keeps the wait from Retry-After', async () => {
  fail(429, 'RATE_LIMITED', { 'Retry-After': '60' });
  await assert.rejects(getTracking('SDK7M2Q9XP4'), {
    code: 'RATE_LIMITED',
    retryAfter: 60,
  });
});

test('no connection: becomes NETWORK', async () => {
  failNetwork();
  await assert.rejects(getTracking('SDK7M2Q9XP4'), { code: 'NETWORK' });
});
