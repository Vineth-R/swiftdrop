import { test, expect, type Page, type Route } from '@playwright/test';

import {
  fail,
  mockApi,
  ok,
  REQUEST_ID,
  SERVER_WORDS,
  signedOut,
  type Reply,
} from './support/mock-api';
import { signInAs, type Role } from './support/sign-in';

const endpoint = 'GET /courier/deliveries';
const listUrl = '**/api/v1/courier/deliveries?**';
const now = new Date('2026-11-04T18:45:00Z');
const today = '2026-11-05';

// Synthetic fixtures follow API v1.1 §6.1; no completion time or reason is invented.
const delivered = {
  id: '00000000-0000-4000-8000-000000000001',
  trackingNumber: 'SDHISTORY001',
  status: 'DELIVERED',
  pickupAddress: '10 Test Road',
  dropoffAddress: '45 Lake Drive',
  dropoffCity: 'Kandy',
  recipientName: 'History Test Recipient',
  recipientPhone: '+94770000000',
  weightKg: 2,
  assignedAt: '2026-11-04T09:00:00Z',
};
const failed = {
  ...delivered,
  id: '00000000-0000-4000-8000-000000000002',
  trackingNumber: 'SDHISTORY002',
  status: 'FAILED',
};
const second = {
  ...delivered,
  id: '00000000-0000-4000-8000-000000000003',
  trackingNumber: 'SDHISTORY003',
};
const active = {
  ...delivered,
  id: '00000000-0000-4000-8000-000000000004',
  trackingNumber: 'SDHISTORY004',
  status: 'PICKED_UP',
};

function pageOf(data: unknown[], page = 1, totalItems = data.length): Reply {
  return {
    status: 200,
    body: {
      data,
      meta: {
        requestId: REQUEST_ID,
        pagination: {
          page,
          pageSize: 20,
          totalItems,
          totalPages: Math.ceil(totalItems / 20),
        },
      },
    },
  };
}

const answer = (route: Route, reply: Reply) =>
  route.fulfill({
    status: reply.status,
    contentType: 'application/json',
    body: JSON.stringify(reply.body),
  });

function deferred() {
  let release!: () => void;
  const promise = new Promise<void>((resolve) => {
    release = resolve;
  });
  return { promise, release };
}

const history = (page: Page) =>
  page.getByRole('region', { name: 'Deliveries' });
const table = (page: Page) => page.getByRole('table', { name: 'Deliveries' });
const summary = (page: Page) =>
  history(page)
    .getByRole('status')
    .filter({ hasText: /^\d+ deliver(y|ies) on/ });
const next = (page: Page) =>
  page.getByRole('button', { name: 'Go to next page' });
const previous = (page: Page) =>
  page.getByRole('button', { name: 'Go to previous page' });

function lastDeliveryQuery(calls: Array<{ key: string; url: string }>) {
  const call = calls.filter((item) => item.key === endpoint).at(-1);
  if (!call) throw new Error('Expected a courier delivery request');
  return new URL(call.url).searchParams;
}

test.use({
  colorScheme: 'light',
  timezoneId: 'America/Los_Angeles',
  // Local runs may reuse installed Chrome; CI defaults to bundled Chromium.
  channel: process.env['PLAYWRIGHT_CHANNEL'],
});

test.beforeEach(async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.clock.setFixedTime(now);
});

test('shows assigned history using the documented query and fields', async ({
  page,
}) => {
  const api = await signInAs(page, 'COURIER', {
    [endpoint]: pageOf([delivered]),
  });
  await page.goto('/history');
  await expect(page).toHaveTitle('History — SwiftDrop');
  await expect(
    page.getByRole('heading', { level: 1, name: 'History' }),
  ).toBeVisible();
  await expect(table(page).getByText(delivered.trackingNumber)).toBeVisible();
  await expect(
    table(page).getByText('DELIVERED', { exact: true }),
  ).toBeVisible();
  await expect(table(page).getByText(delivered.recipientName)).toBeVisible();
  await expect(table(page).getByText('45 Lake Drive, Kandy')).toBeVisible();
  await expect(summary(page)).toHaveText(`1 delivery on ${today}.`);
  await expect(page.getByLabel('Delivery date')).toHaveValue(today);
  await expect(page.getByLabel('Delivery status')).toHaveValue('');
  expect(Object.fromEntries(lastDeliveryQuery(api.calls))).toEqual({
    date: today,
    page: '1',
    pageSize: '20',
  });
  await expect(
    page
      .getByRole('navigation', { name: 'Main', exact: true })
      .getByRole('link', { name: 'History' }),
  ).toHaveAttribute('aria-current', 'page');
});

