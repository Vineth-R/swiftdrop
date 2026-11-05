import { test, expect } from '@playwright/test';

import { REQUEST_ID, SERVER_WORDS, fail, ok } from './support/mock-api';
import { signInAs } from './support/sign-in';

test.use({ colorScheme: 'light' });

const daily = [
  { date: '2026-11-01', created: 42, delivered: 35, failed: 3 },
  { date: '2026-11-02', created: 51, delivered: 44, failed: 2 },
];

const outcomes = {
  from: '2026-10-04',
  to: '2026-11-03',
  totals: { created: 410, delivered: 352, failed: 21, inProgress: 37 },
  successRate: 94.37,
  failureRate: 5.63,
  topFailureReasons: [
    { reason: 'Recipient not available', count: 9 },
    { reason: 'Incorrect address', count: 6 },
  ],
};

const emptyOutcomes = {
  ...outcomes,
  totals: { created: 0, delivered: 0, failed: 0, inProgress: 0 },
  successRate: 0,
  failureRate: 0,
  topFailureReasons: [],
};

test.beforeEach(async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
});

test('an administrator sees outcomes, the chart and the daily table', async ({
  page,
}) => {
  await signInAs(page, 'ADMIN', {
    'GET /reports/daily': ok(daily),
    'GET /reports/outcomes': ok(outcomes),
  });
  await page.goto('/reports');

  await expect(
    page.getByRole('heading', { level: 1, name: 'Reports' }),
  ).toBeVisible();
  await expect(page.getByText(/352 of 410 parcels created/)).toBeVisible();
  await expect(page.getByText('94.37%')).toBeVisible();
  await expect(page.getByText('5.63%')).toBeVisible();
  const reasons = page.getByRole('table', { name: 'Top failure reasons' });
  await expect(
    reasons.getByRole('cell', { name: 'Recipient not available' }),
  ).toBeVisible();

  const dailyTable = page.getByRole('table', { name: 'Daily parcels' });
  await expect(dailyTable.getByRole('row')).toHaveCount(3);
  await expect(
    dailyTable.getByRole('cell', { name: '1 Nov 2026' }),
  ).toBeVisible();
  await expect(
    page.getByRole('img', {
      name: /Bar chart of delivered and failed parcels/,
    }),
  ).toBeVisible();
});

test('the range is sent to both report endpoints', async ({ page }) => {
  await signInAs(page, 'ADMIN', {
    'GET /reports/daily': ok(daily),
    'GET /reports/outcomes': ok(outcomes),
  });
  await page.goto('/reports');
  await expect(page.getByText('94.37%')).toBeVisible();

  const dailyRequest = page.waitForRequest(
    /\/reports\/daily\?from=2026-10-01&to=2026-10-31$/,
  );
  const outcomesRequest = page.waitForRequest(
    /\/reports\/outcomes\?from=2026-10-01&to=2026-10-31$/,
  );
  await page.getByLabel('From', { exact: true }).fill('2026-10-01');
  await page.getByLabel('To', { exact: true }).fill('2026-10-31');
  await page.getByRole('button', { name: 'Apply range' }).click();
  await dailyRequest;
  await outcomesRequest;
});

test('a range that is wrong is explained and nothing is requested', async ({
  page,
}) => {
  const { calls } = await signInAs(page, 'ADMIN', {
    'GET /reports/daily': ok(daily),
    'GET /reports/outcomes': ok(outcomes),
  });
  await page.goto('/reports');
  await expect(page.getByText('94.37%')).toBeVisible();
  const before = calls.filter((call) =>
    call.key.startsWith('GET /reports'),
  ).length;

  await page.getByLabel('From', { exact: true }).fill('2026-11-04');
  await page.getByLabel('To', { exact: true }).fill('2026-11-01');
  await page.getByRole('button', { name: 'Apply range' }).click();
  await expect(
    page.getByText('The end date must be on or after the start date.'),
  ).toBeVisible();

  await page.getByLabel('From', { exact: true }).fill('2026-01-01');
  await page.getByLabel('To', { exact: true }).fill('2026-11-01');
  await page.getByRole('button', { name: 'Apply range' }).click();
  await expect(
    page.getByText('Choose a range of 90 days or fewer.'),
  ).toBeVisible();

  const after = calls.filter((call) =>
    call.key.startsWith('GET /reports'),
  ).length;
  expect(after).toBe(before);
});

