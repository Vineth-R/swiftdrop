import { test, expect } from '@playwright/test';

test('home page links to the dispatch preview', async ({ page }) => {
  await page.goto('/');

  await page
    .locator('nav[aria-label="Main navigation"]')
    .getByRole('link', { name: /Dispatch/ })
    .click();

  await expect(page).toHaveURL(/\/dispatch$/);
  await expect(
    page.getByRole('heading', { name: /Dispatcher Control Hub/ }),
  ).toBeVisible();
  await expect(
    page.getByRole('heading', { name: '1. Unassigned Parcels Queue' }),
  ).toBeVisible();
  await expect(
    page.getByRole('heading', { name: '2. Courier Workload Panel' }),
  ).toBeVisible();
});

test('dispatch preview enforces the simulated role boundary', async ({
  page,
}) => {
  await page.goto('/dispatch');

  await page.getByLabel('Simulated user role').selectOption('COURIER');

  await expect(
    page.getByRole('heading', { name: '403 Forbidden: Access Restricted' }),
  ).toBeVisible();

  await page.getByRole('button', { name: 'Switch Role to DISPATCHER' }).click();

  await expect(
    page.getByRole('heading', { name: /Dispatcher Control Hub/ }),
  ).toBeVisible();
});
