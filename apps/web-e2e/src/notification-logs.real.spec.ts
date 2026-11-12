import { randomUUID } from 'node:crypto';

import { expect, test } from '@playwright/test';

import { apiLogin, credentialsFor } from './support/real-stack';

type Zone = { id: string; name: string };

const realStackSelected = process.env['REAL_STACK'] === '1';

/**
 * Real stack: a customer parcel triggers the notification worker, and the
 * ADMIN notification-log screen shows the persisted delivery. Run with:
 *
 *   REAL_STACK=1 pnpm exec nx e2e @org/web-e2e --project=real-stack
 *
 * When the suite is selected, missing prerequisites fail loudly instead of
 * silently skipping the scenario.
 */
test.skip(!realStackSelected, 'set REAL_STACK=1 and run --project=real-stack');

function required(account: 'ADMIN' | 'CUSTOMER' | 'DISPATCHER') {
  const credentials = credentialsFor(account);
  if (!credentials) {
    throw new Error(
      `REAL_STACK_${account}_EMAIL and REAL_STACK_${account}_PASSWORD are required when the real-stack suite is selected.`,
    );
  }
  return credentials;
}

async function signIn(
  page: import('@playwright/test').Page,
  email: string,
  password: string,
  home: RegExp,
) {
  await page.goto('/login');
  await page.locator('#login-email').fill(email);
  await page.locator('#login-password').fill(password);
  await page.getByRole('button', { name: 'Log in' }).click();
  await expect(page).toHaveURL(home);
}

test('a real notification appears in the ADMIN log with masked PII', async ({
  page,
  request,
}) => {
  const customer = required('CUSTOMER');
  const admin = required('ADMIN');
  const customerToken = await apiLogin(
    request,
    customer.email,
    customer.password,
  );
  const auth = { Authorization: `Bearer ${customerToken}` };

  const zonesResponse = await request.get('/api/v1/zones');
  expect(zonesResponse.ok()).toBeTruthy();
  const zones = ((await zonesResponse.json()) as { data: Zone[] }).data;
  expect(zones.length).toBeGreaterThan(1);

  let route: { from: Zone; to: Zone; fee: number } | null = null;
  for (const from of zones) {
    for (const to of zones) {
      if (route || from.id === to.id) continue;
      const estimate = await request.post('/api/v1/pricing/estimate', {
        headers: auth,
        data: {
          originZoneId: from.id,
          destinationZoneId: to.id,
          weightKg: 1,
          sizeCategory: 'SMALL',
        },
      });
      if (estimate.ok()) {
        const fee = ((await estimate.json()) as { data: { total: number } })
          .data.total;
        route = { from, to, fee };
      }
    }
  }
  expect(route, 'no priced pair of zones in the seed data').not.toBeNull();
  if (!route) return;

  const contact = (name: string, zone: Zone) => ({
    name,
    phone: '+94771234567',
    address: '12 Test Lane',
    city: zone.name,
    zoneId: zone.id,
  });
  const created = await request.post('/api/v1/parcels', {
    headers: { ...auth, 'Idempotency-Key': randomUUID() },
    data: {
      sender: contact('Notification Sender', route.from),
      recipient: contact('Notification Recipient', route.to),
      weightKg: 1,
      sizeCategory: 'SMALL',
      expectedFee: route.fee,
    },
  });
  expect(created.status(), await created.text()).toBe(201);
  const parcel = (
    (await created.json()) as { data: { id: string; trackingNumber: string } }
  ).data;

  // The worker is asynchronous. Poll the real ADMIN API until the SENT log for
  // this parcel exists, then assert the UI. A timeout is a real failure.
  const adminToken = await apiLogin(request, admin.email, admin.password);
  const adminAuth = { Authorization: `Bearer ${adminToken}` };
  let persisted = false;
  for (let attempt = 0; attempt < 60; attempt += 1) {
    const response = await request.get(
      '/api/v1/admin/notification-logs?status=SENT&pageSize=100',
      { headers: adminAuth },
    );
    expect(response.ok()).toBeTruthy();
    const items = (
      (await response.json()) as {
        data: Array<{ parcelId: string; channel: string }>;
      }
    ).data;
    if (
      items.some(
        (item) => item.parcelId === parcel.id && item.channel === 'EMAIL',
      )
    ) {
      persisted = true;
      break;
    }
    await page.waitForTimeout(1000);
  }
  expect(persisted, 'notification worker did not persist a SENT log').toBe(
    true,
  );

  await signIn(page, admin.email, admin.password, /\/dashboard$/);
  await page.goto('/admin/notification-logs');
  await expect(
    page.getByRole('heading', { name: /notification/i }),
  ).toBeVisible();

  const masked = `${customer.email[0]}***@${customer.email.split('@')[1]}`;
  const row = page.getByRole('row').filter({ hasText: masked }).first();
  await expect(row).toBeVisible({ timeout: 15_000 });
  await expect(row).toContainText('EMAIL');
  await expect(row).toContainText('SENT');
  await expect(row).not.toContainText(customer.email);

  // Created time is rendered in the first cell.
  const createdCell = row.getByRole('cell').first();
  await expect(createdCell).not.toHaveText('');

  await page.getByLabel('Filter by delivery status').click();
  await page.getByRole('option', { name: 'SENT' }).click();
  await expect(row).toBeVisible();

  await page.getByLabel('Filter by delivery status').click();
  await page.getByRole('option', { name: 'FAILED' }).click();
  await expect(row).toHaveCount(0);
});

test('a DISPATCHER cannot open the ADMIN notification log', async ({
  page,
}) => {
  const dispatcher = required('DISPATCHER');
  await signIn(page, dispatcher.email, dispatcher.password, /\/unassigned$/);
  await page.goto('/admin/notification-logs');
  await expect(page.getByText('403 · Forbidden')).toBeVisible();
  await expect(page.getByText('You do not have access')).toBeVisible();
});
