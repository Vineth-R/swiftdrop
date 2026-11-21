import assert from 'node:assert/strict';
import { afterEach, describe, it } from 'node:test';

import { fail, failNetwork, ok, restoreFetch } from './api-test-support.mjs';

const { api, ApiError, onSessionExpired, refreshSession } =
  await import('./api.ts');

afterEach(restoreFetch);

/**
 * Answers each call from `answer(path, nthCallToThatPath)` and returns the
 * calls it saw. A reply is `[status, body]`.
 */
function answerBy(answer) {
  const calls = [];
  const seen = new Map();
  globalThis.fetch = async (url, init = {}) => {
    const path = String(url).replace('/api/v1', '');
    const nth = (seen.get(path) ?? 0) + 1;
    seen.set(path, nth);
    calls.push({ path, method: init.method ?? 'GET' });
    const [status, body] = answer(path, nth);
    return new Response(JSON.stringify(body), { status });
  };
  return calls;
}

const problem = (code) => ({
  error: { code, message: 'RAW SERVER WORDS' },
  meta: {},
});
const success = (data) => ({ data, meta: {} });

/** Counts how often the app was told the session is gone. */
function countExpiries() {
  const count = { value: 0 };
  count.stop = onSessionExpired(() => {
    count.value += 1;
  });
  return count;
}

describe('api() when the session ends', () => {
  it('renews once and sends the call again after a revoked session', async () => {
    const calls = answerBy((path, nth) => {
      if (path === '/auth/refresh') return [200, success({})];
      return nth === 1
        ? [401, problem('SESSION_REVOKED')]
        : [200, success({ id: 'merchant-1' })];
    });
    const expired = countExpiries();

    const result = await api('/merchants/me');
    expired.stop();

    assert.deepEqual(result, { id: 'merchant-1' });
    assert.deepEqual(
      calls.map((call) => `${call.method} ${call.path}`),
      ['GET /merchants/me', 'POST /auth/refresh', 'GET /merchants/me'],
    );
    assert.equal(expired.value, 0, 'a renewed session is not an ended one');
  });

  it('ends the session when the renewal is refused, and sends nothing twice', async () => {
    const calls = answerBy((path) =>
      path === '/auth/refresh'
        ? [401, problem('REFRESH_TOKEN_INVALID')]
        : [401, problem('SESSION_REVOKED')],
    );
    const expired = countExpiries();

    await assert.rejects(
      api('/merchants/me'),
      (error) => error instanceof ApiError && error.code === 'SESSION_REVOKED',
    );
    expired.stop();

    assert.equal(expired.value, 1);
    assert.deepEqual(
      calls.map((call) => call.path),
      ['/merchants/me', '/auth/refresh'],
    );
  });

  it('ends the session when the call is still refused after a renewal', async () => {
    const calls = answerBy((path) =>
      path === '/auth/refresh'
        ? [200, success({})]
        : [401, problem('SESSION_REVOKED')],
    );
    const expired = countExpiries();

    await assert.rejects(api('/merchants/me'), ApiError);
    expired.stop();

    assert.equal(expired.value, 1);
    assert.equal(
      calls.filter((call) => call.path === '/auth/refresh').length,
      1,
    );
  });

  it('keeps a wrong current password as a wrong password, not a session end', async () => {
    const calls = answerBy(() => [401, problem('INVALID_CREDENTIALS')]);
    const expired = countExpiries();

    await assert.rejects(
      api('/users/me/password', { method: 'PUT', body: {} }),
      (error) =>
        error instanceof ApiError && error.code === 'INVALID_CREDENTIALS',
    );
    expired.stop();

    assert.equal(expired.value, 0);
    assert.equal(calls.length, 1, 'no renewal was attempted');
  });

  it('never renews for the auth endpoints themselves', async () => {
    const calls = answerBy(() => [401, problem('REFRESH_TOKEN_INVALID')]);
    const expired = countExpiries();

    await assert.rejects(api('/auth/logout', { method: 'POST' }), ApiError);
    expired.stop();

    assert.equal(expired.value, 0);
    assert.equal(calls.length, 1);
  });

  it('renews once for several calls that all find the token expired', async () => {
    const calls = answerBy((path, nth) => {
      if (path === '/auth/refresh') return [200, success({})];
      return nth === 1 ? [401, problem('UNAUTHENTICATED')] : [200, success({})];
    });

    await Promise.all([api('/parcels'), api('/users/me'), api('/couriers/')]);

    assert.equal(
      calls.filter((call) => call.path === '/auth/refresh').length,
      1,
    );
  });
});

describe('refreshSession', () => {
  it('renews once with a POST to /auth/refresh', async () => {
    const calls = ok({});
    assert.equal(await refreshSession(), true);
    assert.equal(calls.length, 1);
    assert.equal(calls[0].url, '/api/v1/auth/refresh');
    assert.equal(calls[0].method, 'POST');
  });

  it('returns false and reports the session gone when renewal fails', async () => {
    fail(401, 'UNAUTHENTICATED');
    let expired = 0;
    const stop = onSessionExpired(() => {
      expired += 1;
    });

    assert.equal(await refreshSession(), false);
    stop();
    assert.equal(expired, 1);
  });

  it('returns false and reports the session gone when the network fails', async () => {
    failNetwork();
    let expired = 0;
    const stop = onSessionExpired(() => {
      expired += 1;
    });

    assert.equal(await refreshSession(), false);
    stop();
    assert.equal(expired, 1);
  });
});
