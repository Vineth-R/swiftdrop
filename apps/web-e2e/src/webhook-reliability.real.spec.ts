import { Buffer } from 'node:buffer';
import { createHmac, randomUUID } from 'node:crypto';

import { expect, test, type APIRequestContext } from '@playwright/test';

import { apiLogin, apiUserId, credentialsForRun } from './support/real-stack';

const EVENT_TIMEOUT = 60_000;
const PNG_PROOF = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAIAAAACCAYAAABytg0kAAAAH0lEQVR4nAASAO3/Av8AAP8AAAAAAAAAAAAAAAAAAwAfFwIBJnyuSgAAAABJRU5ErkJggg==',
  'base64',
);

const sinkControl = 'http://localhost:8099/control';

type Attempt = {
  eventId: string;
  trackingNumber: string;
  attemptNumber: number;
  httpStatus: number | null;
  success: boolean;
  nextRetryAt: string | null;
  deadLettered: boolean;
};

type SinkRequest = {
  headers: Record<string, string | string[]>;
  body: string;
  status: number;
};

type MerchantCase = {
  token: string;
  trackingNumber: string;
  parcelID: string;
  secret: string;
};

test.describe('real-stack webhook reliability', () => {
  test.describe.configure({ retries: 1 });
  test.setTimeout(180_000);

  test('a transient failure is retried and then delivered exactly once', async ({
    request,
  }) => {
    await resetSink(request, [500, 200]);
    const merchant = await createMerchantCase(request);
    await triggerStatus(
      request,
      merchant.parcelID,
      'FAILED',
      'Recipient refused the delivery',
    );

    const attempts = await waitForAttempts(request, merchant, 2);
    expect(attempts.map((a) => a.attemptNumber)).toEqual([1, 2]);
    expect(attempts[0]).toMatchObject({
      success: false,
      httpStatus: 500,
      deadLettered: false,
    });
    expect(attempts[0].nextRetryAt).toBeTruthy();
    expect(attempts[1]).toMatchObject({
      success: true,
      httpStatus: 200,
      deadLettered: false,
    });

    const sink = await sinkRequests(request);
    expect(sink).toHaveLength(2);
    expect(sink[0].status).toBe(500);
    expect(sink[1].status).toBe(200);
    expect(sink[0].body).toBe(sink[1].body);
    expect(signatureMatches(sink[1], merchant.secret)).toBe(true);
  });

  test('persistent failure records all six attempts and dead-letters the event', async ({
    request,
  }) => {
    await resetSink(request, [500, 500, 500, 500, 500, 500], 500);
    const merchant = await createMerchantCase(request);
    await triggerStatus(
      request,
      merchant.parcelID,
      'FAILED',
      'Recipient refused the delivery',
    );

    const attempts = await waitForAttempts(request, merchant, 6);
    expect(attempts.map((a) => a.attemptNumber)).toEqual([1, 2, 3, 4, 5, 6]);
    expect(attempts.every((a) => a.success === false)).toBe(true);
    expect(attempts.every((a) => a.httpStatus === 500)).toBe(true);
    expect(attempts[5]).toMatchObject({
      deadLettered: true,
      nextRetryAt: null,
    });
    expect(await sinkRequests(request)).toHaveLength(6);
  });

  test('rotation makes subsequent deliveries use the new secret', async ({
    request,
  }) => {
    await resetSink(request, [200], 200);
    const merchant = await createMerchantCase(request);
    // Pickup records PICKED_UP and IN_TRANSIT, so the old secret signs both.
    await confirmPickup(request, merchant.parcelID);
    let attempts = await waitForAttempts(request, merchant, 2);
    expect(attempts.every((attempt) => attempt.success)).toBe(true);
    let sink = await sinkRequests(request);
    expect(sink).toHaveLength(2);
    expect(
      sink.every((entry) => signatureMatches(entry, merchant.secret)),
    ).toBe(true);

    const rotated = await rotateWebhook(request, merchant.token);
    expect(rotated).toMatch(/^whsec_/);
    expect(rotated).not.toBe(merchant.secret);

    await triggerStatus(request, merchant.parcelID, 'OUT_FOR_DELIVERY');
    await expect
      .poll(async () => (await sinkRequests(request)).length, {
        timeout: EVENT_TIMEOUT,
        intervals: [500, 1000, 2000],
      })
      .toBe(3);
    sink = await sinkRequests(request);
    expect(signatureMatches(sink[2], rotated)).toBe(true);
    expect(signatureMatches(sink[2], merchant.secret)).toBe(false);

    attempts = await waitForAttempts(request, merchant, 3);
    expect(attempts.filter((attempt) => attempt.success)).toHaveLength(3);
  });
});

