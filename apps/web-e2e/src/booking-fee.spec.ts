import { expect, test, type Page } from '@playwright/test';

import { fail, ok, SERVER_WORDS, type Handler } from './support/mock-api';
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

async function open(page: Page, estimate: Handler) {
  await signInAs(page, 'CUSTOMER', {
    'GET /zones': ok(zones),
    'POST /pricing/estimate': estimate,
  });
  await page.goto('/new');
  await page.getByLabel('Pickup zone', { exact: false }).selectOption(origin);
  await page
    .getByLabel('Destination zone', { exact: false })
    .selectOption(destination);
  await page.getByLabel('Weight', { exact: false }).fill('2');
}

const routeMessage =
  'Delivery is not available between these zones. Choose another route.';
const weightMessage =
  'A delivery fee is not available for this weight. Change the weight and try again.';

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
    await expect(
      page.getByText('Change the marked details to see a fee estimate.'),
    ).toBeVisible();
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
    ).toBeVisible();
  });

  test('too many requests shows how long to wait', async ({ page }) => {
    await open(
      page,
      fail(429, 'RATE_LIMITED', { headers: { 'Retry-After': '30' } }),
    );
    await expect(
      page.getByText('Too many attempts from this device'),
    ).toBeVisible();
    await expect(page.getByText(/\d+ seconds/)).toBeVisible();
  });

  test('a server fault keeps its Reference and Retry', async ({ page }) => {
    await open(page, fail(500, 'INTERNAL_ERROR'));
    await expect(page.getByText('Something broke on our side')).toBeVisible();
    await expect(
      page.getByRole('button', { name: 'Retry estimate' }),
    ).toBeVisible();
  });
});
