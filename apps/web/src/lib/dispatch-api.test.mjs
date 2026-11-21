import assert from 'node:assert/strict';
import { afterEach, test } from 'node:test';

import {
  fail,
  failNetwork,
  ok,
  restoreFetch,
} from './api-test-support.mjs';

const { ApiError } = await import('./api.ts');
const {
  listUnassignedParcels,
  assignCourier,
  getCourierWorkload,
  searchAllParcels,
  listCouriers,
  listZones,
} = await import('./dispatch-api.ts');

afterEach(restoreFetch);

test('listUnassignedParcels sends GET /dispatch/unassigned without query', async () => {
  const calls = ok([]);
  await listUnassignedParcels();
  assert.equal(calls.length, 1);
  assert.equal(calls[0].method, 'GET');
  assert.equal(calls[0].url, '/api/v1/dispatch/unassigned');
});

test('listUnassignedParcels sends query parameters correctly', async () => {
  const calls = ok([]);
  await listUnassignedParcels({
    zoneId: 'z-123',
    from: '2026-11-01',
    to: '2026-11-05',
    page: 2,
    pageSize: 20,
    sort: 'createdAt:desc',
  });
  assert.equal(calls.length, 1);
  assert.equal(calls[0].method, 'GET');
  assert.equal(
    calls[0].url,
    '/api/v1/dispatch/unassigned?zoneId=z-123&from=2026-11-01&to=2026-11-05&page=2&pageSize=20&sort=createdAt%3Adesc',
  );
});

test('assignCourier sends PUT /dispatch/parcels/:id/assignment with body', async () => {
  const payload = { courierId: 'c-456', reason: 'Reassigning due to overload' };
  const calls = ok({
    parcelId: 'p-1',
    trackingNumber: 'SWD-12345678',
    courierId: 'c-456',
    assignedBy: 'dispatcher-1',
    assignedAt: '2026-11-06T12:00:00Z',
  });

  const result = await assignCourier('p-1', payload);
  assert.equal(calls.length, 1);
  assert.equal(calls[0].method, 'PUT');
  assert.equal(calls[0].url, '/api/v1/dispatch/parcels/p-1/assignment');
  assert.deepEqual(calls[0].body, payload);
  assert.equal(result.courierId, 'c-456');
});

test('getCourierWorkload sends GET /dispatch/workload', async () => {
  const calls = ok([
    {
      courierId: 'c-1',
      fullName: 'John Doe',
      isAvailable: true,
      activeDeliveries: 3,
      byStatus: { PICKED_UP: 2, OUT_FOR_DELIVERY: 1 },
    },
  ]);

  const workload = await getCourierWorkload();
  assert.equal(calls.length, 1);
  assert.equal(calls[0].method, 'GET');
  assert.equal(calls[0].url, '/api/v1/dispatch/workload');
  assert.equal(workload.length, 1);
  assert.equal(workload[0].activeDeliveries, 3);
});

test('searchAllParcels sends GET /admin/parcels with filters', async () => {
  const calls = ok([]);
  await searchAllParcels({
    status: 'IN_TRANSIT',
    courierId: 'c-1',
    trackingNumber: 'SWD',
    page: 1,
    pageSize: 20,
  });
  assert.equal(calls.length, 1);
  assert.equal(calls[0].method, 'GET');
  assert.equal(
    calls[0].url,
    '/api/v1/admin/parcels?status=IN_TRANSIT&courierId=c-1&trackingNumber=SWD&page=1&pageSize=20',
  );
});

test('listCouriers sends GET /couriers/ with availability filter', async () => {
  const calls = ok([]);
  await listCouriers({ isAvailable: true, page: 1, pageSize: 50 });
  assert.equal(calls.length, 1);
  assert.equal(calls[0].method, 'GET');
  // The trailing slash matters: nginx redirects /couriers without it.
  assert.equal(
    calls[0].url,
    '/api/v1/couriers/?isAvailable=true&page=1&pageSize=50',
  );
});

test('listZones sends GET /zones', async () => {
  const calls = ok([{ id: 'z-1', code: 'CMB', name: 'Colombo', districts: ['Colombo'] }]);
  const zones = await listZones();
  assert.equal(calls.length, 1);
  assert.equal(calls[0].method, 'GET');
  assert.equal(calls[0].url, '/api/v1/zones');
  assert.equal(zones.length, 1);
  assert.equal(zones[0].code, 'CMB');
});

test('assignCourier translates 409 PARCEL_CLOSED', async () => {
  fail(409, 'PARCEL_CLOSED');
  await assert.rejects(
    () => assignCourier('p-1', { courierId: 'c-1' }),
    (error) => error instanceof ApiError && error.code === 'PARCEL_CLOSED',
  );
});

test('assignCourier translates 422 COURIER_NOT_ELIGIBLE', async () => {
  fail(422, 'COURIER_NOT_ELIGIBLE');
  await assert.rejects(
    () => assignCourier('p-1', { courierId: 'c-1' }),
    (error) => error instanceof ApiError && error.code === 'COURIER_NOT_ELIGIBLE',
  );
});

test('network failure translates to NETWORK code', async () => {
  failNetwork();
  await assert.rejects(
    () => getCourierWorkload(),
    (error) => error instanceof ApiError && error.code === 'NETWORK',
  );
});

