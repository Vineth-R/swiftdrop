import { randomUUID } from 'node:crypto';

import {
  expect,
  test,
  type APIRequestContext,
  type Page,
} from '@playwright/test';

import { apiLogin, apiUserId, credentialsForRun } from './support/real-stack';

const LIVE_TIMEOUT = 20_000;
const EVENT_TIMEOUT = 60_000;

type Zone = { id: string; code: string; name: string };

type MerchantCredentials = { email: string; password: string };

type CreatedOrder = { orderId: string; trackingNumber: string };

type WebhookAttempt = {
  eventId: string;
  trackingNumber: string;
  attemptNumber: number;
};

type NotificationLog = {
  eventId: string;
  parcelId: string;
  channel: string;
  status: string;
};

type RealtimeMessage = {
  type: string;
  data: Record<string, unknown>;
};

/**
 * The group event-flow item (M1, M6, M7, M8, M9) on the real stack: ONE courier
 * status change on a merchant's parcel is checked in every consumer.
 *
 *   1. A fresh merchant is registered, approved, given an API key and a webhook.
 *   2. The merchant creates an order; the dispatcher assigns the parcel.
 *   3. A socket subscribed to the parcel is opened.
 *   4. The courier sets PICKED_UP (the one change).
 *
 * Then it asserts that the same event reached the realtime service, the merchant
 * order snapshot, the webhook worker and the notification worker, and that the
 * webhook attempt and the SENT notification log share the event ID.
 *
 * The webhook target is a public HTTPS placeholder, so the HTTP outcome is not
 * asserted. What is asserted is that the worker processed the event and recorded
 * exactly one first attempt for it.
 *
 * Run with the Compose stack up and the seed applied:
 *
 *   REAL_STACK=1 pnpm exec nx run @org/web-e2e:e2e --args="--project=real-stack"
 *
 * The stack needs WEBHOOK_SECRET_KEY and the parcel status queues wired to
 * parcel.status_changed.v1. A non-development APP_ENV also needs
 * REALTIME_ALLOWED_ORIGINS to include the gateway host.
 */
test.describe('one status change reaches every consumer', () => {
  // Logins share a one-minute rate limit; one retry covers a brief 429.
  test.describe.configure({ retries: 1 });

  test('a courier moves a merchant parcel: socket, order, webhook and email', async ({
    page,
    request,
  }) => {
    test.setTimeout(180_000);
    const admin = credentialsForRun('ADMIN');
    const dispatcher = credentialsForRun('DISPATCHER');
    const courier = credentialsForRun('COURIER_1');
    test.skip(
      !admin || !dispatcher || !courier,
      'set REAL_STACK_ADMIN_*, REAL_STACK_DISPATCHER_* and REAL_STACK_COURIER_1_*',
    );
    if (!admin || !dispatcher || !courier) return;

    const zones = await loadZones(request);
    const colombo = zones.find((zone) => zone.code === 'CMB');
    const kandy = zones.find((zone) => zone.code === 'KDY');
    if (!colombo || !kandy) {
      throw new Error('CMB and KDY zones are required for the event-flow test');
    }

    const adminToken = await apiLogin(request, admin.email, admin.password);
    const merchant = await registerMerchant(request, colombo.id);
    await approveMerchant(request, adminToken, merchant.businessName);
    const merchantToken = await apiLogin(
      request,
      merchant.email,
      merchant.password,
    );
    const apiKey = await createApiKey(request, merchantToken);
    await saveWebhook(request, merchantToken);

    const order = await createOrder(request, apiKey, kandy.id);
    const parcelId = await findParcelId(
      request,
      adminToken,
      order.trackingNumber,
    );

    const dispatcherToken = await apiLogin(
      request,
      dispatcher.email,
      dispatcher.password,
    );
    const courierToken = await apiLogin(
      request,
      courier.email,
      courier.password,
    );
    const courierId = await apiUserId(request, courierToken);
    await assignParcel(request, dispatcherToken, parcelId, courierId);

    // Subscribe before the one status change so the event cannot be missed.
    await openParcelSocket(page, order.trackingNumber);
    await expect
      .poll(() => page.evaluate<boolean>('window.__eventFlow.subscribed'), {
        timeout: LIVE_TIMEOUT,
      })
      .toBe(true);

    await updateCourierStatus(request, courierToken, parcelId, 'PICKED_UP');

    // Realtime: exactly one update, with only the four public tracking fields.
    await expect
      .poll(() => page.evaluate<number>('window.__eventFlow.messages.length'), {
        timeout: LIVE_TIMEOUT,
      })
      .toBe(1);
    const messages = await page.evaluate<RealtimeMessage[]>(
      'window.__eventFlow.messages',
    );
    expect(messages).toHaveLength(1);
    expect(messages[0].type).toBe('parcel.status.updated');
    expect(Object.keys(messages[0].data).sort()).toEqual([
      'previousStatus',
      'status',
      'trackingNumber',
      'updatedAt',
    ]);
    expect(messages[0].data).toMatchObject({
      trackingNumber: order.trackingNumber,
      previousStatus: 'CREATED',
      status: 'PICKED_UP',
    });
    expect(JSON.stringify(messages[0])).not.toContain(parcelId);

    await page.evaluate('window.__eventFlowSocket.close()');

    // Merchant order snapshot.
    await expect
      .poll(
        async () => (await getOrder(request, apiKey, order.orderId)).status,
        { timeout: EVENT_TIMEOUT },
      )
      .toBe('PICKED_UP');

    // Webhook worker: the first attempt is recorded for the same event.
    const attempt = await waitForWebhookAttempt(
      request,
      merchantToken,
      order.trackingNumber,
    );
    expect(attempt.trackingNumber).toBe(order.trackingNumber);
    expect(attempt.eventId).toMatch(
      /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/,
    );

    // Notification worker: exactly one SENT EMAIL log for the same event.
    const logs = await waitForNotificationLogs(
      request,
      adminToken,
      parcelId,
      attempt.eventId,
    );
    expect(logs).toHaveLength(1);
    expect(logs[0].channel).toBe('EMAIL');
    expect(logs[0].status).toBe('SENT');
    expect(logs[0].eventId).toBe(attempt.eventId);
  });
});

