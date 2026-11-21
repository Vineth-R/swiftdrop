import assert from 'node:assert/strict';
import { test } from 'node:test';

import './api-test-support.mjs';

const { readError } = await import('./api-errors.ts');

function problem(status, code, headers = {}) {
  return new Response(
    JSON.stringify({
      error: { code, message: 'RAW SERVER WORDS' },
      meta: { requestId: 'req-1' },
    }),
    { status, headers },
  );
}

test('maps PROOF_LIMIT_REACHED to its own code', async () => {
  const error = await readError(problem(409, 'PROOF_LIMIT_REACHED'));

  assert.equal(error.code, 'PROOF_LIMIT_REACHED');
  assert.equal(error.status, 409);
  assert.equal(error.requestId, 'req-1');
});

test('keeps an unrecognised 409 as CONFLICT', async () => {
  const error = await readError(problem(409, 'SOMETHING_ELSE'));

  assert.equal(error.code, 'CONFLICT');
  assert.equal(error.status, 409);
});

test('keeps the API code alongside the screen code', async () => {
  const error = await readError(problem(422, 'WEBHOOK_URL_NOT_ALLOWED'));

  assert.equal(error.code, 'VALIDATION_FAILED');
  assert.equal(error.apiCode, 'WEBHOOK_URL_NOT_ALLOWED');
});

test('reads Retry-After from a 429', async () => {
  const error = await readError(
    problem(429, 'RATE_LIMITED', { 'Retry-After': '30' }),
  );

  assert.equal(error.code, 'RATE_LIMITED');
  assert.equal(error.retryAfter, 30);
});
