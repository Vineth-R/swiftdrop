import assert from 'node:assert/strict';
import { afterEach, test } from 'node:test';

import {
  fail,
  failNetwork,
  ok,
  restoreFetch,
  stubFetch,
} from './api-test-support.mjs';

const { ApiError } = await import('./api.ts');
const {
  createPricingRule,
  createWeightBand,
  createZone,
  estimate,
  getZones,
  listAdminZones,
  listPricingRules,
  listSizeSurcharges,
  listWeightBands,
  setSizeSurcharge,
  updatePricingRule,
  updateWeightBand,
  updateZone,
} = await import('./pricing-api.ts');

const zones = { originZoneId: 'zone-a', destinationZoneId: 'zone-b' };
const input = { ...zones, weightKg: 2, sizeCategory: 'SMALL' };
const band = { minWeightKg: 5, maxWeightKg: 10, charge: 250 };

afterEach(restoreFetch);

test('every size has a name and a volume, in one label', async () => {
  const { sizeCategories, sizeLabel } = await import('./pricing-api.ts');
  assert.deepEqual(sizeCategories.map(sizeLabel), [
    'Small (up to 5 L)',
    'Medium (up to 20 L)',
    'Large (up to 60 L)',
    'Oversized (over 60 L)',
  ]);
});

// What each call must send. The paths are the routes of the pricing service.
for (const [name, run, method, path, body] of [
  ['estimate', () => estimate(input), 'POST', '/pricing/estimate', input],
  ['getZones', () => getZones(), 'GET', '/zones'],
  [
    'listAdminZones without a query',
    () => listAdminZones(),
    'GET',
    '/admin/zones',
  ],
  [
    'listAdminZones with a filter and paging',
    () => listAdminZones({ isActive: false, page: 2, sort: 'createdAt:asc' }),
    'GET',
    '/admin/zones?isActive=false&page=2&sort=createdAt%3Aasc',
  ],
  [
    'createZone',
    () => createZone({ code: 'GLE', name: 'Galle', districts: ['Galle'] }),
    'POST',
    '/admin/zones',
    { code: 'GLE', name: 'Galle', districts: ['Galle'] },
  ],
  [
    'updateZone',
    () => updateZone('zone/1', { isActive: false }),
    'PATCH',
    '/admin/zones/zone%2F1',
    { isActive: false },
  ],
  [
    'listPricingRules without a query',
    () => listPricingRules(),
    'GET',
    '/admin/pricing/rules',
  ],
  [
    'listPricingRules with filters',
    () =>
      listPricingRules({ originZoneId: 'zone-a', isActive: false, page: 2 }),
    'GET',
    '/admin/pricing/rules?originZoneId=zone-a&isActive=false&page=2',
  ],
  [
    'createPricingRule',
    () => createPricingRule({ ...zones, baseFee: 450 }),
    'POST',
    '/admin/pricing/rules',
    { ...zones, baseFee: 450 },
  ],
  [
    'updatePricingRule',
    () => updatePricingRule('rule-1', { baseFee: 500 }),
    'PATCH',
    '/admin/pricing/rules/rule-1',
    { baseFee: 500 },
  ],
  [
    'listWeightBands',
    () => listWeightBands({ page: 2, pageSize: 50 }),
    'GET',
    '/admin/pricing/bands?page=2&pageSize=50',
  ],
  [
    'createWeightBand',
    () => createWeightBand(band),
    'POST',
    '/admin/pricing/bands',
    band,
  ],
  [
    'updateWeightBand',
    () => updateWeightBand('band-1', { charge: 300 }),
    'PATCH',
    '/admin/pricing/bands/band-1',
    { charge: 300 },
  ],
  [
    'listSizeSurcharges',
    () => listSizeSurcharges(),
    'GET',
    '/admin/pricing/surcharges',
  ],
  [
    'setSizeSurcharge',
    () => setSizeSurcharge('MEDIUM', 175),
    'PUT',
    '/admin/pricing/surcharges/MEDIUM',
    { surcharge: 175 },
  ],
]) {
  test(`${name} sends ${method} ${path}`, async () => {
    const calls = ok({});
    await run();
    assert.equal(calls.length, 1);
    assert.equal(calls[0].method, method);
    assert.equal(calls[0].url, `/api/v1${path}`);
    assert.deepEqual(calls[0].body, body);
  });
}