test('shows a loading state while the reports load', async ({ page }) => {
  await signInAs(page, 'ADMIN', {
    'GET /reports/daily': { ...ok(daily), delayMs: 800 },
    'GET /reports/outcomes': { ...ok(outcomes), delayMs: 800 },
  });
  await page.goto('/reports');
  await expect(page.getByText('Loading outcomes')).toBeAttached();
  await expect(page.getByText('94.37%')).toBeVisible();
  await expect(page.getByText('Loading outcomes')).toHaveCount(0);
});

test('shows an empty state and resets the range when no parcel falls in it', async ({
  page,
}) => {
  const { calls } = await signInAs(page, 'ADMIN', {
    'GET /reports/daily': ok(
      daily.map((row) => ({ ...row, created: 0, delivered: 0, failed: 0 })),
    ),
    'GET /reports/outcomes': ok(emptyOutcomes),
  });
  await page.goto('/reports');
  await expect(
    page.getByRole('heading', { name: 'No parcels in this range' }),
  ).toBeVisible();

  const before = calls.filter((call) =>
    call.key.startsWith('GET /reports'),
  ).length;
  await page.getByRole('button', { name: 'Reset range' }).click();
  await expect
    .poll(
      () => calls.filter((call) => call.key.startsWith('GET /reports')).length,
    )
    .toBeGreaterThan(before);
});

test('says so when nothing failed', async ({ page }) => {
  await signInAs(page, 'ADMIN', {
    'GET /reports/daily': ok(daily),
    'GET /reports/outcomes': ok({
      ...outcomes,
      totals: { created: 10, delivered: 10, failed: 0, inProgress: 0 },
      successRate: 100,
      failureRate: 0,
      topFailureReasons: [],
    }),
  });
  await page.goto('/reports');
  await expect(
    page.getByRole('heading', { name: 'No failed parcels' }),
  ).toBeVisible();
  await expect(
    page.getByText('Every finished parcel in this range was delivered.'),
  ).toBeVisible();
});

test('a server fault shows a reference, hides the server words and can be retried', async ({
  page,
}) => {
  let failing = true;
  await signInAs(page, 'ADMIN', {
    'GET /reports/daily': () =>
      failing ? fail(500, 'INTERNAL_ERROR') : ok(daily),
    'GET /reports/outcomes': () =>
      failing ? fail(500, 'INTERNAL_ERROR') : ok(outcomes),
  });
  await page.goto('/reports');

  // Next.js keeps an empty role="alert" route announcer on every page,
  // so pick ours by its text.
  const alert = page
    .getByRole('alert')
    .filter({ hasText: 'Reports could not be loaded' });
  await expect(alert).toContainText('Reports could not be loaded');
  await expect(alert).toContainText(REQUEST_ID);
  await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);

  failing = false;
  await alert.getByRole('button', { name: 'Try again' }).click();
  await expect(page.getByText('94.37%')).toBeVisible();
  await expect(alert).toHaveCount(0);
});

test('a dispatcher sees the reports too', async ({ page }) => {
  await signInAs(page, 'DISPATCHER', {
    'GET /reports/daily': ok(daily),
    'GET /reports/outcomes': ok(outcomes),
  });
  await page.goto('/reports');
  await expect(
    page.getByRole('heading', { level: 1, name: 'Reports' }),
  ).toBeVisible();
  await expect(page.getByText('94.37%')).toBeVisible();
});

for (const role of ['CUSTOMER', 'MERCHANT', 'COURIER'] as const) {
  test(`a ${role.toLowerCase()} is refused and no report is requested`, async ({
    page,
  }) => {
    const { calls } = await signInAs(page, role, {
      'GET /reports/daily': ok(daily),
      'GET /reports/outcomes': ok(outcomes),
    });
    await page.goto('/reports');

    await expect(
      page.getByRole('heading', { name: 'You do not have access' }),
    ).toBeVisible();
    await expect(
      page.getByText('This page is for dispatchers and administrators'),
    ).toBeVisible();
    expect(calls.some((call) => call.key.startsWith('GET /reports'))).toBe(
      false,
    );
  });
}

test('on a phone nothing scrolls sideways', async ({ page }) => {
  await page.setViewportSize({ width: 375, height: 812 });
  await signInAs(page, 'ADMIN', {
    'GET /reports/daily': ok(daily),
    'GET /reports/outcomes': ok(outcomes),
  });
  await page.goto('/reports');
  await expect(page.getByText('94.37%')).toBeVisible();

  const overflow = await page.evaluate(
    'document.documentElement.scrollWidth - document.documentElement.clientWidth',
  );
  expect(overflow, 'the reports page scrolls sideways').toBe(0);
});