async function loadZones(request: APIRequestContext): Promise<Zone[]> {
  const response = await request.get('/api/v1/zones');
  expect(response.ok()).toBeTruthy();
  return ((await response.json()) as { data: Zone[] }).data;
}

async function registerMerchant(
  request: APIRequestContext,
  zoneId: string,
): Promise<MerchantCredentials & { businessName: string }> {
  const suffix = `${Date.now()}-${Math.floor(Math.random() * 10_000)}`;
  const email = `event-flow.${suffix}@example.test`;
  const businessName = `Event Flow ${suffix}`;
  const response = await request.post('/api/v1/merchants/register', {
    data: {
      fullName: 'Event Flow Merchant',
      email,
      phone: '+94771234567',
      password: 'Str0ngPass!',
      business: {
        name: businessName,
        registrationNumber: `PV ${suffix}`,
        contactEmail: email,
        contactPhone: '+94771234567',
        businessAddress: '88 Galle Road, Colombo 03',
        pickupAddress: '88 Galle Road, Colombo 03',
        pickupZoneId: zoneId,
      },
    },
  });
  expect(response.status(), await response.text()).toBe(201);
  return { email, password: 'Str0ngPass!', businessName };
}

async function approveMerchant(
  request: APIRequestContext,
  adminToken: string,
  businessName: string,
): Promise<void> {
  const auth = { Authorization: `Bearer ${adminToken}` };
  const list = await request.get(
    `/api/v1/admin/merchants?q=${encodeURIComponent(businessName)}`,
    { headers: auth },
  );
  expect(list.ok()).toBeTruthy();
  const merchants = ((await list.json()) as { data: Array<{ id: string }> })
    .data;
  const merchantId = merchants[0]?.id;
  if (!merchantId) throw new Error('the new merchant was not listed');
  const approved = await request.post(
    `/api/v1/admin/merchants/${merchantId}/approve`,
    { headers: auth },
  );
  expect(approved.ok()).toBeTruthy();
}

async function createApiKey(
  request: APIRequestContext,
  merchantToken: string,
): Promise<string> {
  const response = await request.post('/api/v1/merchants/me/keys', {
    headers: { Authorization: `Bearer ${merchantToken}` },
    data: { name: 'Event flow key' },
  });
  expect(response.status(), await response.text()).toBe(201);
  return ((await response.json()) as { data: { apiKey: string } }).data.apiKey;
}

async function saveWebhook(
  request: APIRequestContext,
  merchantToken: string,
): Promise<void> {
  const response = await request.put('/api/v1/merchants/me/webhook', {
    headers: { Authorization: `Bearer ${merchantToken}` },
    data: {
      url: `https://example.com/swiftdrop-event-flow-${randomUUID()}`,
      eventTypes: ['parcel.status_changed'],
      isActive: true,
    },
  });
  expect(response.ok(), await response.text()).toBeTruthy();
}

async function createOrder(
  request: APIRequestContext,
  apiKey: string,
  zoneId: string,
): Promise<CreatedOrder> {
  const response = await request.post('/api/v1/merchant/orders', {
    headers: { 'X-API-Key': apiKey, 'Idempotency-Key': randomUUID() },
    data: {
      externalOrderId: `EF-${randomUUID()}`,
      recipient: {
        name: 'Event Flow Recipient',
        phone: '+94712345678',
        address: '45 Lake Drive',
        city: 'Kandy',
        zoneId,
      },
      weightKg: 1,
      sizeCategory: 'SMALL',
      description: 'Event flow parcel',
    },
  });
  expect(response.status(), await response.text()).toBe(201);
  const order = ((await response.json()) as { data: CreatedOrder }).data;
  expect(order.orderId).toBeTruthy();
  expect(order.trackingNumber).toMatch(/^SDN/);
  return order;
}