test('estimate returns the fee and passes on the abort signal', async () => {
  const fee = {
    ...input,
    currency: 'LKR',
    baseFee: 450,
    weightCharge: 100,
    sizeSurcharge: 0,
    total: 550,
  };
  const calls = ok(fee);
  const { signal } = new AbortController();
  assert.deepEqual(await estimate(input, signal), fee);
  assert.equal(calls[0].signal, signal);
});

test('getZones returns the zones', async () => {
  const list = [{ id: 'zone-a', code: 'CMB', name: 'Colombo', districts: [] }];
  ok(list);
  assert.deepEqual(await getZones(), list);
});

test('a list call also returns the pagination', async () => {
  const pagination = { page: 2, pageSize: 20, totalItems: 21, totalPages: 2 };
  const rule = { id: 'rule-1', baseFee: 450 };
  stubFetch(200, { data: [rule], meta: { pagination } });
  const { data, meta } = await listPricingRules({ page: 2 });
  assert.deepEqual(data, [rule]);
  assert.deepEqual(meta.pagination, pagination);
});

for (const [name, run, status, apiCode, code] of [
  [
    'estimate, unknown zone',
    () => estimate(input),
    404,
    'ZONE_NOT_FOUND',
    'ZONE_NOT_FOUND',
  ],
  [
    'estimate, unpriced route',
    () => estimate(input),
    422,
    'ROUTE_NOT_SERVICEABLE',
    'ROUTE_NOT_SERVICEABLE',
  ],
  [
    'estimate, weight outside every band',
    () => estimate(input),
    422,
    'WEIGHT_NOT_PRICED',
    'WEIGHT_NOT_PRICED',
  ],
  [
    'estimate, invalid field',
    () => estimate(input),
    422,
    'VALIDATION_ERROR',
    'VALIDATION_FAILED',
  ],
  [
    'createZone, code in use',
    () => createZone({ code: 'CMB', name: 'Colombo' }),
    409,
    'ZONE_CODE_EXISTS',
    'ZONE_CODE_EXISTS',
  ],
  [
    'createPricingRule, route already priced',
    () => createPricingRule({ ...zones, baseFee: 450 }),
    409,
    'RULE_EXISTS',
    'RULE_EXISTS',
  ],
  [
    'createWeightBand, overlap',
    () => createWeightBand(band),
    409,
    'BAND_OVERLAP',
    'BAND_OVERLAP',
  ],
  [
    'updateWeightBand, overlap',
    () => updateWeightBand('band-1', { maxWeightKg: 20 }),
    409,
    'BAND_OVERLAP',
    'BAND_OVERLAP',
  ],
  [
    'updateZone, missing',
    () => updateZone('zone-1', { name: 'Galle' }),
    404,
    'NOT_FOUND',
    'NOT_FOUND',
  ],
  [
    'listPricingRules, not an admin',
    () => listPricingRules(),
    403,
    'FORBIDDEN',
    'FORBIDDEN',
  ],
  [
    'getZones, service down',
    () => getZones(),
    503,
    'SERVICE_UNAVAILABLE',
    'INTERNAL',
  ],
]) {
  test(`${name}: ${status} ${apiCode} becomes ${code}`, async () => {
    fail(status, apiCode);
    await assert.rejects(run(), (error) => {
      assert.ok(error instanceof ApiError);
      assert.equal(error.code, code);
      assert.equal(error.status, status);
      return true;
    });
  });
}

test('estimate, too many requests: keeps the wait from Retry-After', async () => {
  fail(429, 'RATE_LIMITED', { 'Retry-After': '30' });
  await assert.rejects(estimate(input), {
    code: 'RATE_LIMITED',
    retryAfter: 30,
  });
});

test('getZones, no connection: becomes NETWORK', async () => {
  failNetwork();
  await assert.rejects(getZones(), { code: 'NETWORK' });
});