async function resetSink(
  request: APIRequestContext,
  statuses: number[],
  defaultStatus = 200,
) {
  const response = await request.post(sinkControl, {
    data: { statuses, defaultStatus, reset: true },
  });
  expect(response.ok()).toBeTruthy();
}

async function sinkRequests(
  request: APIRequestContext,
): Promise<SinkRequest[]> {
  const response = await request.get(sinkControl);
  expect(response.ok()).toBeTruthy();
  return ((await response.json()) as { requests: SinkRequest[] }).requests;
}

async function createMerchantCase(
  request: APIRequestContext,
): Promise<MerchantCase> {
  const admin = credentialsForRun('ADMIN');
  const dispatcher = credentialsForRun('DISPATCHER');
  const courier = credentialsForRun('COURIER_1');
  test.skip(
    !admin || !dispatcher || !courier,
    'real-stack accounts are required',
  );
  if (!admin || !dispatcher || !courier) throw new Error('accounts missing');

  const zones = await loadZones(request);
  const colombo = zones.find((zone) => zone.code === 'CMB');
  const kandy = zones.find((zone) => zone.code === 'KDY');
  if (!colombo || !kandy) throw new Error('CMB and KDY zones are required');

  const suffix = `${Date.now()}-${Math.floor(Math.random() * 10_000)}`;
  const email = `webhook-reliability.${suffix}@example.test`;
  const business = `Webhook Reliability ${suffix}`;
  const registered = await request.post('/api/v1/merchants/register', {
    data: {
      fullName: 'Webhook Reliability Merchant',
      email,
      phone: '+94771234567',
      password: 'Str0ngPass!',
      business: {
        name: business,
        registrationNumber: `PV ${suffix}`,
        contactEmail: email,
        contactPhone: '+94771234567',
        businessAddress: '88 Galle Road, Colombo 03',
        pickupAddress: '88 Galle Road, Colombo 03',
        pickupZoneId: colombo.id,
      },
    },
  });
  expect(registered.status(), await registered.text()).toBe(201);

  const adminToken = await apiLogin(request, admin.email, admin.password);
  const merchantID = await findMerchantID(request, adminToken, business);
  const approved = await request.post(
    `/api/v1/admin/merchants/${merchantID}/approve`,
    { headers: { Authorization: `Bearer ${adminToken}` } },
  );
  expect(approved.ok()).toBeTruthy();
  const merchant = await apiLogin(request, email, 'Str0ngPass!');
  const webhook = await request.put('/api/v1/merchants/me/webhook', {
    headers: { Authorization: `Bearer ${merchant}` },
    data: {
      url: 'https://webhook-sink:8098/hook',
      eventTypes: ['parcel.status_changed'],
      isActive: true,
    },
  });
  expect(webhook.ok(), await webhook.text()).toBeTruthy();
  const secret = ((await webhook.json()) as { data: { signingSecret: string } })
    .data.signingSecret;
  expect(secret).toMatch(/^whsec_/);

  const key = await request.post('/api/v1/merchants/me/keys', {
    headers: { Authorization: `Bearer ${merchant}` },
    data: { name: 'Webhook reliability key' },
  });
  expect(key.status(), await key.text()).toBe(201);
  const apiKey = ((await key.json()) as { data: { apiKey: string } }).data
    .apiKey;
  const order = await request.post('/api/v1/merchant/orders', {
    headers: { 'X-API-Key': apiKey, 'Idempotency-Key': randomUUID() },
    data: {
      externalOrderId: `WR-${randomUUID()}`,
      recipient: {
        name: 'Webhook Recipient',
        phone: '+94712345678',
        address: '45 Lake Drive',
        city: 'Kandy',
        zoneId: kandy.id,
      },
      weightKg: 1,
      sizeCategory: 'SMALL',
      description: 'Webhook reliability parcel',
    },
  });
  expect(order.status(), await order.text()).toBe(201);
  const trackingNumber = (
    (await order.json()) as {
      data: { trackingNumber: string };
    }
  ).data.trackingNumber;
  const parcelID = await findParcelID(request, adminToken, trackingNumber);

  const dispatcherToken = await apiLogin(
    request,
    dispatcher.email,
    dispatcher.password,
  );
  const courierToken = await apiLogin(request, courier.email, courier.password);
  const courierID = await apiUserId(request, courierToken);
  const assigned = await request.put(
    `/api/v1/dispatch/parcels/${parcelID}/assignment`,
    {
      headers: { Authorization: `Bearer ${dispatcherToken}` },
      data: { courierId: courierID, reason: 'Webhook reliability test' },
    },
  );
  expect(assigned.status(), await assigned.text()).toBe(200);

  return { token: merchant, trackingNumber, parcelID, secret };
}

