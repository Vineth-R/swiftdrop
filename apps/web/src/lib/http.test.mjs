import assert from 'node:assert/strict';
import { afterEach, test } from 'node:test';

import './api-test-support.mjs';

const { send } = await import('./http.ts');
const realFetch = globalThis.fetch;

afterEach(() => {
  globalThis.fetch = realFetch;
});

test('keeps JSON request behavior unchanged', async () => {
  let request;
  globalThis.fetch = async (url, init) => {
    request = { url: String(url), ...init };
    return new Response(null, { status: 204 });
  };

  const controller = new AbortController();
  await send('/test', {
    method: 'POST',
    body: { value: 'parcel' },
    signal: controller.signal,
    headers: { 'X-Test': 'yes' },
  });

  assert.equal(request.url, '/api/v1/test');
  assert.equal(request.body, JSON.stringify({ value: 'parcel' }));
  assert.equal(request.headers['Content-Type'], 'application/json');
  assert.equal(request.headers.Accept, 'application/json');
  assert.equal(request.headers['X-Test'], 'yes');
  assert.equal(request.credentials, 'same-origin');
  assert.equal(request.cache, 'no-store');
  assert.equal(request.signal, controller.signal);
});

test('passes FormData through and lets fetch create the multipart boundary', async () => {
  let request;
  globalThis.fetch = async (_url, init) => {
    request = init;
    return new Response(null, { status: 204 });
  };
  const form = new FormData();
  form.append('recipientName', 'Nimali Perera');

  await send('/courier/deliveries/parcel/proof', {
    method: 'POST',
    body: form,
  });

  assert.equal(request.body, form);
  assert.equal(request.headers['Content-Type'], undefined);
  assert.equal(request.headers.Accept, 'application/json');
  assert.equal(request.credentials, 'same-origin');
  assert.equal(request.cache, 'no-store');
});
