import { expect, test, type Page } from '@playwright/test';

import {
  fail,
  ok,
  REQUEST_ID,
  SERVER_WORDS,
  type Handler,
} from './support/mock-api';
import { signInAs } from './support/sign-in';

/**
 * How the booking wizard (/new) shows fee estimate problems: beside the field
 * they are about, with a countdown for a rate limit (DESIGN.md sections 9 and 10).
 */

const origin = '7a1e0c10-2b3d-4c5e-8f90-a1b2c3d4e5f6';
const destination = '9b2f1d21-3c4e-4d6f-9a01-b2c3d4e5f607';
const fee = {
  currency: 'LKR',
  baseFee: 450,
  weightCharge: 100,
  sizeSurcharge: 0,
  total: 550,
};
const zones = [
  { id: origin, code: 'CMB', name: 'Colombo', districts: [] },
  { id: destination, code: 'KDY', name: 'Kandy', districts: [] },
];
const parcel = {
  id: 'e5a1b7c2-6d3f-4a8b-9c10-2d3e4f5a6b7c',
  trackingNumber: 'SDK7M2Q9XP4',
  status: 'CREATED',
  sender: { name: 'Nimal Perera', city: 'Colombo' },
  recipient: { name: 'Amali Fernando', city: 'Kandy' },
  weightKg: 2,
  sizeCategory: 'SMALL',
  description: 'Books',
  fee,
  createdAt: '2026-11-02T10:00:00Z',
};

async function fillRoute(page: Page) {
  await page.getByLabel('Pickup zone', { exact: false }).selectOption(origin);
  await page
    .getByLabel('Destination zone', { exact: false })
    .selectOption(destination);
  await page.getByLabel('Weight', { exact: false }).fill('2');
}

async function openWith(
  page: Page,
  handlers: Record<string, Handler>,
): Promise<void> {
  await signInAs(page, 'CUSTOMER', { 'GET /zones': ok(zones), ...handlers });
  await page.goto('/new');
  await fillRoute(page);
}

async function open(page: Page, estimate: Handler): Promise<void> {
  await openWith(page, { 'POST /pricing/estimate': estimate });
}

async function fillContacts(page: Page) {
  for (const [label, value] of Object.entries({
    'Sender name': 'Nimal Perera',
    'Sender phone': '+94771234567',
    'Sender address': '12 Temple Road',
    'Sender city': 'Colombo',
    'Recipient name': 'Amali Fernando',
    'Recipient phone': '+94712345678',
    'Recipient address': '45 Lake Drive',
    'Recipient city': 'Kandy',
  }))
    await page.getByLabel(label, { exact: false }).fill(value);
  await page
    .getByLabel('Description (optional)', { exact: false })
    .fill('Books');
}

async function continueToContacts(page: Page) {
  await expect(page.getByText('LKR 550.00').first()).toBeVisible();
  await page.getByRole('button', { name: 'Continue', exact: true }).click();
}

async function reachReview(page: Page) {
  await continueToContacts(page);
  await fillContacts(page);
  await page.getByRole('button', { name: 'Continue', exact: true }).click();
  await expect(
    page.getByText('Review delivery', { exact: false }).first(),
  ).toBeVisible();
}

const routeMessage =
  'Delivery is not available between these zones. Choose another route.';
const weightMessage =
  'A delivery fee is not available for this weight. Change the weight and try again.';
const fieldGuidance = 'Change the marked details to see a fee estimate.';

