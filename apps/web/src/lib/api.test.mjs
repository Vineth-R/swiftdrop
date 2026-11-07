import assert from 'node:assert/strict';
import { afterEach, describe, it } from 'node:test';

import { fail, failNetwork, ok, restoreFetch } from './api-test-support.mjs';

const { onSessionExpired, refreshSession } = await import('./api.ts');

afterEach(restoreFetch);

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
