import { Buffer } from 'node:buffer';
import { randomUUID } from 'node:crypto';

import {
  expect,
  test,
  type APIRequestContext,
  type Page,
} from '@playwright/test';

import {
  apiLogin,
  apiUserId,
  createParcel,
  credentialsForRun,
  signInThroughForm,
  type CreatedParcel,
  type Credentials,
} from './support/real-stack';

const REAL_TIMEOUT = 20_000;

// A 1x1 transparent PNG. The proof endpoint validates decoded image content, so
// the test does not need a binary fixture in the repository.
const PNG = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAIAAAACCAYAAABytg0kAAAAH0lEQVR4nAASAO3/Av8AAP8AAAAAAAAAAAAAAAAAAwAfFwIBJnyuSgAAAABJRU5ErkJggg==',
  'base64',
);

test.describe('real-stack courier completion', () => {
  test.setTimeout(180_000);

  test('a courier delivers with proof and records a failed delivery', async ({
    page,
    request,
  }) => {
    const customer = credentialsForRun('CUSTOMER');
    const dispatcher = credentialsForRun('DISPATCHER');
    const courier = credentialsForRun('COURIER_1');
    test.skip(
      !customer || !dispatcher || !courier,
      'set REAL_STACK_CUSTOMER_*, REAL_STACK_DISPATCHER_* and REAL_STACK_COURIER_1_*',
    );
    if (!customer || !dispatcher || !courier) return;

    const customerToken = await apiLogin(
      request,
      customer.email,
      customer.password,
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
    const courierID = await apiUserId(request, courierToken);
    const delivered = await createParcel(request, customerToken, randomUUID());
    const failed = await createParcel(request, customerToken, randomUUID());
    await assignParcel(request, dispatcherToken, delivered.id, courierID);
    await assignParcel(request, dispatcherToken, failed.id, courierID);

    await signIn(page, courier);

    await openDelivery(page, delivered.trackingNumber);
    await confirmPickup(page);
    await markOutForDelivery(page);

    await page.getByLabel('Recipient name').fill('Amali Fernando');
    await page.getByLabel('Delivery proof photo').setInputFiles({
      name: 'proof.png',
      mimeType: 'image/png',
      buffer: PNG,
    });
    await page.getByLabel('Delivery note').fill('Handed to recipient');
    await page.getByRole('button', { name: 'Confirm delivery' }).click();
    await page
      .getByRole('dialog', { name: 'Confirm delivery?' })
      .getByRole('button', { name: 'Confirm delivery' })
      .click();
    await expect(
      page.getByText('Delivery confirmed. The parcel is now delivered.'),
    ).toBeVisible({ timeout: REAL_TIMEOUT });
    await expect(
      page.getByRole('heading', { level: 2, name: 'Delivered', exact: true }),
    ).toBeVisible({ timeout: REAL_TIMEOUT });
    await expect(page.getByText('Handed to recipient').first()).toBeVisible();

    // Activity lists pickup first and delivery second; open the delivery photo.
    const popupPromise = page.waitForEvent('popup');
    await page.getByRole('button', { name: 'View proof photo' }).last().click();
    const popup = await popupPromise;
    await popup.waitForURL(/proofs\/[^/]+\/photo(?:\/content)?/, {
      timeout: REAL_TIMEOUT,
    });
    await popup.close();

    await openDelivery(page, failed.trackingNumber);
    await confirmPickup(page);
    await markOutForDelivery(page);
    await page.getByRole('button', { name: 'Report delivery failure' }).click();
    await page
      .getByLabel('Failure reason')
      .fill('Recipient was unavailable at the address');
    await page
      .getByRole('dialog', { name: 'Report a failed delivery' })
      .getByRole('button', { name: 'Report failure' })
      .click();
    await expect(page.getByText('Delivery failure recorded.')).toBeVisible({
      timeout: REAL_TIMEOUT,
    });
    await expect(
      page.getByRole('heading', {
        level: 2,
        name: 'Delivery failed',
        exact: true,
      }),
    ).toBeVisible({ timeout: REAL_TIMEOUT });
    await expect(
      page.getByText('Recipient was unavailable at the address').first(),
    ).toBeVisible();
  });
});

async function signIn(page: Page, credentials: Credentials) {
  await signInThroughForm(page, credentials.email, credentials.password);
  await expect(page.locator('#login-email')).toHaveCount(0);
}

async function openDelivery(page: Page, trackingNumber: string) {
  await page.goto('/dashboard');
  const card = page.locator('li').filter({ hasText: trackingNumber }).first();
  await expect(card).toBeVisible({ timeout: REAL_TIMEOUT });
  await card.getByRole('link', { name: 'View delivery' }).click();
  await expect(page).toHaveURL(/\/deliveries\/[0-9a-f-]+$/);
}

async function confirmPickup(page: Page) {
  await page.getByLabel('Pickup proof photo').setInputFiles({
    name: 'pickup.png',
    mimeType: 'image/png',
    buffer: PNG,
  });
  await page.getByRole('button', { name: 'Confirm pickup' }).click();
  await expect(
    page.getByText('Pickup confirmed. The parcel is now in transit.'),
  ).toBeVisible({ timeout: REAL_TIMEOUT });
}

async function markOutForDelivery(page: Page) {
  await page.getByRole('button', { name: 'Mark out for delivery' }).click();
  await expect(
    page.getByText('The parcel is now out for delivery.'),
  ).toBeVisible({ timeout: REAL_TIMEOUT });
}

async function assignParcel(
  request: APIRequestContext,
  token: string,
  parcelID: string,
  courierID: string,
): Promise<CreatedParcel> {
  const response = await request.put(
    `/api/v1/dispatch/parcels/${parcelID}/assignment`,
    {
      headers: { Authorization: `Bearer ${token}` },
      data: { courierId: courierID, reason: 'Courier completion test' },
    },
  );
  expect(response.status(), await response.text()).toBe(200);
  return { id: parcelID, trackingNumber: '' };
}
