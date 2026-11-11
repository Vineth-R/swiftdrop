import { randomUUID } from 'node:crypto';

import {
  expect,
  test,
  type APIRequestContext,
  type Page,
} from '@playwright/test';

import { apiLogin } from './support/real-stack';

const adminEmail =
  process.env['REAL_STACK_ADMIN_EMAIL'] || 'seed.admin@example.com';
const adminPassword = process.env['REAL_STACK_ADMIN_PASSWORD'] || 'Str0ngPass!';
const rabbitURL =
  process.env['REAL_STACK_RABBITMQ_URL'] || 'http://localhost:15672';
const rabbitUser = process.env['RABBITMQ_DEFAULT_USER'] || 'swiftdrop';
const rabbitPass = process.env['RABBITMQ_DEFAULT_PASS'] || 'swiftdrop';

function uniqueEmail(prefix: string) {
  return `${prefix}.${Date.now()}.${Math.floor(Math.random() * 10000)}@example.test`;
}

async function signIn(page: Page, email: string, password: string) {
  await page.goto('/login');
  await page.locator('#login-email').fill(email);
  await page.locator('#login-password').fill(password);
  await page.getByRole('button', { name: 'Log in' }).click();
  await expect(page.getByText('Signed in as')).toBeVisible({ timeout: 20_000 });
}

async function zones(request: APIRequestContext) {
  const response = await request.get('/api/v1/zones');
  expect(response.ok()).toBeTruthy();
  const body = (await response.json()) as {
    data?: Array<{ id: string; code: string }>;
  };
  return body.data ?? [];
}

async function adminToken(request: APIRequestContext) {
  return apiLogin(request, adminEmail, adminPassword);
}

async function findMerchantID(
  request: APIRequestContext,
  admin: string,
  businessName: string,
) {
  const response = await request.get(
    `/api/v1/admin/merchants?q=${encodeURIComponent(businessName)}`,
    { headers: { Authorization: `Bearer ${admin}` } },
  );
  expect(response.ok()).toBeTruthy();
  const body = (await response.json()) as { data?: Array<{ id: string }> };
  const id = body.data?.[0]?.id;
  if (!id) throw new Error('merchant was not found in the admin list');
  return id;
}

async function registerMerchant(
  page: Page,
  request: APIRequestContext,
  email: string,
  businessName: string,
  zoneID: string,
) {
  await page.goto('/register/merchant');
  await page.getByLabel('Full name').fill('Real Stack Merchant');
  await page.locator('#merchant-email').fill(email);
  await page.getByLabel('Phone number').fill('+94771234567');
  await page.locator('#merchant-password').fill('Str0ngPass!');
  await page.getByLabel('Confirm password').fill('Str0ngPass!');
  await page.getByLabel('Business name').fill(businessName);
  await page
    .getByLabel('Business registration number')
    .fill(`PV ${Date.now()}`);
  await page.getByLabel('Business address').fill('88 Galle Road, Colombo 03');
  await page
    .getByLabel('Default pickup address')
    .fill('88 Galle Road, Colombo 03');
  await page.getByLabel('Default pickup zone').selectOption(zoneID);
  await page.getByRole('button', { name: 'Submit application' }).click();
  await expect(page.getByText('Application submitted')).toBeVisible({
    timeout: 20_000,
  });
}

async function approveMerchant(
  request: APIRequestContext,
  admin: string,
  businessName: string,
) {
  const merchantID = await findMerchantID(request, admin, businessName);
  const approved = await request.post(
    `/api/v1/admin/merchants/${merchantID}/approve`,
    { headers: { Authorization: `Bearer ${admin}` } },
  );
  expect(approved.ok()).toBeTruthy();
  return merchantID;
}

async function createKey(page: Page) {
  await page.goto('/api-keys');
  await page.getByRole('button', { name: 'Create key' }).click();
  await page.getByLabel('Key name').fill('Real stack orders');
  await page.getByRole('button', { name: 'Create key' }).last().click();
  const secret = page.locator('code.select-all').first();
  await expect(secret).toBeVisible({ timeout: 20_000 });
  return (await secret.innerText()).trim();
}

async function publishStatusEvent(
  request: APIRequestContext,
  envelope: Record<string, unknown>,
) {
  const response = await request.post(
    `${rabbitURL}/api/exchanges/%2F/swiftdrop.events/publish`,
    {
      headers: {
        Authorization: `Basic ${Buffer.from(`${rabbitUser}:${rabbitPass}`).toString('base64')}`,
      },
      data: {
        properties: { content_type: 'application/json' },
        routing_key: 'parcel.status_changed.v1',
        payload: JSON.stringify(envelope),
        payload_encoding: 'string',
      },
    },
  );
  expect(response.ok()).toBeTruthy();
}

