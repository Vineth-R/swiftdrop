import assert from 'node:assert/strict';
import { afterEach, test } from 'node:test';

import { restoreFetch, stubFetch } from './api-test-support.mjs';

const { listCourierDeliveries, uploadCourierDeliveryProof } =
  await import('./courier-api.ts');

const delivery = {
  id: '00000000-0000-4000-8000-000000000101',
  trackingNumber: 'SDTODAY001',
  status: 'OUT_FOR_DELIVERY',
  pickupAddress: '10 Test Road, Colombo',
  dropoffAddress: '45 Lake Drive',
  dropoffCity: 'Kandy',
  recipientName: 'Nimali Perera',
  recipientPhone: '+94771234567',
  weightKg: 2.5,
  assignedAt: '2026-11-05T09:00:00Z',
};

const pagination = {
  page: 1,
  pageSize: 20,
  totalItems: 1,
  totalPages: 1,
};

function reply(overrides = {}) {
  return {
    data: [delivery],
    meta: { requestId: 'req-1', pagination: { ...pagination, ...overrides } },
  };
}

afterEach(restoreFetch);

test('returns the items and pagination and sends the Colombo date', async () => {
  const { signal } = new AbortController();
  const calls = stubFetch(200, reply());
  const result = await listCourierDeliveries({
    date: '2026-11-09',
    page: 1,
    signal,
  });

  assert.deepEqual(result, { items: [delivery], pagination });
  assert.equal(calls.length, 1);
  assert.equal(calls[0].method, 'GET');
  assert.equal(calls[0].signal, signal);
  assert.equal(
    calls[0].url,
    '/api/v1/courier/deliveries?date=2026-11-09&page=1&pageSize=20',
  );
});

test('sends the optional completed-status filter', async () => {
  const calls = stubFetch(200, reply({ page: 2 }));
  await listCourierDeliveries({
    date: '2026-11-09',
    status: 'DELIVERED',
    page: 2,
  });

  assert.equal(
    calls[0].url,
    '/api/v1/courier/deliveries?date=2026-11-09&page=2&pageSize=20&status=DELIVERED',
  );
});

test('accepts an empty page with zero totals', async () => {
  const emptyPagination = {
    page: 1,
    pageSize: 20,
    totalItems: 0,
    totalPages: 0,
  };
  stubFetch(200, {
    data: [],
    meta: { requestId: 'req-1', pagination: emptyPagination },
  });

  const result = await listCourierDeliveries({ date: '2026-11-09', page: 1 });

  assert.deepEqual(result, { items: [], pagination: emptyPagination });
});

const malformed = [
  [
    'data is not an array',
    { data: {}, meta: { requestId: 'req-1', pagination } },
  ],
  ['missing pagination', { data: [], meta: { requestId: 'req-1' } }],
  [
    'null pagination',
    { data: [], meta: { requestId: 'req-1', pagination: null } },
  ],
  ['non-integer page', reply({ page: '1' })],
  ['page differs from the request', reply({ page: 2 })],
  ['non-integer pageSize', reply({ pageSize: '20' })],
  ['pageSize below one', reply({ pageSize: 0 })],
  ['non-integer totalItems', reply({ totalItems: 1.5 })],
  ['negative totalItems', reply({ totalItems: -1 })],
  ['non-integer totalPages', reply({ totalPages: '1' })],
  ['negative totalPages', reply({ totalPages: -1 })],
];

for (const [name, body] of malformed) {
  test(`rejects ${name}`, async () => {
    stubFetch(200, body);
    await assert.rejects(
      listCourierDeliveries({ date: '2026-11-09', page: 1 }),
      { code: 'INTERNAL', status: 200 },
    );
  });
}

test('keeps the request id on a contract error', async () => {
  stubFetch(200, { data: [], meta: { requestId: 'req-1' } });
  await assert.rejects(listCourierDeliveries({ date: '2026-11-09', page: 1 }), {
    code: 'INTERNAL',
    status: 200,
    requestId: 'req-1',
  });
});

test('builds the documented courier proof multipart form', async () => {
  let request;
  globalThis.fetch = async (url, init) => {
    request = { url: String(url), ...init };
    return new Response(
      JSON.stringify({
        data: {
          id: '00000000-0000-4000-8000-000000000201',
          parcelId: delivery.id,
          recipientName: 'Nimali Perera',
          note: 'Handed over',
          hasPhoto: true,
          createdAt: '2026-11-10T09:00:00Z',
        },
        meta: {},
      }),
      { status: 201, headers: { 'Content-Type': 'application/json' } },
    );
  };
  const photo = new File(['png'], 'proof.png', { type: 'image/png' });
  const controller = new AbortController();

  const result = await uploadCourierDeliveryProof(
    delivery.id,
    { photo, recipientName: 'Nimali Perera', note: 'Handed over' },
    controller.signal,
  );

  assert.equal(request.url, `/api/v1/courier/deliveries/${delivery.id}/proof`);
  assert.equal(request.method, 'POST');
  assert.equal(request.signal, controller.signal);
  assert.ok(request.body instanceof FormData);
  assert.equal(request.body.get('photo'), photo);
  assert.equal(request.body.get('recipientName'), 'Nimali Perera');
  assert.equal(request.body.get('note'), 'Handed over');
  assert.equal(request.headers['Content-Type'], undefined);
  assert.equal(result.parcelId, delivery.id);
});
