import { Buffer } from 'node:buffer';

import { expect, test, type Page } from '@playwright/test';

import {
  apiLogin,
  apiUserId,
  credentialsForRun,
  signInThroughForm,
  type Credentials,
} from './support/real-stack';

const REAL_TIMEOUT = 20_000;
const EVENT_TIMEOUT = 60_000;
const PNG = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAIAAAACCAYAAABytg0kAAAAH0lEQVR4nAASAO3/Av8AAP8AAAAAAAAAAAAAAAAAAwAfFwIBJnyuSgAAAABJRU5ErkJggg==',
  'base64',
);

/**
 * The Phase 4 acceptance journey. Every role uses its own browser context and
 * the real UI: customer booking, dispatcher assignment, courier completion
 * with proof, then customer tracking, dispatcher state and notification logs.
 */
test('customer, dispatcher and courier complete one delivery through the real UI', async ({
  browser,
  request,
}) => {
  test.setTimeout(240_000);
  const customer = credentialsForRun('CUSTOMER');
  const dispatcher = credentialsForRun('DISPATCHER');
  const courier = credentialsForRun('COURIER_1');
  const admin = credentialsForRun('ADMIN');
  test.skip(
    !customer || !dispatcher || !courier || !admin,
    'set REAL_STACK_CUSTOMER_*, REAL_STACK_DISPATCHER_*, REAL_STACK_COURIER_1_* and REAL_STACK_ADMIN_*',
  );
  if (!customer || !dispatcher || !courier || !admin) return;

  const baseURL = test.info().project.use.baseURL;
  const customerContext = await browser.newContext({ baseURL });
  const dispatcherContext = await browser.newContext({ baseURL });
  const courierContext = await browser.newContext({ baseURL });
  try {
    const customerPage = await customerContext.newPage();
    const dispatcherPage = await dispatcherContext.newPage();
    const courierPage = await courierContext.newPage();

    const courierToken = await apiLogin(
      request,
      courier.email,
      courier.password,
    );
    const courierID = await apiUserId(request, courierToken);

    const parcel = await createThroughWizard(customerPage, customer);
    await assignThroughUI(dispatcherPage, dispatcher, parcel, courierID);
    await customerPage.goto(`/track?number=${parcel.trackingNumber}`);
    await expect(
      customerPage.getByText('CREATED', { exact: true }),
    ).toBeVisible({ timeout: REAL_TIMEOUT });

    await completeThroughCourierUI(courierPage, courier, parcel);

    // Re-open the public tracking page after the terminal update. The dedicated
    // courier workflow test covers the live single-transition path; this journey
    // verifies the authoritative final projection for the whole workflow.
    await customerPage.goto(`/track?number=${parcel.trackingNumber}`);
    await expect(
      customerPage.getByText('DELIVERED', { exact: true }),
    ).toBeVisible({ timeout: REAL_TIMEOUT });

    await customerPage.goto(`/parcels/${parcel.parcelID}`);
    await expect(
      customerPage.getByText('DELIVERED', { exact: true }).first(),
    ).toBeVisible({ timeout: REAL_TIMEOUT });
    await expect(
      customerPage
        .getByRole('list', { name: 'Delivery history' })
        .getByRole('listitem')
        .last(),
    ).toContainText('DELIVERED');

    await dispatcherPage.reload();
    await search(dispatcherPage, parcel.trackingNumber);
    const dispatcherRow = dispatcherPage
      .getByRole('table', { name: 'All parcels' })
      .getByRole('row')
      .filter({ hasText: parcel.trackingNumber });
    await expect(dispatcherRow).toContainText('DELIVERED', {
      timeout: EVENT_TIMEOUT,
    });

    const adminToken = await apiLogin(request, admin.email, admin.password);
    await expect
      .poll(
        async () => {
          const response = await request.get(
            '/api/v1/admin/notification-logs?pageSize=100',
            { headers: { Authorization: `Bearer ${adminToken}` } },
          );
          if (!response.ok()) return 0;
          const body = (await response.json()) as {
            data?: Array<{ parcelId?: string }>;
          };
          return (body.data ?? []).filter(
            (entry) => entry.parcelId === parcel.parcelID,
          ).length;
        },
        { timeout: EVENT_TIMEOUT },
      )
      .toBeGreaterThan(0);
  } finally {
    await Promise.all([
      customerContext.close(),
      dispatcherContext.close(),
      courierContext.close(),
    ]);
  }
});

type ParcelJourney = { trackingNumber: string; parcelID: string };

