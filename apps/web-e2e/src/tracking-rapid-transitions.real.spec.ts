import { randomUUID } from 'node:crypto';

import { expect, test, type APIRequestContext } from '@playwright/test';

import {
  apiLogin,
  apiUserId,
  createParcel,
  credentialsForRun,
} from './support/real-stack';

const REAL_TIMEOUT = 20_000;

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

  for (const [status, extra] of [
    ['PICKED_UP', {}],
    ['IN_TRANSIT', {}],
    ['OUT_FOR_DELIVERY', {}],
    ['DELIVERED', { recipientName: 'Rapid Recipient' }],
  ] as const) {
    await updateStatus(request, courierToken, parcel.id, status, extra);
  }

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
  extra: Record<string, string>,
) {
  const response = await request.post(
    `/api/v1/courier/deliveries/${parcelID}/status`,
    {
      headers: { Authorization: `Bearer ${token}` },
      data: { status, ...extra },
    },
  );
  expect(response.status(), await response.text()).toBe(200);
}