test('defaults to all statuses so active deliveries are not hidden', async ({
  page,
}) => {
  const api = await signInAs(page, 'COURIER', {
    [endpoint]: pageOf([active, failed]),
  });
  await page.goto('/history');
  await expect(page.getByLabel('Delivery status')).toHaveValue('');
  await expect(table(page).getByText(active.trackingNumber)).toBeVisible();
  await expect(
    table(page).getByText('PICKED UP', { exact: true }),
  ).toBeVisible();
  await expect(summary(page)).toHaveText(`2 deliveries on ${today}.`);
  expect(lastDeliveryQuery(api.calls).has('status')).toBe(false);
});

for (const [instant, expected] of [
  ['2026-11-04T18:29:59Z', '2026-11-04'],
  ['2026-11-04T18:30:00Z', '2026-11-05'],
]) {
  test(`defaults to Colombo's date at ${instant}`, async ({ page }) => {
    await page.clock.setFixedTime(new Date(instant));
    const api = await signInAs(page, 'COURIER', { [endpoint]: pageOf([]) });
    await page.goto('/history');
    await expect(page.getByLabel('Delivery date')).toHaveValue(expected);
    await expect
      .poll(() =>
        api.calls
          .filter((item) => item.key === endpoint)
          .map((item) => new URL(item.url).searchParams.get('date')),
      )
      .toContain(expected);
  });
}

test('shows loading skeletons and prevents duplicate refreshes', async ({
  page,
}) => {
  await signInAs(page, 'COURIER');
  const gate = deferred();
  await page.route(listUrl, async (route) => {
    await gate.promise;
    await answer(route, pageOf([delivered]));
  });
  try {
    await page.goto('/history');
    await expect(
      history(page)
        .getByRole('status')
        .filter({ hasText: 'Loading deliveries' }),
    ).toHaveAttribute('aria-busy', 'true');
    await expect(
      page.getByRole('button', { name: 'Loading history' }),
    ).toBeDisabled();
    await expect(history(page).getByText(delivered.trackingNumber)).toHaveCount(
      0,
    );
  } finally {
    gate.release();
  }
  await expect(table(page).getByText(delivered.trackingNumber)).toBeVisible();
});

test('shows an empty state without made-up deliveries', async ({ page }) => {
  await signInAs(page, 'COURIER', { [endpoint]: pageOf([]) });
  await page.goto('/history');
  await expect(
    page.getByRole('heading', { name: 'No deliveries' }),
  ).toBeVisible();
  await expect(summary(page)).toHaveText(`0 deliveries on ${today}.`);
  await expect(table(page)).toHaveCount(0);
  await expect(
    page.getByRole('navigation', { name: 'Delivery history pages' }),
  ).toHaveCount(0);
});

test('keeps filters on a server failure, hides server text and retries', async ({
  page,
}) => {
  let hasError = false;
  const api = await signInAs(page, 'COURIER', {
    [endpoint]: () =>
      hasError ? fail(500, 'INTERNAL_ERROR') : pageOf([delivered]),
  });
  await page.goto('/history');
  await expect(table(page).getByText(delivered.trackingNumber)).toBeVisible();
  hasError = true;
  await page.getByLabel('Delivery date').fill('2026-11-03');
  await expect(history(page).getByRole('alert')).toContainText(
    'Something broke on our side',
  );
  await expect(history(page).getByRole('alert')).toContainText(
    `Reference ${REQUEST_ID}`,
  );
  await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
  await expect(page.getByLabel('Delivery date')).toHaveValue('2026-11-03');
  await expect(history(page).getByText(delivered.trackingNumber)).toHaveCount(
    0,
  );
  hasError = false;
  await page.getByRole('button', { name: 'Try again' }).click();
  await expect(table(page).getByText(delivered.trackingNumber)).toBeVisible();
  expect(lastDeliveryQuery(api.calls).get('date')).toBe('2026-11-03');
});

test('shows a safe network error', async ({ page }) => {
  await signInAs(page, 'COURIER');
  await page.route(listUrl, (route) => route.abort('failed'));
  await page.goto('/history');
  await expect(history(page).getByRole('alert')).toContainText(
    'We could not reach SwiftDrop',
  );
  await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Try again' })).toBeEnabled();
});

