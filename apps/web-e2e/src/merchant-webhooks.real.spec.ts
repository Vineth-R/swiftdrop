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

// A public HTTPS address. The test only needs the attempt to be recorded; the
// HTTP outcome may be 200, a refusal or a timeout. The endpoint is never used
// for any other purpose.
const webhookURL = `https://example.com/swiftdrop-webhook-${Date.now()}`;
const testTrackingNumber = 'SDNTEST0001';

function uniqueEmail(prefix: string) {
  return `${prefix}.${Date.now()}.${Math.floor(Math.random() * 10000)}@example.test`;
}

async function zones(request: APIRequestContext) {
  const response = await request.get('/api/v1/zones');
  expect(response.ok()).toBeTruthy();
  const body = (await response.json()) as {
    data?: Array<{ id: string; code: string }>;
  };
  return body.data ?? [];
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
  email: string,
  businessName: string,
  zoneID: string,
) {
  await page.goto('/register/merchant');
  await page.getByLabel('Full name').fill('Webhook Merchant');
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

async function registerMerchantByAPI(
  request: APIRequestContext,
  email: string,
  businessName: string,
  zoneID: string,
) {
  const response = await request.post('/api/v1/merchants/register', {
    data: {
      fullName: 'Other Webhook Merchant',
      email,
      phone: '+94771234567',
      password: 'Str0ngPass!',
      business: {
        name: businessName,
        registrationNumber: `PV ${Date.now()} W`,
        contactEmail: email,
        contactPhone: '+94771234567',
        businessAddress: '1 Other Road',
        pickupAddress: '1 Other Road',
        pickupZoneId: zoneID,
      },
    },
  });
  expect(response.status()).toBe(201);
}

test.describe('real stack merchant webhooks', () => {
  test.setTimeout(240_000);

  test('saves, rotates, tests and scopes the merchant webhook', async ({
    page,
    request,
  }) => {
    const allZones = await zones(request);
    const colombo = allZones.find((zone) => zone.code === 'CMB');
    if (!colombo) throw new Error('CMB zone is required for the webhook test');

    const admin = await apiLogin(request, adminEmail, adminPassword);
    const email = uniqueEmail('merchant-webhooks');
    const business = `Webhook Shop ${Date.now()}`;
    await registerMerchant(page, email, business, colombo.id);
    await approveMerchant(request, admin, business);

    const merchantToken = await apiLogin(request, email, 'Str0ngPass!');
    await page.context().addCookies([
      {
        name: 'sd_access',
        value: merchantToken,
        domain: 'localhost',
        path: '/',
        httpOnly: false,
        secure: false,
        sameSite: 'Lax',
      },
    ]);
    await page.goto('/webhook');
    await expect(
      page.getByRole('heading', { level: 1, name: 'Webhook' }),
    ).toBeVisible({ timeout: 20_000 });

    const address = page.locator('#webhook-url');
    const save = page.getByRole('button', { name: /^Save webhook$/ });
    await address.fill(webhookURL);
    await save.click();

    const secret = page.locator('code.select-all').first();
    await expect(secret).toBeVisible({ timeout: 20_000 });
    const firstSecret = (await secret.innerText()).trim();
    expect(firstSecret).toMatch(/^whsec_/);
    await page
      .getByRole('button', { name: 'I have saved this secret' })
      .click();
    await expect(secret).toHaveCount(0);

    // Rotation is the only other time the API returns a plaintext secret.
    await page.getByLabel('Generate a new signing secret').check();
    await save.click();
    const rotated = page.locator('code.select-all').first();
    await expect(rotated).toBeVisible({ timeout: 20_000 });
    const rotatedSecret = (await rotated.innerText()).trim();
    expect(rotatedSecret).toMatch(/^whsec_/);
    expect(rotatedSecret).not.toBe(firstSecret);
    await page
      .getByRole('button', { name: 'I have saved this secret' })
      .click();
    await expect(rotated).toHaveCount(0);

    await page.getByRole('button', { name: 'Send test event' }).click();
    await expect(
      page.getByText(
        /Test event delivered|Your server answered HTTP|No answer from your server/,
      ),
    ).toBeVisible({ timeout: 30_000 });

    // The test send is recorded by the merchant service and appears in the
    // delivery log, not only in the immediate result banner.
    await page.reload();
    await expect(page.getByText(testTrackingNumber).first()).toBeVisible({
      timeout: 30_000,
    });

    await page.getByLabel('Tracking number').fill(testTrackingNumber);
    await page.getByRole('button', { name: 'Apply filters' }).click();
    await expect(page.getByText(testTrackingNumber).first()).toBeVisible();
    await page.getByRole('button', { name: 'Clear' }).click();

    // A different approved merchant has its own empty webhook state and cannot
    // read the first merchant's delivery log.
    const otherEmail = uniqueEmail('merchant-webhooks-other');
    const otherBusiness = `Other Webhook Shop ${Date.now()}`;
    await registerMerchantByAPI(request, otherEmail, otherBusiness, colombo.id);
    await approveMerchant(request, admin, otherBusiness);
    const otherToken = await apiLogin(request, otherEmail, 'Str0ngPass!');
    const otherWebhook = await request.get('/api/v1/merchants/me/webhook', {
      headers: { Authorization: `Bearer ${otherToken}` },
    });
    expect(otherWebhook.status()).toBe(404);
    const otherDeliveries = await request.get(
      '/api/v1/merchants/me/webhook/deliveries',
      { headers: { Authorization: `Bearer ${otherToken}` } },
    );
    expect(otherDeliveries.ok()).toBeTruthy();
    const otherBody = await otherDeliveries.text();
    expect(otherBody).not.toContain(testTrackingNumber);
    expect(otherBody).not.toContain(webhookURL);
  });
});
