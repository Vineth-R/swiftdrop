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

/**
 * `days` consecutive days from 1 Oct 2026. Nothing finishes before day number
 * `firstFinishedDay` (counting from 0); every day from then on delivers two.
 */
function sparseDays(days: number, firstFinishedDay: number) {
  return Array.from({ length: days }, (_, index) => ({
    date: new Date(Date.UTC(2026, 9, 1 + index)).toISOString().slice(0, 10),
    created: 1,
    delivered: index >= firstFinishedDay ? 2 : 0,
    failed: 0,
  }));
}

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
  await expect(page.getByText('Tallest day: 46.')).toBeVisible();
});
test('a quick range button loads that many days ending today', async ({
  page,
}) => {
  await signInAs(page, 'ADMIN', {
    'GET /reports/daily': ok(daily),
    'GET /reports/outcomes': ok(outcomes),
  });
  await page.goto('/reports');
  await expect(page.getByText('94.37%')).toBeVisible();

  const request = page.waitForRequest((candidate) => {
    const url = new URL(candidate.url());
    if (!url.pathname.endsWith('/reports/daily')) return false;
    const from = Date.parse(`${url.searchParams.get('from')}T00:00:00Z`);
    const to = Date.parse(`${url.searchParams.get('to')}T00:00:00Z`);
    return (to - from) / 86_400_000 === 6;
  });
  await page.getByRole('button', { name: 'Last 7 days' }).click();
  await request;
});

test('the quick range that matches the current range is marked as pressed', async ({
  page,
}) => {
  await signInAs(page, 'ADMIN', {
    'GET /reports/daily': ok(daily),
    'GET /reports/outcomes': ok(outcomes),
  });
  await page.goto('/reports');
  // The theme switch in the header has pressed buttons too, so look only here.
  const quickRanges = page.getByRole('group', { name: 'Quick ranges' });
  const pressed = (name: string) =>
    quickRanges.getByRole('button', { name, pressed: true });

  // The page opens on the last 30 days, so clicking it again changes nothing
  // and the button already says why.
  await expect(pressed('Last 30 days')).toBeVisible();
  await expect(pressed('Last 7 days')).toHaveCount(0);

  await page.getByRole('button', { name: 'Last 90 days' }).click();
  await expect(pressed('Last 90 days')).toBeVisible();
  await expect(pressed('Last 30 days')).toHaveCount(0);

  // A range typed by hand matches none of them.
  const threeDaysAgo = new Date(Date.now() - 3 * 86_400_000)
    .toISOString()
    .slice(0, 10);
  await page.getByLabel('From', { exact: true }).fill(threeDaysAgo);
  await page.getByRole('button', { name: 'Apply range' }).click();
  await expect(page.getByText('Loading outcomes')).toHaveCount(0);
  await expect(quickRanges.getByRole('button', { pressed: true })).toHaveCount(
    0,
  );
});

test('a quick range button keeps its focus and the page does not shift while it loads', async ({
  page,
}) => {
  await signInAs(page, 'ADMIN', {
    'GET /reports/daily': ok(daily),
    'GET /reports/outcomes': ok(outcomes),
  });
  await page.goto('/reports');
  await expect(page.getByText('94.37%')).toBeVisible();

  const button = page.getByRole('button', { name: 'Last 7 days' });
  const before = await button.boundingBox();

  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route('**/api/v1/reports/**', async (route) => {
    await gate;
    await route.fallback();
  });
  await button.click();

  await expect(page.getByText('Loading outcomes')).toBeAttached();
  await expect(button).toBeEnabled();
  await expect(button).toBeFocused();
  expect((await button.boundingBox())?.y).toBe(before?.y);

  release();
  await expect(page.getByText('Loading outcomes')).toHaveCount(0);
  await expect(button).toBeFocused();
});