test('an API 403 shows no permission and offers no retry', async ({ page }) => {
  await signInAs(page, 'COURIER', { [endpoint]: fail(403, 'FORBIDDEN') });
  await page.goto('/history');
  await expect(history(page).getByRole('alert')).toContainText(
    'You do not have access to delivery history',
  );
  await expect(page.getByRole('button', { name: 'Try again' })).toHaveCount(0);
  await expect(page.getByLabel('Delivery date')).toBeDisabled();
  await expect(page.getByText(REQUEST_ID)).toHaveCount(0);
  await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
});

test('signed-out visitors return to login without requesting deliveries', async ({
  page,
}) => {
  const api = await mockApi(page, signedOut);
  await page.goto('/history');
  await expect(page).toHaveURL(/\/login\?next=%2Fhistory$/);
  await expect(page.getByLabel('Email', { exact: true })).toBeVisible();
  expect(api.calls.filter((item) => item.key === endpoint)).toHaveLength(0);
});

for (const role of ['CUSTOMER', 'MERCHANT', 'DISPATCHER', 'ADMIN'] as Role[]) {
  test(`${role} cannot access History or request courier data`, async ({
    page,
  }) => {
    const api = await signInAs(page, role, { [endpoint]: pageOf([delivered]) });
    await page.goto('/history');
    await expect(
      page.getByRole('heading', { name: 'You do not have access' }),
    ).toBeVisible();
    await expect(
      page.getByRole('link', { name: 'Back to my dashboard' }),
    ).toBeVisible();
    await expect(page.getByLabel('Delivery date')).toHaveCount(0);
    expect(api.calls.filter((item) => item.key === endpoint)).toHaveLength(0);
  });
}

test('pages both ways, preserves filters and resets the page on each filter change', async ({
  page,
}) => {
  const api = await signInAs(page, 'COURIER', {
    [endpoint]: ({ query }) => {
      const pageNumber = Number(query.get('page'));
      const row = query.get('status') === 'FAILED' ? failed : delivered;
      return pageOf(
        [
          {
            ...row,
            ...(pageNumber === 2
              ? { id: second.id, trackingNumber: second.trackingNumber }
              : {}),
          },
        ],
        pageNumber,
        21,
      );
    },
  });
  await page.goto('/history');
  await expect(previous(page)).toBeDisabled();
  await next(page).click();
  await expect(table(page).getByText(second.trackingNumber)).toBeVisible();
  await expect(next(page)).toBeDisabled();
  await expect(
    page.getByRole('navigation', { name: 'Delivery history pages' }),
  ).toContainText('Page 2 of 2');
  await previous(page).click();
  await expect(table(page).getByText(delivered.trackingNumber)).toBeVisible();
  await next(page).click();
  await expect(table(page).getByText(second.trackingNumber)).toBeVisible();
  await page.getByLabel('Delivery status').selectOption('FAILED');
  await expect(table(page).getByText(failed.trackingNumber)).toBeVisible();
  await expect(previous(page)).toBeDisabled();
  await next(page).click();
  await expect(table(page).getByText(second.trackingNumber)).toBeVisible();
  let query = lastDeliveryQuery(api.calls);
  expect(Object.fromEntries(query)).toEqual({
    date: today,
    status: 'FAILED',
    page: '2',
    pageSize: '20',
  });
  await page.getByLabel('Delivery date').fill('2026-11-02');
  await expect(table(page).getByText(failed.trackingNumber)).toBeVisible();
  await expect(previous(page)).toBeDisabled();
  query = lastDeliveryQuery(api.calls);
  expect(Object.fromEntries(query)).toEqual({
    date: '2026-11-02',
    status: 'FAILED',
    page: '1',
    pageSize: '20',
  });
});

test('clearing the date clears old results and makes no invalid request', async ({
  page,
}) => {
  const api = await signInAs(page, 'COURIER', {
    [endpoint]: pageOf([delivered]),
  });
  await page.goto('/history');
  await expect(table(page).getByText(delivered.trackingNumber)).toBeVisible();
  const count = api.calls.filter((item) => item.key === endpoint).length;
  await page.getByLabel('Delivery date').fill('');
  await page.getByLabel('Delivery status').focus();
  await expect(
    page.getByRole('heading', { name: 'Choose a delivery date' }),
  ).toBeVisible();
  await expect(page.getByLabel('Delivery date')).toHaveAttribute(
    'aria-invalid',
    'true',
  );
  await expect(page.getByRole('button', { name: 'Refresh' })).toBeDisabled();
  await expect(history(page).getByText(delivered.trackingNumber)).toHaveCount(
    0,
  );
  expect(api.calls.filter((item) => item.key === endpoint)).toHaveLength(count);
});