async function triggerStatus(
  request: APIRequestContext,
  parcelID: string,
  status: string,
  failureReason?: string,
) {
  const courier = credentialsForRun('COURIER_1');
  if (!courier) throw new Error('COURIER_1 is required');
  const token = await apiLogin(request, courier.email, courier.password);
  const response = await request.post(
    `/api/v1/courier/deliveries/${parcelID}/status`,
    {
      headers: { Authorization: `Bearer ${token}` },
      data: failureReason ? { status, failureReason } : { status },
    },
  );
  expect(response.status(), await response.text()).toBe(200);
}

async function confirmPickup(request: APIRequestContext, parcelID: string) {
  const courier = credentialsForRun('COURIER_1');
  if (!courier) throw new Error('COURIER_1 is required');
  const token = await apiLogin(request, courier.email, courier.password);
  const response = await request.post(
    `/api/v1/courier/deliveries/${parcelID}/pickup`,
    {
      headers: { Authorization: `Bearer ${token}` },
      multipart: {
        photo: {
          name: 'pickup.png',
          mimeType: 'image/png',
          buffer: PNG_PROOF,
        },
      },
    },
  );
  expect(response.status(), await response.text()).toBe(200);
}

async function waitForAttempts(
  request: APIRequestContext,
  merchant: MerchantCase,
  count: number,
): Promise<Attempt[]> {
  let attempts: Attempt[] = [];
  await expect
    .poll(
      async () => {
        const response = await request.get(
          `/api/v1/merchants/me/webhook/deliveries?trackingNumber=${encodeURIComponent(merchant.trackingNumber)}&pageSize=100`,
          { headers: { Authorization: `Bearer ${merchant.token}` } },
        );
        if (!response.ok()) return 0;
        attempts = ((await response.json()) as { data: Attempt[] }).data.sort(
          (a, b) => a.attemptNumber - b.attemptNumber,
        );
        return attempts.length;
      },
      { timeout: EVENT_TIMEOUT, intervals: [500, 1000, 2000] },
    )
    .toBeGreaterThanOrEqual(count);
  return attempts;
}

async function rotateWebhook(
  request: APIRequestContext,
  token: string,
): Promise<string> {
  const response = await request.put('/api/v1/merchants/me/webhook', {
    headers: { Authorization: `Bearer ${token}` },
    data: {
      url: 'https://webhook-sink:8098/hook',
      eventTypes: ['parcel.status_changed'],
      isActive: true,
      rotateSecret: true,
    },
  });
  expect(response.ok(), await response.text()).toBeTruthy();
  const secret = (
    (await response.json()) as { data: { signingSecret?: string } }
  ).data.signingSecret;
  if (!secret) throw new Error('rotation returned no signing secret');
  return secret;
}

function signatureMatches(request: SinkRequest, secret: string): boolean {
  const header = (name: string) => {
    const value = request.headers[name];
    return Array.isArray(value) ? value[0] : value;
  };
  const timestamp = header('x-swiftdrop-timestamp');
  const signature = header('x-swiftdrop-signature');
  if (!timestamp || !signature) return false;
  const expected = `sha256=${createHmac('sha256', secret)
    .update(`${timestamp}.${request.body}`)
    .digest('hex')}`;
  return signature === expected;
}

type Zone = { id: string; code: string; name: string };

async function loadZones(request: APIRequestContext): Promise<Zone[]> {
  const response = await request.get('/api/v1/zones');
  expect(response.ok()).toBeTruthy();
  return ((await response.json()) as { data: Zone[] }).data;
}

async function findMerchantID(
  request: APIRequestContext,
  token: string,
  business: string,
): Promise<string> {
  const response = await request.get(
    `/api/v1/admin/merchants?q=${encodeURIComponent(business)}`,
    { headers: { Authorization: `Bearer ${token}` } },
  );
  expect(response.ok()).toBeTruthy();
  const id = ((await response.json()) as { data: Array<{ id: string }> })
    .data[0]?.id;
  if (!id) throw new Error('merchant was not listed');
  return id;
}

async function findParcelID(
  request: APIRequestContext,
  token: string,
  trackingNumber: string,
): Promise<string> {
  const response = await request.get(
    `/api/v1/admin/parcels?trackingNumber=${encodeURIComponent(trackingNumber)}`,
    { headers: { Authorization: `Bearer ${token}` } },
  );
  expect(response.ok()).toBeTruthy();
  const id = ((await response.json()) as { data: Array<{ id: string }> })
    .data[0]?.id;
  if (!id) throw new Error('parcel was not listed');
  return id;
}
