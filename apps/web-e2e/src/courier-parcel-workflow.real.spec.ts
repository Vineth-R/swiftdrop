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
  type Credentials,
} from './support/real-stack';

const LIVE_TIMEOUT = 20_000;

type CourierStatus =
  'PICKED_UP' | 'IN_TRANSIT' | 'OUT_FOR_DELIVERY' | 'DELIVERED' | 'FAILED';

/**
 * Real stack: a customer's parcel is assigned to the first courier. The courier
 * moves it to Picked up on their own screen, and the change appears on the
 * public tracking page and on the dispatcher's All parcels page through a live
 * update, without a reload.
 *
 * Run with the Compose stack up and the seed applied:
 *
 *   REAL_STACK=1 pnpm exec nx e2e @org/web-e2e --project=real-stack
 *
 * This file needs both live-update changes deployed: /track (#139) and
 * /parcels (#140). See support/real-stack.ts for the REAL_STACK_* variables.
 */
test.describe('courier parcel workflow on the real stack', () => {
  // Logins share a one-minute rate limit; one retry covers a brief 429.
  test.describe.configure({ retries: 1 });

  test('a courier marks a parcel picked up and /track follows live', async ({
    browser,
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
    await assignParcel(request, dispatcherToken, parcel.id, courierId);

    // A signed-out context: this is the page a customer shares.
    const publicContext = await browser.newContext({
      baseURL: test.info().project.use.baseURL,
    });
    const trackingPage = await publicContext.newPage();
    try {
      await trackingPage.goto(`/track?number=${parcel.trackingNumber}`);
      await expect(
        trackingPage.getByText('Live. Changes appear here as they happen.', {
          exact: true,
        }),
      ).toBeVisible();
      await expect(
        trackingPage.getByText('CREATED', { exact: true }),
      ).toBeVisible();
      await expect(currentStep(trackingPage)).toContainText('Created');
      const mark = await markPage(trackingPage);

      // The courier signs in, finds the new assignment on Today, and moves it.
      await signIn(page, courier);
      const card = page
        .locator('li')
        .filter({ hasText: parcel.trackingNumber });
      await expect(card).toBeVisible({ timeout: LIVE_TIMEOUT });
      await card.getByRole('link', { name: 'View delivery' }).click();
      await expect(page).toHaveURL(new RegExp(`/deliveries/${parcel.id}$`));
      const deliveryStatus = page.getByRole('region', {
        name: 'Delivery status',
      });
      await expect(
        deliveryStatus.getByText('CREATED', { exact: true }),
      ).toBeVisible({ timeout: LIVE_TIMEOUT });
      await page.getByLabel('Next status').selectOption('PICKED_UP');
      await page.getByRole('button', { name: 'Save status' }).click();
      await expect(
        page.getByText('Delivery status saved', { exact: true }),
      ).toBeVisible({ timeout: LIVE_TIMEOUT });

      // No Refresh and no reload: /track heard the change.
      await expect(
        trackingPage.getByText('PICKED UP', { exact: true }),
      ).toBeVisible({ timeout: LIVE_TIMEOUT });
      await expect(currentStep(trackingPage)).toContainText('Picked up');
      expect(await pageMark(trackingPage)).toBe(mark);
    } finally {
      await publicContext.close();
    }
  });

  test('the dispatcher sees the change on All parcels without reloading', async ({
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
    await assignParcel(request, dispatcherToken, parcel.id, courierId);

    await signIn(page, dispatcher);
    await page.goto('/parcels');
    await expect(
      page.getByRole('heading', { name: 'All parcels' }),
    ).toBeVisible();
    // The caption appears only once the dispatcher's socket is live.
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
    await expect(row.getByText('CREATED', { exact: true })).toBeVisible();
    const mark = await markPage(page);

    // The courier moves the parcel on through the API (this test is about the
    // dispatcher's page); the dispatcher page is not touched.
    await updateCourierStatus(request, courierToken, parcel.id, 'PICKED_UP');

    await expect(row.getByText('PICKED UP', { exact: true })).toBeVisible({
      timeout: LIVE_TIMEOUT,
    });
    expect(await pageMark(page)).toBe(mark);
  });
});

async function signIn(page: Page, credentials: Credentials): Promise<void> {
  await page.goto('/login');
  await page.locator('#login-email').fill(credentials.email);
  await page.locator('#login-password').fill(credentials.password);
  await page.getByRole('button', { name: 'Log in' }).click();
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
      data: { courierId, reason: 'Real-stack courier workflow test' },
    },
  );
  expect(response.status(), await response.text()).toBe(200);
}

async function updateCourierStatus(
  request: APIRequestContext,
  token: string,
  parcelId: string,
  status: CourierStatus,
): Promise<void> {
  const response = await request.post(
    `/api/v1/courier/deliveries/${parcelId}/status`,
    {
      headers: { Authorization: `Bearer ${token}` },
      data: { status },
    },
  );
  expect(response.status(), await response.text()).toBe(200);
}

/** The reached step the tracking history marks as the parcel's current one. */
const currentStep = (page: Page) =>
  page
    .getByRole('list', { name: 'Status history' })
    .locator('[aria-current="step"]');

/**
 * Marks the loaded document. A browser reload drops the mark, so finding it
 * again after an update proves the page itself was never reloaded.
 * The e2e project has no DOM types, so the page code is passed as a string.
 */
async function markPage(page: Page): Promise<string> {
  const mark = randomUUID();
  await page.evaluate(`window.__e2ePageMark = ${JSON.stringify(mark)}`);
  return mark;
}

function pageMark(page: Page): Promise<string> {
  return page.evaluate<string>('window.__e2ePageMark');
}