for (const oldReply of ['success', 'failure'] as const) {
  test(`aborts an outdated request and ignores its late ${oldReply}`, async ({
    page,
  }) => {
    await signInAs(page, 'COURIER', { [endpoint]: pageOf([delivered]) });
    await page.goto('/history');
    await expect(table(page).getByText(delivered.trackingNumber)).toBeVisible();
    const gate = deferred();
    const answered = deferred();
    await page.route(listUrl, async (route) => {
      const query = new URL(route.request().url()).searchParams;
      if (query.get('date') === '2026-11-01') {
        await gate.promise;
        try {
          await answer(
            route,
            oldReply === 'success'
              ? pageOf([second])
              : fail(500, 'INTERNAL_ERROR'),
          );
        } finally {
          answered.release();
        }
      } else {
        await answer(route, pageOf([failed]));
      }
    });
    const oldRequest = page.waitForRequest((request) =>
      request.url().includes('date=2026-11-01'),
    );
    await page.getByLabel('Delivery date').fill('2026-11-01');
    const request = await oldRequest;
    await expect(history(page).getByText(delivered.trackingNumber)).toHaveCount(
      0,
    );
    const aborted = page.waitForEvent(
      'requestfailed',
      (item) => item === request,
    );
    try {
      await page.getByLabel('Delivery status').selectOption('FAILED');
      await page.getByLabel('Delivery date').fill('2026-11-02');
      await aborted;
      await expect(table(page).getByText(failed.trackingNumber)).toBeVisible();
    } finally {
      gate.release();
    }
    await answered.promise;
    await expect(table(page).getByText(failed.trackingNumber)).toBeVisible();
    await expect(history(page).getByText(second.trackingNumber)).toHaveCount(0);
    await expect(history(page).getByRole('alert')).toHaveCount(0);
    await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
  });
}

test('missing pagination is an error, not a fabricated result', async ({
  page,
}) => {
  await signInAs(page, 'COURIER', { [endpoint]: ok([delivered]) });
  await page.goto('/history');
  await expect(history(page).getByRole('alert')).toContainText(
    'Something broke on our side',
  );
  await expect(table(page)).toHaveCount(0);
  await expect(history(page).getByText(delivered.trackingNumber)).toHaveCount(
    0,
  );
});

test('an unavailable endpoint shows no fabricated data or empty success', async ({
  page,
}) => {
  // An unhandled endpoint gets the helper's real-contract 404 response.
  const api = await signInAs(page, 'COURIER');
  await page.goto('/history');
  await expect(history(page)).toContainText(
    'Delivery history is not available yet',
  );
  await expect(table(page)).toHaveCount(0);
  await expect(history(page).getByRole('listitem')).toHaveCount(0);
  await expect(
    page.getByRole('heading', { name: 'No deliveries' }),
  ).toHaveCount(0);
  await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
  await expect(page.getByText(REQUEST_ID)).toHaveCount(0);
  expect(api.calls.some((item) => item.key === endpoint)).toBe(true);
});

test('honours Retry-After without clearing filters', async ({ page }) => {
  await page.clock.install({ time: now });
  let hasError = true;
  await signInAs(page, 'COURIER', {
    [endpoint]: () =>
      hasError
        ? fail(429, 'RATE_LIMITED', { headers: { 'Retry-After': '2' } })
        : pageOf([delivered]),
  });
  await page.goto('/history');
  await expect(history(page)).toContainText(
    'Too many attempts from this device',
  );
  await expect(page.getByRole('button', { name: 'Try again' })).toBeDisabled();
  await expect(page.getByLabel('Delivery date')).toBeDisabled();
  await expect(page.getByLabel('Delivery status')).toBeDisabled();
  await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
  hasError = false;
  await page.clock.runFor(2200);
  await expect(page.getByRole('button', { name: 'Try again' })).toBeEnabled();
  await page.getByRole('button', { name: 'Try again' }).click();
  await expect(table(page).getByText(delivered.trackingNumber)).toBeVisible();
  await expect(page.getByLabel('Delivery date')).toHaveValue(today);
});

