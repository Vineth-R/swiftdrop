import { Buffer } from 'node:buffer';
import { randomUUID } from 'node:crypto';

import { expect, test, type APIRequestContext } from '@playwright/test';

import {
  apiLogin,
  apiUserId,
  createParcel,
  credentialsForRun,
} from './support/real-stack';

const REAL_TIMEOUT = 20_000;
const PNG = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAIAAAACCAYAAABytg0kAAAAH0lEQVR4nAASAO3/Av8AAP8AAAAAAAAAAAAAAAAAAwAfFwIBJnyuSgAAAABJRU5ErkJggg==',
  'base64',
);

/**
 * A tracking page is open before one parcel moves through four rapid status
 * transitions. The live messages must keep their sub-second order so the final
 * headline and history agree on DELIVERED.
 */
test('rapid courier transitions leave tracking on the final status', async ({
  page,
  request,
}) => {
  const customer = credentialsForRun('CUSTOMER');
  const dispatcher = credentialsForRun('DISPATCHER');
  const courier = credentialsForRun('COURIER_1');
  test.skip(
    !customer || !dispatcher || !courier,
    'set CUSTOMER, DISPATCHER and COURIER_1 real-stack credentials',
  );
  if (!customer || !dispatcher || !courier) return;

  const customerToken = await apiLogin(
    request,
    customer.email,
    customer.password,
  );
  const parcel = await createParcel(request, customerToken, randomUUID());
  const dispatcherToken = await apiLogin(
    request,
    dispatcher.email,
    dispatcher.password,
  );
  const courierToken = await apiLogin(request, courier.email, courier.password);
  const courierID = await apiUserId(request, courierToken);
  await assign(request, dispatcherToken, parcel.id, courierID);

  await page.goto(`/track?number=${parcel.trackingNumber}`);
  await expect(page.getByText('CREATED', { exact: true })).toBeVisible({
    timeout: REAL_TIMEOUT,
  });
  // The socket must be subscribed before the transitions so this is a real
  // live-reconciliation test, not a page reload.
  await expect(
    page.getByText('Live. Changes appear here as they happen.', {
      exact: true,
    }),
  ).toBeVisible({ timeout: REAL_TIMEOUT });

  // One pickup action records PICKED_UP and IN_TRANSIT in one transaction.
  await confirmPickup(request, courierToken, parcel.id);
  await updateStatus(request, courierToken, parcel.id, 'OUT_FOR_DELIVERY');
  await confirmDelivery(request, courierToken, parcel.id, 'Rapid Recipient');

  await expect(page.getByText('DELIVERED', { exact: true })).toBeVisible({
    timeout: REAL_TIMEOUT,
  });
  const history = page.getByRole('list', { name: 'Status history' });
  await expect(history.locator('[aria-current="step"]')).toContainText(
    'Delivered',
  );
  await expect(history.getByRole('listitem')).toHaveCount(5);
});

async function assign(
  request: APIRequestContext,
  token: string,
  parcelID: string,
  courierID: string,
) {
  const response = await request.put(
    `/api/v1/dispatch/parcels/${parcelID}/assignment`,
    {
      headers: { Authorization: `Bearer ${token}` },
      data: { courierId: courierID, reason: 'Rapid transition test' },
    },
  );
  expect(response.status(), await response.text()).toBe(200);
}

async function updateStatus(
  request: APIRequestContext,
  token: string,
  parcelID: string,
  status: string,
) {
  const response = await request.post(
    `/api/v1/courier/deliveries/${parcelID}/status`,
    {
      headers: { Authorization: `Bearer ${token}` },
      data: { status },
    },
  );
  expect(response.status(), await response.text()).toBe(200);
}

async function confirmPickup(
  request: APIRequestContext,
  token: string,
  parcelID: string,
) {
  const response = await request.post(
    `/api/v1/courier/deliveries/${parcelID}/pickup`,
    {
      headers: { Authorization: `Bearer ${token}` },
      multipart: {
        photo: { name: 'pickup.png', mimeType: 'image/png', buffer: PNG },
      },
    },
  );
  expect(response.status(), await response.text()).toBe(200);
}

async function confirmDelivery(
  request: APIRequestContext,
  token: string,
  parcelID: string,
  recipientName: string,
) {
  const response = await request.post(
    `/api/v1/courier/deliveries/${parcelID}/delivery`,
    {
      headers: { Authorization: `Bearer ${token}` },
      multipart: {
        recipientName,
        photo: { name: 'delivery.png', mimeType: 'image/png', buffer: PNG },
      },
    },
  );
  expect(response.status(), await response.text()).toBe(200);
}