test.describe('booking fee estimate problems', () => {
  test('a route with no price is shown beside the destination zone', async ({
    page,
  }) => {
    await open(page, fail(422, 'ROUTE_NOT_SERVICEABLE'));
    await expect(page.locator('#delivery-recipientZone')).toHaveAttribute(
      'aria-invalid',
      'true',
    );
    await expect(page.getByText(routeMessage)).toHaveCount(1);
    await expect(page.getByText(fieldGuidance)).toBeVisible();
    await expect(
      page.getByRole('button', { name: 'Retry estimate' }),
    ).toHaveCount(0);
    await expect(page.locator('main').getByRole('alert')).toHaveCount(0);
    await expect(
      page.getByRole('button', { name: 'Continue', exact: true }),
    ).toBeDisabled();
    await expect(page.locator('body')).not.toContainText(SERVER_WORDS);
  });

  test('a weight with no price is shown beside the weight', async ({
    page,
  }) => {
    await open(page, fail(422, 'WEIGHT_NOT_PRICED'));
    await expect(page.locator('#delivery-weight')).toHaveAttribute(
      'aria-invalid',
      'true',
    );
    await expect(page.getByText(weightMessage)).toHaveCount(1);
    await expect(page.getByText(fieldGuidance)).toBeVisible();
    await expect(
      page.getByRole('button', { name: 'Retry estimate' }),
    ).toHaveCount(0);
    await expect(page.locator('main').getByRole('alert')).toHaveCount(0);
  });

  test('the message clears once a new estimate works', async ({ page }) => {
    await open(page, ({ body }) =>
      (body as { weightKg: number }).weightKg === 2
        ? fail(422, 'WEIGHT_NOT_PRICED')
        : ok(fee),
    );
    await expect(page.getByText(weightMessage)).toBeVisible();
    await page.getByLabel('Weight', { exact: false }).fill('3');
    await expect(page.getByText('LKR 550.00').first()).toBeVisible();
    await expect(page.getByText(weightMessage)).toHaveCount(0);
    await expect(page.locator('#delivery-weight')).not.toHaveAttribute(
      'aria-invalid',
      'true',
    );
  });

  test('an existing field message stays while the next estimate is pending', async ({
    page,
  }) => {
    await open(page, ({ body }) =>
      (body as { weightKg: number }).weightKg === 2
        ? fail(422, 'WEIGHT_NOT_PRICED')
        : { ...ok(fee), delayMs: 700 },
    );
    await expect(page.getByText(weightMessage)).toBeVisible();
    await page.getByLabel('Weight', { exact: false }).fill('3');
    await expect(page.getByText('Calculating delivery fee')).toBeAttached();
    await expect(page.getByText(weightMessage)).toBeVisible();
    await expect(page.getByText('LKR 550.00').first()).toBeVisible();
    await expect(page.getByText(weightMessage)).toHaveCount(0);
  });

  test('a route message clears when only the weight changes and the fee works', async ({
    page,
  }) => {
    await open(page, ({ body }) =>
      (body as { weightKg: number }).weightKg === 2
        ? fail(422, 'ROUTE_NOT_SERVICEABLE')
        : ok(fee),
    );
    await expect(page.getByText(routeMessage)).toBeVisible();
    await page.getByLabel('Weight', { exact: false }).fill('3');
    await expect(page.getByText('LKR 550.00').first()).toBeVisible();
    await expect(page.getByText(routeMessage)).toHaveCount(0);
    await expect(page.locator('#delivery-recipientZone')).not.toHaveAttribute(
      'aria-invalid',
      'true',
    );
  });

  test('a 422 from the estimate lands beside the fields it names', async ({
    page,
  }) => {
    await open(
      page,
      fail(422, 'VALIDATION_ERROR', {
        details: [
          { field: 'weightKg', issue: SERVER_WORDS },
          { field: 'destinationZoneId', issue: SERVER_WORDS },
        ],
      }),
    );
    await expect(page.locator('#delivery-weight')).toHaveAttribute(
      'aria-invalid',
      'true',
    );
    await expect(page.locator('#delivery-recipientZone')).toHaveAttribute(
      'aria-invalid',
      'true',
    );
    await expect(page.getByText('Choose a zone.')).toBeVisible();
    await expect(
      page.getByText('Check this information and try again.'),
    ).toBeVisible();
    await expect(page.getByText(fieldGuidance)).toBeVisible();
    await expect(page.locator('main').getByRole('alert')).toHaveCount(0);
    await expect(page.locator('body')).not.toContainText(SERVER_WORDS);
  });

  test('a zone that has gone stays an alert, as a warning, with Retry', async ({
    page,
  }) => {
    await open(page, fail(404, 'ZONE_NOT_FOUND'));
    const warning = page
      .getByRole('status')
      .filter({ hasText: 'A selected zone is no longer available' });
    await expect(warning).toBeVisible();
    await expect(page.locator('main').getByRole('alert')).toHaveCount(0);
    await expect(
      page.getByRole('button', { name: 'Retry estimate' }),
    ).toBeEnabled();
  });

  test('too many requests shows the wait and holds Retry estimate', async ({
    page,
  }) => {
    await open(
      page,
      fail(429, 'RATE_LIMITED', { headers: { 'Retry-After': '1' } }),
    );
    await expect(
      page.getByText('Too many attempts from this device'),
    ).toBeVisible();
    await expect(page.getByText(/\d+ seconds/)).toBeVisible();
    const retry = page.getByRole('button', { name: 'Retry estimate' });
    await expect(retry).toBeDisabled();
    await expect(page.getByText('You can try again now.')).toBeVisible({
      timeout: 5000,
    });
    await expect(retry).toBeEnabled();
  });

  test('an estimate rate limit stops automatic estimates until the wait ends', async ({
    page,
  }) => {
    let estimateCalls = 0;
    await openWith(page, {
      'POST /pricing/estimate': () => {
        estimateCalls += 1;
        return estimateCalls === 1
          ? fail(429, 'RATE_LIMITED', { headers: { 'Retry-After': '1' } })
          : ok(fee);
      },
    });
    await expect(
      page.getByText('Too many attempts from this device'),
    ).toBeVisible();
    const before = estimateCalls;
    await page.getByLabel('Weight', { exact: false }).fill('3');
    // Past the 350 ms estimate debounce, but before the one-second wait ends.
    await page.waitForTimeout(500);
    expect(estimateCalls).toBe(before);
    const retry = page.getByRole('button', { name: 'Retry estimate' });
    await expect(retry).toBeEnabled({ timeout: 5000 });
    await retry.click();
    await expect(page.getByText('LKR 550.00').first()).toBeVisible();
  });

  test('an estimate rate limit does not block creation once a fee is available', async ({
    page,
  }) => {
    let estimateCalls = 0;
    const { calls } = await signInAs(page, 'CUSTOMER', {
      'GET /zones': ok(zones),
      'POST /pricing/estimate': () => {
        estimateCalls += 1;
        return estimateCalls === 1
          ? fail(429, 'RATE_LIMITED', { headers: { 'Retry-After': '1' } })
          : ok(fee);
      },
      'POST /parcels': ok(parcel, 201),
    });
    await page.goto('/new');
    await fillRoute(page);
    const retry = page.getByRole('button', { name: 'Retry estimate' });
    await expect(retry).toBeEnabled({ timeout: 5000 });
    await retry.click();
    await continueToContacts(page);
    await fillContacts(page);
    await page.getByRole('button', { name: 'Continue', exact: true }).click();
    const create = page.getByRole('button', {
      name: 'Create delivery',
      exact: true,
    });
    await expect(create).toBeEnabled();
    await create.click();
    await expect(
      page.getByRole('heading', { name: 'Delivery created' }),
    ).toBeVisible();
    expect(calls.filter((call) => call.key === 'POST /parcels')).toHaveLength(
      1,
    );
  });

  test('a create rate limit holds Create delivery until the wait ends', async ({
    page,
  }) => {
    let createCalls = 0;
    await signInAs(page, 'CUSTOMER', {
      'GET /zones': ok(zones),
      'POST /pricing/estimate': ok(fee),
      'POST /parcels': () => {
        createCalls += 1;
        return createCalls === 1
          ? fail(429, 'RATE_LIMITED', { headers: { 'Retry-After': '1' } })
          : ok(parcel, 201);
      },
    });
    await page.goto('/new');
    await fillRoute(page);
    await reachReview(page);
    const create = page.getByRole('button', {
      name: 'Create delivery',
      exact: true,
    });
    await create.click();
    await expect(
      page.getByText('Too many attempts from this device'),
    ).toBeVisible();
    await expect(create).toBeDisabled();
    await expect(page.getByText('You can try again now.')).toBeVisible({
      timeout: 5000,
    });
    await expect(create).toBeEnabled();
    await create.click();
    await expect(
      page.getByRole('heading', { name: 'Delivery created' }),
    ).toBeVisible();
    expect(createCalls).toBe(2);
  });

  test('editing during a create rate limit does not clear its countdown', async ({
    page,
  }) => {
    await signInAs(page, 'CUSTOMER', {
      'GET /zones': ok(zones),
      'POST /pricing/estimate': ok(fee),
      'POST /parcels': fail(429, 'RATE_LIMITED', {
        headers: { 'Retry-After': '30' },
      }),
    });
    await page.goto('/new');
    await fillRoute(page);
    await reachReview(page);
    const create = page.getByRole('button', {
      name: 'Create delivery',
      exact: true,
    });
    await create.click();
    await expect(
      page.getByText('Too many attempts from this device'),
    ).toBeVisible();
    await expect(create).toBeDisabled();

    // A rate limit belongs to the endpoint, so editing a field must not clear it.
    await page.getByRole('button', { name: 'Back', exact: true }).click();
    await page
      .getByLabel('Sender name', { exact: false })
      .fill('Nimali Perera');
    await page.getByRole('button', { name: 'Continue', exact: true }).click();
    await expect(create).toBeDisabled();
    await expect(
      page.getByText('Too many attempts from this device'),
    ).toBeVisible();
  });

  test('a successful estimate does not remove a create-form field message', async ({
    page,
  }) => {
    await signInAs(page, 'CUSTOMER', {
      'GET /zones': ok(zones),
      'POST /pricing/estimate': ({ body }) =>
        (body as { weightKg: number }).weightKg === 3
          ? ok({ ...fee, baseFee: 550, total: 650 })
          : ok(fee),
      'POST /parcels': fail(422, 'VALIDATION_ERROR', {
        details: [{ field: 'recipient.zoneId', issue: SERVER_WORDS }],
      }),
    });
    await page.goto('/new');
    await fillRoute(page);
    await reachReview(page);
    await page
      .getByRole('button', { name: 'Create delivery', exact: true })
      .click();

    const zone = page.locator('#delivery-recipientZone');
    await expect(zone).toHaveAttribute('aria-invalid', 'true');
    await expect(
      page.getByText('Check this information and try again.'),
    ).toBeVisible();

    // Changing a route input starts a new estimate (a different fee proves it);
    // it must not clear the create-form message that the server put on the field.
    await page.getByLabel('Weight', { exact: false }).fill('3');
    await expect(page.getByText('LKR 650.00').first()).toBeVisible();
    await expect(zone).toHaveAttribute('aria-invalid', 'true');
    await expect(
      page.getByText('Check this information and try again.'),
    ).toBeVisible();
  });

  test('a server fault keeps its Reference and Retry', async ({ page }) => {
    await open(page, fail(500, 'INTERNAL_ERROR'));
    await expect(page.getByText('Something broke on our side')).toBeVisible();
    await expect(page.getByText(`Reference ${REQUEST_ID}`)).toBeVisible();
    await expect(
      page.getByRole('button', { name: 'Retry estimate' }),
    ).toBeEnabled();
  });
});
