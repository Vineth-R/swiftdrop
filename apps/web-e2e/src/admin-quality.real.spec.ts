import { expect, test } from '@playwright/test';

import {
  apiLogin,
  credentialsForRun,
  signInThroughForm,
} from './support/real-stack';

const REAL_TIMEOUT = 20_000;

/**
 * Real-stack read journeys for the three administrative screens that previously
 * had mocked-only UI coverage: users, pricing configuration and reports. The
 * tests assert successful backend responses and real seeded content; they do
 * not mutate shared seed data.
 */
test.describe('real-stack administrative screens', () => {
  test('admin users loads the real user list', async ({ page, request }) => {
    test.setTimeout(90_000);
    const admin = credentialsForRun('ADMIN');
    test.skip(!admin, 'set REAL_STACK_ADMIN_*');
    if (!admin) return;

    const token = await apiLogin(request, admin.email, admin.password);
    const response = await request.get(
      '/api/v1/admin/users/?page=1&pageSize=5',
      {
        headers: { Authorization: `Bearer ${token}` },
      },
    );
    expect(response.ok()).toBeTruthy();
    const body = (await response.json()) as { data: Array<{ email: string }> };
    expect(body.data.length).toBeGreaterThan(0);

    await signInThroughForm(page, admin.email, admin.password);
    await page.goto('/admin/users');
    await expect(
      page.getByRole('heading', { level: 1, name: 'Users' }),
    ).toBeVisible({ timeout: REAL_TIMEOUT });
    await expect(page.getByRole('table', { name: 'Users' })).toBeVisible({
      timeout: REAL_TIMEOUT,
    });
    await expect(
      page
        .getByRole('table', { name: 'Users' })
        .getByText(body.data[0].email, { exact: false })
        .first(),
    ).toBeVisible({ timeout: REAL_TIMEOUT });
  });

  test('pricing administration loads real zones, rules and bands', async ({
    page,
  }) => {
    test.setTimeout(90_000);
    const admin = credentialsForRun('ADMIN');
    test.skip(!admin, 'set REAL_STACK_ADMIN_*');
    if (!admin) return;

    await signInThroughForm(page, admin.email, admin.password);
    const rules = page.waitForResponse(
      (response) =>
        response.url().includes('/api/v1/admin/pricing/rules') &&
        response.status() === 200,
    );
    const bands = page.waitForResponse(
      (response) =>
        response.url().includes('/api/v1/admin/pricing/bands') &&
        response.status() === 200,
    );
    await page.goto('/admin/pricing');
    await rules;
    await bands;
    await expect(
      page.getByRole('heading', { level: 1, name: 'Pricing' }),
    ).toBeVisible({ timeout: REAL_TIMEOUT });
    await expect(page.getByRole('heading', { level: 2 })).toHaveText([
      'Pricing rules',
      'Weight bands',
      'Size surcharges',
    ]);
  });

  test('reports loads real daily and outcome data or its empty state', async ({
    page,
  }) => {
    test.setTimeout(90_000);
    const admin = credentialsForRun('ADMIN');
    test.skip(!admin, 'set REAL_STACK_ADMIN_*');
    if (!admin) return;

    await signInThroughForm(page, admin.email, admin.password);
    const daily = page.waitForResponse(
      (response) =>
        response.url().includes('/api/v1/reports/daily') &&
        response.status() === 200,
    );
    const outcomes = page.waitForResponse(
      (response) =>
        response.url().includes('/api/v1/reports/outcomes') &&
        response.status() === 200,
    );
    await page.goto('/reports');
    await daily;
    await outcomes;
    await expect(
      page.getByRole('heading', { level: 1, name: 'Reports' }),
    ).toBeVisible({ timeout: REAL_TIMEOUT });
    const outcomes = page.getByRole('heading', { level: 2, name: 'Outcomes' });
    const empty = page.getByText('No parcels in this range');
    await expect(outcomes.or(empty)).toBeVisible({ timeout: REAL_TIMEOUT });
    if (await outcomes.isVisible()) {
      await expect(
        page.getByRole('heading', { level: 2, name: 'Daily parcels' }),
      ).toBeVisible({ timeout: REAL_TIMEOUT });
    }
  });
});