async function findParcelId(
  request: APIRequestContext,
  adminToken: string,
  trackingNumber: string,
): Promise<string> {
  const response = await request.get(
    `/api/v1/admin/parcels?trackingNumber=${encodeURIComponent(trackingNumber)}`,
    { headers: { Authorization: `Bearer ${adminToken}` } },
  );
  expect(response.ok()).toBeTruthy();
  const parcels = ((await response.json()) as { data: Array<{ id: string }> })
    .data;
  const parcelId = parcels[0]?.id;
  if (!parcelId) throw new Error('the new parcel was not found');
  return parcelId;
}

async function assignParcel(
  request: APIRequestContext,
  dispatcherToken: string,
  parcelId: string,
  courierId: string,
): Promise<void> {
  const response = await request.put(
    `/api/v1/dispatch/parcels/${parcelId}/assignment`,
    {
      headers: { Authorization: `Bearer ${dispatcherToken}` },
      data: { courierId, reason: 'Event-flow test assignment' },
    },
  );
  expect(response.status(), await response.text()).toBe(200);
}

async function updateCourierStatus(
  request: APIRequestContext,
  courierToken: string,
  parcelId: string,
  status: string,
): Promise<void> {
  const response = await request.post(
    `/api/v1/courier/deliveries/${parcelId}/status`,
    {
      headers: { Authorization: `Bearer ${courierToken}` },
      data: { status },
    },
  );
  expect(response.status(), await response.text()).toBe(200);
}

async function getOrder(
  request: APIRequestContext,
  apiKey: string,
  orderId: string,
): Promise<{ status: string }> {
  const response = await request.get(`/api/v1/merchant/orders/${orderId}`, {
    headers: { 'X-API-Key': apiKey },
  });
  expect(response.ok()).toBeTruthy();
  return ((await response.json()) as { data: { status: string } }).data;
}

async function waitForWebhookAttempt(
  request: APIRequestContext,
  merchantToken: string,
  trackingNumber: string,
): Promise<WebhookAttempt> {
  let last: WebhookAttempt | undefined;
  await expect
    .poll(
      async () => {
        const response = await request.get(
          `/api/v1/merchants/me/webhook/deliveries?trackingNumber=${encodeURIComponent(trackingNumber)}&pageSize=100`,
          { headers: { Authorization: `Bearer ${merchantToken}` } },
        );
        if (!response.ok()) return false;
        const attempts = ((await response.json()) as { data: WebhookAttempt[] })
          .data;
        last = attempts.find((attempt) => attempt.attemptNumber === 1);
        return Boolean(last);
      },
      { timeout: EVENT_TIMEOUT, intervals: [500, 1000, 2000] },
    )
    .toBe(true);
  if (!last) throw new Error('the webhook attempt was not recorded');
  return last;
}

async function waitForNotificationLogs(
  request: APIRequestContext,
  adminToken: string,
  parcelId: string,
  eventId: string,
): Promise<NotificationLog[]> {
  let matches: NotificationLog[] = [];
  await expect
    .poll(
      async () => {
        const response = await request.get(
          '/api/v1/admin/notification-logs?status=SENT&pageSize=100',
          { headers: { Authorization: `Bearer ${adminToken}` } },
        );
        if (!response.ok()) return false;
        const logs = ((await response.json()) as { data: NotificationLog[] })
          .data;
        matches = logs.filter(
          (log) => log.parcelId === parcelId && log.eventId === eventId,
        );
        return matches.length > 0;
      },
      { timeout: EVENT_TIMEOUT, intervals: [500, 1000, 2000] },
    )
    .toBe(true);
  return matches;
}

/**
 * Opens an anonymous socket (single-parcel subscriptions are public, as on the
 * tracking page) and records incoming messages on `window.__eventFlow`.
 * The e2e project has no DOM types, so the page code is passed as a string.
 */
async function openParcelSocket(
  page: Page,
  trackingNumber: string,
): Promise<void> {
  await page.goto('/track');
  await page.evaluate(`
    (() => {
      const scheme = location.protocol === 'https:' ? 'wss:' : 'ws:';
      const socket = new WebSocket(scheme + '//' + location.host + '/ws');
      const state = { subscribed: false, messages: [], closed: false };
      window.__eventFlow = state;
      socket.onopen = () => socket.send(JSON.stringify({
        type: 'subscribe',
        trackingNumber: ${JSON.stringify(trackingNumber)},
      }));
      socket.onmessage = (event) => {
        const message = JSON.parse(event.data);
        if (message.type === 'subscribed') state.subscribed = true;
        if (message.type === 'parcel.status.updated') state.messages.push(message);
      };
      socket.onclose = () => { state.closed = true; };
      window.__eventFlowSocket = socket;
    })();
  `);
}