test.describe('real stack merchant orders', () => {
  test.setTimeout(180_000);

  test('a merchant creates an order, receives a status sync and is isolated from another merchant', async ({
    page,
    request,
  }) => {
    const allZones = await zones(request);
    const colombo = allZones.find((zone) => zone.code === 'CMB');
    const kandy = allZones.find((zone) => zone.code === 'KDY');
    if (!colombo || !kandy) {
      throw new Error(
        'CMB and KDY zones are required for the merchant order test',
      );
    }

    const email = uniqueEmail('merchant-orders');
    const business = `Real Orders ${Date.now()}`;
    await registerMerchant(page, request, email, business, colombo.id);
    const admin = await adminToken(request);
    await approveMerchant(request, admin, business);

    await signIn(page, email, 'Str0ngPass!');
    const apiKey = await createKey(page);

    const externalOrderID = `SZ-${Date.now()}`;
    const createdResponse = await request.post('/api/v1/merchant/orders', {
      headers: { 'X-API-Key': apiKey },
      data: {
        externalOrderId: externalOrderID,
        recipient: {
          name: 'Amali Fernando',
          phone: '+94712345678',
          address: '45 Lake Drive',
          city: 'Kandy',
          zoneId: kandy.id,
        },
        weightKg: 1.5,
        sizeCategory: 'SMALL',
        description: 'Phone case',
      },
    });
    expect(createdResponse.status()).toBe(201);
    const created = (await createdResponse.json()) as {
      data?: {
        orderId?: string;
        trackingNumber?: string;
        status?: string;
        fee?: { total?: number };
      };
    };
    const orderID = created.data?.orderId;
    const trackingNumber = created.data?.trackingNumber;
    expect(orderID).toBeTruthy();
    expect(trackingNumber).toMatch(/^SDN/);
    expect(created.data?.status).toBe('CREATED');
    expect(created.data?.fee?.total).toBeGreaterThan(0);

    await page.goto('/merchant/orders');
    await expect(page.getByText(externalOrderID)).toBeVisible({
      timeout: 20_000,
    });
    await expect(page.getByText(trackingNumber!)).toBeVisible();
    await page.getByRole('link', { name: externalOrderID }).click();
    await expect(
      page.getByRole('heading', { level: 1, name: 'Order details' }),
    ).toBeVisible();
    await expect(page.getByText('CREATED').first()).toBeVisible();

    const me = await page.request.get('/api/v1/merchants/me');
    expect(me.ok()).toBeTruthy();
    const merchant = (await me.json()) as { data?: { id?: string } };
    const merchantID = merchant.data?.id;
    expect(merchantID).toBeTruthy();

    const search = await request.get(
      `/api/v1/admin/parcels?trackingNumber=${encodeURIComponent(trackingNumber!)}`,
      { headers: { Authorization: `Bearer ${admin}` } },
    );
    expect(search.ok()).toBeTruthy();
    const parcelBody = (await search.json()) as {
      data?: Array<{ id: string }>;
    };
    const parcelID = parcelBody.data?.[0]?.id;
    expect(parcelID).toBeTruthy();

    await publishStatusEvent(request, {
      event_id: randomUUID(),
      event_type: 'parcel.status_changed.v1',
      event_version: 1,
      occurred_at: new Date().toISOString(),
      producer: 'parcel',
      correlation_id: `e2e-${orderID}`,
      data: {
        parcel_id: parcelID,
        tracking_number: trackingNumber,
        previous_status: 'CREATED',
        status: 'IN_TRANSIT',
        merchant_id: merchantID,
      },
    });

    await expect
      .poll(
        async () => {
          const response = await page.request.get(
            `/api/v1/merchant/orders/${orderID}`,
          );
          if (!response.ok()) return '';
          const body = (await response.json()) as {
            data?: { status?: string };
          };
          return body.data?.status ?? '';
        },
        { timeout: 30_000 },
      )
      .toBe('IN_TRANSIT');

    await page.reload();
    await expect(page.getByText('IN TRANSIT').first()).toBeVisible();

    await page.goto('/dashboard');
    await expect(page.getByText(externalOrderID)).toBeVisible({
      timeout: 30_000,
    });
    await expect(page.getByText('IN TRANSIT').first()).toBeVisible({
      timeout: 30_000,
    });

    await page.goto('/merchant/orders');
    await expect(page.getByText(externalOrderID)).toBeVisible({
      timeout: 30_000,
    });
    await expect(page.getByText('IN TRANSIT').first()).toBeVisible({
      timeout: 30_000,
    });

    const otherEmail = uniqueEmail('merchant-orders-other');
    const otherBusiness = `Other Orders ${Date.now()}`;
    const otherRegistration = await request.post('/api/v1/merchants/register', {
      data: {
        fullName: 'Other Merchant',
        email: otherEmail,
        phone: '+94771234567',
        password: 'Str0ngPass!',
        business: {
          name: otherBusiness,
          registrationNumber: `PV ${Date.now()} O`,
          contactEmail: otherEmail,
          contactPhone: '+94771234567',
          businessAddress: '1 Other Road',
          pickupAddress: '1 Other Road',
          pickupZoneId: colombo.id,
        },
      },
    });
    expect(otherRegistration.status()).toBe(201);
    await approveMerchant(request, admin, otherBusiness);
    const otherToken = await apiLogin(request, otherEmail, 'Str0ngPass!');
    const forbidden = await request.get(`/api/v1/merchant/orders/${orderID}`, {
      headers: { Authorization: `Bearer ${otherToken}` },
    });
    expect(forbidden.status()).toBe(404);
  });
});