test('an expired API session returns to login safely', async ({ page }) => {
  await signInAs(page, 'COURIER', { [endpoint]: fail(401, 'UNAUTHENTICATED') });
  await page.goto('/history');
  await expect(page).toHaveURL(/\/login\?next=%2Fhistory$/);
  await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
});

test('missing optional contact information is labelled without invented details', async ({
  page,
}) => {
  await signInAs(page, 'COURIER', {
    [endpoint]: pageOf([
      {
        ...delivered,
        recipientName: null,
        dropoffAddress: '',
        dropoffCity: null,
      },
    ]),
  });
  await page.goto('/history');
  await expect(table(page).getByText('Not provided')).toHaveCount(2);
  await expect(table(page).getByText(delivered.recipientName)).toHaveCount(0);
});

test('filter controls have visible focus', async ({ page }) => {
  await signInAs(page, 'COURIER', { [endpoint]: pageOf([delivered]) });
  await page.goto('/history');
  await expect(table(page).getByText(delivered.trackingNumber)).toBeVisible();
  await page.getByLabel('Delivery date').focus();
  await expect(page.getByLabel('Delivery date')).toBeFocused();
  await expect(page.getByLabel('Delivery date')).toHaveCSS(
    'outline-style',
    /solid|auto/,
  );
  await expect
    .poll(() =>
      page.evaluate<number>(
        "parseFloat(getComputedStyle(document.getElementById('courier-history-date')).outlineWidth)",
      ),
    )
    .toBeGreaterThanOrEqual(2);
  // Chromium exposes date-input segments as native keyboard stops. Focus each
  // control directly so this test verifies the shared visual focus contract
  // without asserting browser-specific segment navigation.
  await page.getByLabel('Delivery status').focus();
  await expect(page.getByLabel('Delivery status')).toHaveCSS(
    'outline-style',
    /solid|auto/,
  );
  await expect
    .poll(() =>
      page.evaluate<number>(
        "parseFloat(getComputedStyle(document.getElementById('courier-history-status')).outlineWidth)",
      ),
    )
    .toBeGreaterThanOrEqual(2);
  await expect(page.getByLabel('Delivery status')).toBeFocused();
});

for (const width of [360, 1280]) {
  for (const theme of ['light', 'dark'] as const) {
    const layout = { isTable: width >= 768, cardCount: width < 768 ? 1 : 0 };
    test(`${width}px ${theme}: responsive layout, readable statuses and 48px courier controls`, async ({
      page,
    }) => {
      await page.setViewportSize({ width, height: 900 });
      await page.emulateMedia({ colorScheme: theme });
      await signInAs(page, 'COURIER', {
        [endpoint]: pageOf(
          [
            {
              ...delivered,
              recipientName: 'Recipient'.repeat(15),
              dropoffAddress: 'Destination'.repeat(20),
            },
          ],
          1,
          21,
        ),
      });
      await page.goto('/history');
      await expect(page.locator('html')).toHaveAttribute('data-theme', theme);
      await expect(summary(page)).toHaveText(`21 deliveries on ${today}.`);
      await expect(table(page)).toBeVisible({ visible: layout.isTable });
      await expect(
        history(page)
          .getByRole('listitem')
          .filter({ hasText: delivered.trackingNumber }),
      ).toHaveCount(layout.cardCount);
      await expect(
        history(page)
          .getByText('DELIVERED', { exact: true })
          .filter({ visible: true }),
      ).toBeVisible();
      const main = page.getByRole('main');
      for (const control of [
        main.getByLabel('Delivery date'),
        main.getByLabel('Delivery status'),
        main.getByRole('button', { name: 'Refresh' }),
        previous(page),
        next(page),
      ]) {
        await expect(control).toBeVisible();
        const box = await control.boundingBox();
        expect(box?.height).toBeGreaterThanOrEqual(48);
        expect(box?.width).toBeGreaterThanOrEqual(48);
      }
      expect(
        await page.evaluate(
          'document.documentElement.scrollWidth <= window.innerWidth',
        ),
      ).toBe(true);
      await page.screenshot({
        path: test.info().outputPath(`history-${width}-${theme}.png`),
        fullPage: true,
      });
    });
  }
}