test('a long daily table is split into pages', async ({ page }) => {
  const longDaily = Array.from({ length: 40 }, (_, index) => ({
    date: new Date(Date.UTC(2026, 9, 1 + index)).toISOString().slice(0, 10),
    created: 1,
    delivered: 1,
    failed: 0,
  }));
  await signInAs(page, 'ADMIN', {
    'GET /reports/daily': ok(longDaily),
    'GET /reports/outcomes': ok(outcomes),
  });
  await page.goto('/reports');

  const table = page.getByRole('table', { name: 'Daily parcels' });
  await expect(table.getByRole('row')).toHaveCount(16);
  await expect(page.getByText('Page 1 of 3')).toBeVisible();
  await expect(
    page.getByRole('button', { name: 'Previous', exact: true }),
  ).toBeDisabled();

  await expect(table.getByRole('row').nth(1)).toContainText('9 Nov 2026');

  await page.getByRole('button', { name: 'Next', exact: true }).click();
  await expect(page.getByText('Page 2 of 3')).toBeVisible();
  await expect(table.getByRole('cell', { name: '25 Oct 2026' })).toBeVisible();
});
test('each day of the chart has its numbers as a tooltip', async ({ page }) => {
  await signInAs(page, 'ADMIN', {
    'GET /reports/daily': ok(daily),
    'GET /reports/outcomes': ok(outcomes),
  });
  await page.goto('/reports');

  const chart = page.getByRole('img', {
    name: /Bar chart of delivered and failed parcels/,
  });
  await expect(chart.locator('title')).toHaveCount(2);
  await expect(chart.locator('title').first()).toHaveText(
    '1 Nov 2026: 42 created, 35 delivered, 3 failed, 92% success',
  );
});

test('an all-in-progress range says that nothing finished', async ({
  page,
}) => {
  await signInAs(page, 'ADMIN', {
    'GET /reports/daily': ok(
      daily.map((row) => ({
        ...row,
        created: 10,
        delivered: 0,
        failed: 0,
      })),
    ),
    'GET /reports/outcomes': ok({
      ...outcomes,
      totals: { created: 20, delivered: 0, failed: 0, inProgress: 20 },
      successRate: 0,
      failureRate: 0,
      topFailureReasons: [],
    }),
  });
  await page.goto('/reports');

  await expect(
    page.getByText('No parcels finished in this range.'),
  ).toBeVisible();
  const chart = page.getByRole('img', {
    name: /No parcels finished in this range/,
  });
  await expect(chart).toBeVisible();
  await expect(
    chart.locator('rect.fill-success, rect.fill-danger'),
  ).toHaveCount(0);
  await expect(page.getByText(/tallest day has 1 finished/i)).toHaveCount(0);
  await expect(
    page.getByRole('table', { name: 'Daily parcels' }).getByRole('row'),
  ).toHaveCount(3);
});

test('a range of 31 days or fewer marks the days with nothing finished', async ({
  page,
}) => {
  await signInAs(page, 'ADMIN', {
    'GET /reports/daily': ok(sparseDays(31, 21)),
    'GET /reports/outcomes': ok(outcomes),
  });
  await page.goto('/reports');

  const chart = page.getByRole('img', {
    name: /Bar chart of delivered and failed parcels/,
  });
  await expect(chart.locator('rect.fill-success')).toHaveCount(10);
  await expect(chart.locator('rect.fill-line-strong')).toHaveCount(21);
});

for (const days of [32, 90]) {
  test(`a range of ${days} days does not mark empty days, so it is not a dashed line`, async ({
    page,
  }) => {
    await signInAs(page, 'ADMIN', {
      'GET /reports/daily': ok(sparseDays(days, days - 10)),
      'GET /reports/outcomes': ok(outcomes),
    });
    await page.goto('/reports');

    const chart = page.getByRole('img', {
      name: /Bar chart of delivered and failed parcels/,
    });
    await expect(chart.locator('rect.fill-success')).toHaveCount(10);
    await expect(chart.locator('rect.fill-line-strong')).toHaveCount(0);
  });
}

test('the caption says when nothing finished before the first busy day', async ({
  page,
}) => {
  await signInAs(page, 'ADMIN', {
    'GET /reports/daily': ok(sparseDays(7, 3)),
    'GET /reports/outcomes': ok(outcomes),
  });
  await page.goto('/reports');

  await expect(
    page.getByText('Nothing finished before 4 Oct 2026.'),
  ).toBeVisible();
});

