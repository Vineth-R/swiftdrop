import assert from 'node:assert/strict';
import { afterEach, test } from 'node:test';
import { restoreFetch, stubFetch } from './api-test-support.mjs';

const { api, apiBlob } = await import('./api.ts');
afterEach(restoreFetch);

test('creation preserves the idempotency key during session renewal and retry', async () => {
  const calls = [];
  globalThis.fetch = async (url, init) => {
    calls.push({ url, ...init });
    if (calls.length === 1) return new Response('{}', { status: 401 });
    return new Response(JSON.stringify({ data: { id: 'parcel-1' } }));
  };
  const input = { expectedFee: 550 };
  assert.deepEqual(
    await api('/parcels', {
      method: 'POST',
      body: input,
      headers: { 'Idempotency-Key': 'same-intended-submission' },
    }),
    { id: 'parcel-1' },
  );
  assert.equal(calls[1].url, '/api/v1/auth/refresh');
  for (const call of [calls[0], calls[2]]) {
    assert.equal(call.headers['Idempotency-Key'], 'same-intended-submission');
    assert.equal(call.headers['Content-Type'], 'application/json');
    assert.equal(call.body, JSON.stringify(input));
    assert.equal(call.credentials, 'same-origin');
  }
});

test('PRICE_CHANGED preserves only a valid numeric currentFee', async () => {
  for (const currentFee of [600, 0, -1, '600', null]) {
    stubFetch(409, {
      error: { code: 'PRICE_CHANGED', currentFee, message: 'PRIVATE' },
      meta: { requestId: 'req-price' },
    });
    await assert.rejects(api('/parcels', { method: 'POST', body: {} }), {
      code: 'PRICE_CHANGED',
      requestId: 'req-price',
      currentFee:
        typeof currentFee === 'number' && currentFee >= 0
          ? currentFee
          : undefined,
    });
  }
});

test('PDF download renews the session and preserves the abort signal', async () => {
  const { signal } = new AbortController();
  const calls = [];
  globalThis.fetch = async (url, init) => {
    calls.push({ url, ...init });
    if (calls.length === 1) return new Response('{}', { status: 401 });
    if (url.endsWith('/auth/refresh')) return new Response('{}');
    return new Response('%PDF-1.4\nreceipt', {
      headers: { 'Content-Type': 'application/pdf' },
    });
  };
  const blob = await apiBlob('/parcels/one/receipt', { signal });
  assert.equal(await blob.text(), '%PDF-1.4\nreceipt');
  assert.equal(calls.length, 3);
  assert.equal(calls[2].headers.Accept, 'application/pdf');
  assert.equal(calls[2].signal, signal);
});

test('PDF download maps ownership-safe 404 and rejects non-PDF responses', async () => {
  stubFetch(404, { error: { code: 'NOT_FOUND' } });
  await assert.rejects(apiBlob('/parcels/other/receipt'), {
    code: 'NOT_FOUND',
  });
  stubFetch(200, { data: {} }, { 'Content-Type': 'application/json' });
  await assert.rejects(apiBlob('/parcels/one/receipt'), { code: 'INTERNAL' });
});
