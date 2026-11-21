import assert from 'node:assert/strict';
import test from 'node:test';
import { withSafeErrors } from './safe-errors.mjs';

test('network failures omit Playwright call logs with session cookies', async () => {
  const run = withSafeErrors(async () => {
    throw new Error(
      'apiRequestContext.get: read ECONNRESET\nCall log:\n  - Cookie: sd_access=PRIVATE_SESSION_SENTINEL\n',
    );
  });
  await assert.rejects(run(), (error) => {
    assert.equal(error.message, 'apiRequestContext.get: read ECONNRESET');
    assert.ok(!error.stack.includes('PRIVATE_SESSION_SENTINEL'));
    return true;
  });
});

test('credential headers and original diagnostic properties are not retained', async () => {
  const original = new Error(
    'Request failed\nAuthorization: Bearer PRIVATE_AUTH_SENTINEL\nSet-Cookie: PRIVATE_COOKIE_SENTINEL\nX-Internal-Token: PRIVATE_TOKEN_SENTINEL',
    { cause: new Error('PRIVATE_CAUSE_SENTINEL') },
  );
  original.actual = 'PRIVATE_ACTUAL_SENTINEL';
  await assert.rejects(
    withSafeErrors(async () => {
      throw original;
    })(),
    (error) => {
      assert.ok(error.message.includes('Request failed'));
      assert.ok(error.message.includes('[REDACTED]'));
      assert.ok(!error.stack.includes('PRIVATE_'));
      assert.equal(error.cause, undefined);
      assert.equal(error.actual, undefined);
      return true;
    },
  );
});

test('failed HTTP assertions still fail with the original useful message', async () => {
  const original = new assert.AssertionError({ actual: 503, expected: 201 });
  await assert.rejects(
    withSafeErrors(async () => {
      throw original;
    })(),
    (error) => error.message === original.message,
  );
});

test('successful checks preserve their result', async () => {
  assert.equal(await withSafeErrors(async (value) => value)(201), 201);
});