test('the caption has no early-empty note when the first day has parcels finished', async ({
  page,
}) => {
  await signInAs(page, 'ADMIN', {
    'GET /reports/daily': ok(daily),
    'GET /reports/outcomes': ok(outcomes),
  });
  await page.goto('/reports');

  await expect(page.getByText(/Tallest day: 46\./)).toBeVisible();
  await expect(page.getByText(/Nothing finished before/)).toHaveCount(0);
});

test('the caption has no early-empty note when nothing finished in the range', async ({
  page,
}) => {
  await signInAs(page, 'ADMIN', {
    'GET /reports/daily': ok(sparseDays(7, 7)),
    'GET /reports/outcomes': ok(outcomes),
  });
  await page.goto('/reports');

  await expect(
    page.getByText('No parcels finished in this range.'),
  ).toBeVisible();
  await expect(page.getByText(/Nothing finished before/)).toHaveCount(0);
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
    'GET /reports/daily': ok(daily),
    'GET /reports/outcomes': ok(outcomes),
  });

  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route('**/api/v1/reports/**', async (route) => {
    await gate;
    await route.fallback();
  });

  await page.goto('/reports');
  await expect(page.getByText('Loading outcomes')).toBeAttached();
  release();
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

test('a 503 shows a reference, hides the server words and can be retried', async ({
  page,
}) => {
  let failing = true;
  await signInAs(page, 'ADMIN', {
    'GET /reports/daily': () =>
      failing ? fail(503, 'SERVICE_UNAVAILABLE') : ok(daily),
    'GET /reports/outcomes': () =>
      failing ? fail(503, 'SERVICE_UNAVAILABLE') : ok(outcomes),
  });
  await page.goto('/reports');

  // Next.js keeps an empty role="alert" route announcer on every page,
  // so pick ours by its text.
  const alert = page
    .getByRole('alert')
    .filter({ hasText: 'Something broke on our side' });
  await expect(alert).toContainText('Something broke on our side');
  await expect(alert).toContainText(REQUEST_ID);
  await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);

  failing = false;
  await page.getByRole('button', { name: 'Try again' }).click();
  await expect(page.getByText('94.37%')).toBeVisible();
  await expect(alert).toHaveCount(0);
});

test('a rate limit shows the wait and no server fault or retry', async ({
  page,
}) => {
  await signInAs(page, 'ADMIN', {
    'GET /reports/daily': fail(429, 'RATE_LIMITED', {
      headers: { 'Retry-After': '30' },
    }),
    'GET /reports/outcomes': fail(429, 'RATE_LIMITED', {
      headers: { 'Retry-After': '30' },
    }),
  });
  await page.goto('/reports');

  await expect(
    page.getByText('Too many attempts from this device'),
  ).toBeVisible();
  await expect(page.getByText(/Wait\s+\d+\s+seconds/)).toBeVisible();
  await expect(page.getByText('Something broke on our side')).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Try again' })).toHaveCount(0);
});

test('a server 422 puts our message on the date field and does not retry', async ({
  page,
}) => {
  await signInAs(page, 'ADMIN', {
    'GET /reports/daily': fail(422, 'VALIDATION_ERROR', {
      details: [{ field: 'to', issue: 'must not be before from' }],
    }),
    'GET /reports/outcomes': fail(422, 'VALIDATION_ERROR', {
      details: [{ field: 'to', issue: 'must not be before from' }],
    }),
  });
  await page.goto('/reports');

  await expect(
    page.getByText('Choose a valid end date within the 90-day range.'),
  ).toBeVisible();
  await expect(page.getByLabel('To', { exact: true })).toBeFocused();
  await expect(page.getByRole('button', { name: 'Try again' })).toHaveCount(0);
  await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);

  // The field already holds today (UTC), and typing the same value is not a
  // change, so a fixed date fails on that one day. Yesterday is always a real
  // change and always inside the default range.
  const yesterday = new Date(Date.now() - 86_400_000)
    .toISOString()
    .slice(0, 10);
  await page.getByLabel('To', { exact: true }).fill(yesterday);
  await expect(
    page.getByText('Choose a valid end date within the 90-day range.'),
  ).toHaveCount(0);
});