async function createThroughWizard(
  page: Page,
  customer: Credentials,
): Promise<ParcelJourney> {
  const suffix = Date.now();
  const recipient = `Journey Recipient ${suffix}`;
  await signIn(page, customer);
  await page.goto('/new');
  await expect(
    page.getByRole('heading', { level: 1, name: 'New delivery' }),
  ).toBeVisible({ timeout: REAL_TIMEOUT });

  await page
    .getByLabel('Pickup zone', { exact: false })
    .selectOption({ label: 'Colombo' });
  await page
    .getByLabel('Destination zone', { exact: false })
    .selectOption({ label: 'Kandy' });
  await page.getByLabel('Weight', { exact: false }).fill('2');
  await page.getByLabel('Size category').selectOption('SMALL');
  await expect(page.getByText(/^LKR [\d,]+\.\d{2}$/).first()).toBeVisible({
    timeout: REAL_TIMEOUT,
  });
  await page.getByRole('button', { name: 'Continue', exact: true }).click();

  for (const [label, value] of Object.entries({
    'Sender name': `Journey Sender ${suffix}`,
    'Sender phone': '+94771234567',
    'Sender address': '12 Temple Road',
    'Sender city': 'Colombo',
    'Recipient name': recipient,
    'Recipient phone': '+94712345678',
    'Recipient address': '45 Lake Drive',
    'Recipient city': 'Kandy',
  })) {
    await page.getByLabel(label, { exact: false }).fill(value);
  }
  await page
    .getByLabel('Description (optional)', { exact: false })
    .fill('Phase 4 acceptance journey');
  await page.getByRole('button', { name: 'Continue', exact: true }).click();
  await page.getByRole('button', { name: 'Create delivery' }).click();
  await expect(page.getByText('Ready for collection')).toBeVisible({
    timeout: REAL_TIMEOUT,
  });
  const trackingNumber = await page
    .getByText(/^SD[KN][A-Z0-9]{8}$/)
    .first()
    .innerText();

  await page
    .getByRole('main')
    .getByRole('link', { name: 'My parcels' })
    .click();
  const card = page.locator('li').filter({ hasText: recipient }).first();
  await expect(card).toBeVisible({ timeout: REAL_TIMEOUT });
  const href = await card
    .getByRole('link', { name: `View parcel ${trackingNumber}` })
    .getAttribute('href');
  const parcelID = href?.split('/').pop() ?? '';
  expect(parcelID).toMatch(/^[0-9a-f-]{36}$/);
  return { trackingNumber, parcelID };
}

async function assignThroughUI(
  page: Page,
  dispatcher: Credentials,
  parcel: ParcelJourney,
  courierID: string,
) {
  await signIn(page, dispatcher);
  await page.goto('/parcels');
  await expect(
    page.getByText('Live. Parcel statuses change here as they happen.', {
      exact: true,
    }),
  ).toBeVisible({ timeout: REAL_TIMEOUT });
  await search(page, parcel.trackingNumber);
  const row = page
    .getByRole('table', { name: 'All parcels' })
    .getByRole('row')
    .filter({ hasText: parcel.trackingNumber });
  await row
    .getByRole('button', {
      name: `Assign courier to parcel ${parcel.trackingNumber}`,
    })
    .click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('Courier').selectOption(courierID);
  await dialog.getByRole('button', { name: 'Assign', exact: true }).click();
  await expect(
    page.getByText(`Parcel ${parcel.trackingNumber} assigned successfully.`),
  ).toBeVisible({ timeout: REAL_TIMEOUT });
}

async function completeThroughCourierUI(
  page: Page,
  courier: Credentials,
  parcel: ParcelJourney,
) {
  await signIn(page, courier);
  const card = page
    .locator('li')
    .filter({ hasText: parcel.trackingNumber })
    .first();
  await expect(card).toBeVisible({ timeout: REAL_TIMEOUT });
  await card.getByRole('link', { name: 'View delivery' }).click();
  await expect(
    page.getByRole('region', { name: 'Delivery status' }),
  ).toBeVisible({ timeout: REAL_TIMEOUT });

  await moveToStatus(page, 'PICKED_UP', 'PICKED UP');
  await moveToStatus(page, 'IN_TRANSIT', 'IN TRANSIT');
  await moveToStatus(page, 'OUT_FOR_DELIVERY', 'OUT FOR DELIVERY');

  await page.locator('#courier-proof-photo').setInputFiles({
    name: 'proof.png',
    mimeType: 'image/png',
    buffer: PNG,
  });
  await page.getByLabel('Proof received by').fill('Amali Fernando');
  await page.getByLabel('Delivery note').fill('Handed to recipient');
  await page.getByRole('button', { name: 'Upload proof' }).click();
  await expect(
    page.getByText('Delivery proof uploaded', { exact: true }),
  ).toBeVisible({ timeout: REAL_TIMEOUT });

  await page.getByLabel('Next status').selectOption('DELIVERED');
  await page.getByLabel('Recipient name').fill('Amali Fernando');
  await page.getByRole('button', { name: 'Save status' }).click();
  await expect(
    page
      .getByRole('region', { name: 'Delivery status' })
      .getByText('DELIVERED', { exact: true }),
  ).toBeVisible({ timeout: REAL_TIMEOUT });
}

async function signIn(page: Page, credentials: Credentials) {
  await signInThroughForm(page, credentials.email, credentials.password);
  await expect(page.locator('#login-email')).toHaveCount(0);
}

async function search(page: Page, trackingNumber: string) {
  await page.locator('#search-tracking-number').fill(trackingNumber);
  await expect(
    page
      .getByRole('table', { name: 'All parcels' })
      .getByRole('row')
      .filter({ hasText: trackingNumber }),
  ).toBeVisible({ timeout: REAL_TIMEOUT });
}

async function moveToStatus(page: Page, value: string, label: string) {
  await page.getByLabel('Next status').selectOption(value);
  await page.getByRole('button', { name: 'Save status' }).click();
  await expect(
    page
      .getByRole('region', { name: 'Delivery status' })
      .getByText(label, { exact: true }),
  ).toBeVisible({ timeout: REAL_TIMEOUT });
}
