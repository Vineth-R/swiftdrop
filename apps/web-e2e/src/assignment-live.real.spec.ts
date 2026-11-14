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
  type Credentials,
} from './support/real-stack';

const LIVE_TIMEOUT = 20_000;

/**
 * Real stack: while a dispatcher has All parcels open and filtered to a parcel,
 * the parcel is assigned through the API. The dispatcher page receives the
 * parcel.assigned.updated notice on its scope-all socket, refetches the
 * authoritative REST state, and the row's action changes from Assign to
 * Reassign without a reload.
 */
test.describe('assignment reaches the dispatcher live', () => {
  test.describe.configure({ retries: 1 });

  test('an assignment updates the dispatcher row without reloading', async ({
    page,
    request,
  }) => {
    test.setTimeout(120_000);
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

    const parcel = await createParcel(request, customerToken, randomUUID());
    const courierId = await apiUserId(request, courierToken);

    await signIn(page, dispatcher);
    await page.goto('/parcels');
    await expect(
      page.getByText('Live. Parcel statuses change here as they happen.', {
        exact: true,
      }),
    ).toBeVisible({ timeout: LIVE_TIMEOUT });

    await page.locator('#search-tracking-number').fill(parcel.trackingNumber);
    const table = page.getByRole('table', { name: 'All parcels' });
    const row = table
      .getByRole('row')
      .filter({ hasText: parcel.trackingNumber });
    await expect(row).toBeVisible({ timeout: LIVE_TIMEOUT });
    await expect(
      row.getByRole('button', { name: `Assign courier to parcel ${parcel.trackingNumber}` }),
    ).toBeVisible();

    // A full navigation would reset this marker; a client-side refetch keeps it.
    const mark = await markPage(page);

    await assignParcel(request, dispatcherToken, parcel.id, courierId);

    await expect(
      row.getByRole('button', {
        name: `Reassign parcel ${parcel.trackingNumber}`,
      }),
    ).toBeVisible({ timeout: LIVE_TIMEOUT });
    expect(await pageMark(page)).toBe(mark);
  });
});

async function signIn(page: Page, credentials: Credentials): Promise<void> {
  await signInThroughForm(page, credentials.email, credentials.password);
  await expect(page.locator('#login-email')).toHaveCount(0);
}

async function assignParcel(
  request: APIRequestContext,
  token: string,
  parcelId: string,
  courierId: string,
): Promise<void> {
  const response = await request.put(
    `/api/v1/dispatch/parcels/${parcelId}/assignment`,
    {
      headers: { Authorization: `Bearer ${token}` },
      data: { courierId, reason: 'Assignment live test' },
    },
  );
  expect(response.status(), await response.text()).toBe(200);
}

async function markPage(page: Page): Promise<string> {
  const mark = randomUUID();
  await page.evaluate((value) => {
    (window as unknown as { __mark: string }).__mark = value;
  }, mark);
  return mark;
}

async function pageMark(page: Page): Promise<string | undefined> {
  return page.evaluate(
    () => (window as unknown as { __mark?: string }).__mark,
  );
}