test('a server 403 shows the no-permission state and does not retry', async ({
  page,
}) => {
  await signInAs(page, 'ADMIN', {
    'GET /reports/daily': fail(403, 'FORBIDDEN'),
    'GET /reports/outcomes': fail(403, 'FORBIDDEN'),
  });
  await page.goto('/reports');

  await expect(
    page.getByRole('heading', { name: 'You do not have access' }),
  ).toBeVisible();
  await expect(
    page.getByText('This page is for dispatchers and administrators'),
  ).toBeVisible();
  await expect(page.getByRole('button', { name: 'Try again' })).toHaveCount(0);
  await expect(page.getByText('Something broke on our side')).toHaveCount(0);
});

test('a network failure can be retried', async ({ page }) => {
  await signInAs(page, 'ADMIN', {
    'GET /reports/daily': ok(daily),
    'GET /reports/outcomes': ok(outcomes),
  });
  await page.route('**/api/v1/reports/**', (route) => route.abort('failed'));
  await page.goto('/reports');

  const alert = page
    .getByRole('alert')
    .filter({ hasText: 'We could not reach SwiftDrop' });
  await expect(alert).toBeVisible();
  await expect(alert).not.toContainText(SERVER_WORDS);

  await page.unroute('**/api/v1/reports/**');
  await page.getByRole('button', { name: 'Try again' }).click();
  await expect(page.getByText('94.37%')).toBeVisible();
});

test('invalid submission focuses the first bad date and clears after a fix', async ({
  page,
}) => {
  await signInAs(page, 'ADMIN', {
    'GET /reports/daily': ok(daily),
    'GET /reports/outcomes': ok(outcomes),
  });
  await page.goto('/reports');
  await expect(page.getByText('94.37%')).toBeVisible();

  await page.getByLabel('From', { exact: true }).fill('2026-11-04');
  await page.getByLabel('To', { exact: true }).fill('2026-11-01');
  await page.getByLabel('From', { exact: true }).focus();
  await page.getByLabel('From', { exact: true }).press('Enter');
  await expect(
    page.getByText('The end date must be on or after the start date.'),
  ).toBeVisible();
  await expect(page.getByLabel('To', { exact: true })).toBeFocused();

  await page.getByLabel('To', { exact: true }).fill('2026-11-05');
  await expect(
    page.getByText('The end date must be on or after the start date.'),
  ).toHaveCount(0);

  await page.getByLabel('From', { exact: true }).fill('');
  await page.getByLabel('To', { exact: true }).focus();
  await page.getByLabel('To', { exact: true }).press('Enter');
  await expect(page.getByLabel('From', { exact: true })).toBeFocused();
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

test('the date controls and reset action meet the touch target on a phone', async ({
  page,
}) => {
  await page.setViewportSize({ width: 375, height: 812 });
  await signInAs(page, 'ADMIN', {
    'GET /reports/daily': ok(
      daily.map((row) => ({ ...row, created: 0, delivered: 0, failed: 0 })),
    ),
    'GET /reports/outcomes': ok(emptyOutcomes),
  });
  await page.goto('/reports');
  await expect(
    page.getByRole('heading', { name: 'No parcels in this range' }),
  ).toBeVisible();

  for (const control of [
    page.getByLabel('From', { exact: true }),
    page.getByLabel('To', { exact: true }),
    page.getByRole('button', { name: 'Apply range' }),
    page.getByRole('button', { name: 'Reset range' }),
  ]) {
    const box = await control.boundingBox();
    expect(box, 'the control is visible').not.toBeNull();
    expect(
      box?.height ?? 0,
      'the control is at least 44px tall',
    ).toBeGreaterThanOrEqual(44);
  }
});

test.describe('dark theme', () => {
  test.use({ colorScheme: 'dark' });

  test('the reports screen follows the dark tokens', async ({ page }) => {
    await signInAs(page, 'ADMIN', {
      'GET /reports/daily': ok(daily),
      'GET /reports/outcomes': ok(outcomes),
    });
    await page.goto('/reports');

    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
    await expect(page.getByText('94.37%')).toBeVisible();
    await expect(
      page.getByRole('table', { name: 'Daily parcels' }),
    ).toBeVisible();
  });
});
