import assert from 'node:assert/strict';
import { test } from 'node:test';

import './api-test-support.mjs';

const { todayInColombo } = await import('./colombo-date.ts');

test('uses the Asia/Colombo calendar date', () => {
  assert.equal(todayInColombo(new Date('2026-11-08T18:29:59Z')), '2026-11-08');
  assert.equal(todayInColombo(new Date('2026-11-08T18:30:00Z')), '2026-11-09');
});

test('the Colombo date can differ from the UTC date', () => {
  const now = new Date('2026-11-08T20:00:00Z');
  assert.equal(now.toISOString().slice(0, 10), '2026-11-08');
  assert.equal(todayInColombo(now), '2026-11-09');
});

test('handles the next Colombo midnight', () => {
  assert.equal(todayInColombo(new Date('2026-11-09T18:29:59Z')), '2026-11-09');
  assert.equal(todayInColombo(new Date('2026-11-09T18:30:00Z')), '2026-11-10');
});
