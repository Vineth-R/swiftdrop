import { test, expect } from '@playwright/test';

import { fail, REQUEST_ID, SERVER_WORDS, type Reply } from './support/mock-api';
import { signInAs } from './support/sign-in';

const log = {
  id: '8a0a6a4e-0000-4000-8000-000000000001',
  eventId: '5e4f6a7b-0000-4000-8000-000000000001',
  parcelId: '9b8c7d6e-0000-4000-8000-000000000001',
  channel: 'EMAIL',
  recipient: 'n***@example.com',
  subject: 'Your parcel is out for delivery',
  status: 'SENT',
  errorMessage: null,
  createdAt: '2026-11-03T10:00:00Z',
  sentAt: '2026-11-03T10:00:02Z',
} as const;

const failed = {
  ...log,
  id: '8a0a6a4e-0000-4000-8000-000000000002',
  eventId: '5e4f6a7b-0000-4000-8000-000000000002',
  parcelId: '9b8c7d6e-0000-4000-8000-000000000002',
  recipient: 'k***@example.com',
  subject: 'Delivery attempt failed',
  status: 'FAILED',
  errorMessage: 'The mailbox rejected the message.',
  sentAt: null,
} as const;

const pageOf = (
  data: readonly unknown[],
  pagination = {
    page: 1,
    pageSize: 20,
    totalItems: data.length,
    totalPages: data.length > 0 ? 1 : 0,
  },
): Reply => ({
  status: 200,
  body: {
    data,
    meta: {
      requestId: REQUEST_ID,
      timestamp: '2026-11-03T10:00:00Z',
      pagination,
    },
  },
});

const twoPageHandler = ({ query }: { query: URLSearchParams }): Reply => {
  const page = Number(query.get('page') ?? 1);
  const status = query.get('status');
  const items =
    status === 'FAILED'
      ? page === 2
        ? [{ ...failed, id: 'failed-page-2', recipient: 'f***@example.com' }]
        : [{ ...failed, id: 'failed-page-1', recipient: 'k***@example.com' }]
      : page === 2
        ? [{ ...log, id: 'page-2', recipient: 'p***@example.com' }]
        : [log];

  return pageOf(items, {
    page,
    pageSize: 20,
    totalItems: 21,
    totalPages: 2,
  });
};

test.use({ colorScheme: 'light' });

test.describe('notification logs', () => {
  test.beforeEach(async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
  });

  test('shows masked recipients, statuses and the newest delivery attempt', async ({
    page,
  }) => {
    await signInAs(page, 'ADMIN', {
      'GET /admin/notification-logs': pageOf([failed, log]),
    });
    await page.goto('/admin/notification-logs');

    await expect(
      page.getByRole('heading', { level: 1, name: 'Notification logs' }),
    ).toBeVisible();
    const table = page.getByRole('table');
    await expect(table.getByText('n***@example.com')).toBeVisible();
    await expect(table.getByText('k***@example.com')).toBeVisible();
    await expect(table.getByText('SENT', { exact: true })).toBeVisible();
    await expect(table.getByText('FAILED', { exact: true })).toBeVisible();
    await expect(
      table.getByText('Your parcel is out for delivery'),
    ).toBeVisible();
    await expect(
      table.getByText('The mailbox rejected the message.'),
    ).toBeVisible();
    await expect(page.getByText('nimal@example.com')).toHaveCount(0);
  });

  test('filtering resets pagination to page one', async ({ page }) => {
    const api = await signInAs(page, 'ADMIN', {
      'GET /admin/notification-logs': twoPageHandler,
    });
    await page.goto('/admin/notification-logs');
    await page.getByRole('button', { name: 'Go to next page' }).click();
    await expect(
      page.getByRole('table').getByText('p***@example.com'),
    ).toBeVisible();

    await page
      .getByRole('combobox', { name: 'Filter by delivery status' })
      .click();
    await page.getByRole('option', { name: 'FAILED' }).click();

    await expect(
      page.getByRole('table').getByText('k***@example.com'),
    ).toBeVisible();
    const filteredCall = api.calls
      .filter((call) => call.key === 'GET /admin/notification-logs')
      .map((call) => new URL(call.url))
      .find((url) => url.searchParams.get('status') === 'FAILED');
    expect(filteredCall?.searchParams.get('page')).toBe('1');
    expect(filteredCall?.searchParams.get('status')).toBe('FAILED');
  });

  test('pagination preserves the active filter', async ({ page }) => {
    const api = await signInAs(page, 'ADMIN', {
      'GET /admin/notification-logs': twoPageHandler,
    });
    await page.goto('/admin/notification-logs');

    const filter = page.getByRole('combobox', {
      name: 'Filter by delivery status',
    });
    await filter.click();
    await page.getByRole('option', { name: 'FAILED' }).click();
    await expect(filter).toContainText('FAILED');

    await page.getByRole('button', { name: 'Go to next page' }).click();
    await expect(
      page.getByRole('table').getByText('f***@example.com'),
    ).toBeVisible();
    await expect(filter).toContainText('FAILED');

    const filteredPageTwo = api.calls
      .filter((call) => call.key === 'GET /admin/notification-logs')
      .map((call) => new URL(call.url))
      .find(
        (url) =>
          url.searchParams.get('page') === '2' &&
          url.searchParams.get('status') === 'FAILED',
      );
    expect(filteredPageTwo).toBeTruthy();
  });

  test('the default filter omits the status query parameter', async ({
    page,
  }) => {
    const api = await signInAs(page, 'ADMIN', {
      'GET /admin/notification-logs': pageOf([log]),
    });
    await page.goto('/admin/notification-logs');

    await expect
      .poll(() =>
        api.calls.some((call) => {
          if (call.key !== 'GET /admin/notification-logs') return false;
          const url = new URL(call.url);
          return (
            url.searchParams.get('page') === '1' &&
            !url.searchParams.has('status')
          );
        }),
      )
      .toBe(true);
  });

  test('retry issues a second request after a server error', async ({
    page,
  }) => {
    let allowSuccess = false;
    const api = await signInAs(page, 'ADMIN', {
      'GET /admin/notification-logs': () =>
        allowSuccess ? pageOf([log]) : fail(500, 'INTERNAL_ERROR'),
    });
    await page.goto('/admin/notification-logs');

    const alert = page.locator('[role="alert"]:not(#__next-route-announcer__)');
    await expect(alert).toContainText('Notification logs could not be loaded');
    allowSuccess = true;
    await page.getByRole('button', { name: 'Try again' }).click();
    await expect(
      page.getByRole('table').getByText('n***@example.com'),
    ).toBeVisible();
    expect(
      api.calls.filter((call) => call.key === 'GET /admin/notification-logs')
        .length,
    ).toBeGreaterThanOrEqual(2);
  });

  test('an API 403 shows the safe access error', async ({ page }) => {
    await signInAs(page, 'ADMIN', {
      'GET /admin/notification-logs': fail(403, 'FORBIDDEN'),
    });
    await page.goto('/admin/notification-logs');

    const alert = page.locator('[role="alert"]:not(#__next-route-announcer__)');
    await expect(alert).toContainText(
      'You do not have access to notification logs',
    );
    await expect(alert).toContainText(
      'This view is limited to administrators.',
    );
    await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
  });

  test('an API 401 ends the session instead of showing server text', async ({
    page,
  }) => {
    await signInAs(page, 'ADMIN', {
      'GET /admin/notification-logs': fail(401, 'UNAUTHENTICATED'),
    });
    await page.goto('/admin/notification-logs');

    await expect(page).toHaveURL(/\/login\?next=%2Fadmin%2Fnotification-logs$/);
    await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
  });

  test('a network failure shows our message', async ({ page }) => {
    await signInAs(page, 'ADMIN', {
      'GET /admin/notification-logs': pageOf([log]),
    });
    await page.route('**/api/v1/admin/notification-logs**', (route) =>
      route.abort(),
    );
    await page.goto('/admin/notification-logs');

    const alert = page.locator('[role="alert"]:not(#__next-route-announcer__)');
    await expect(alert).toContainText('Notification logs could not be loaded');
    await expect(alert).toContainText(
      'The notification worker did not answer. Try again in a moment.',
    );
    await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
  });

  test('shows an empty state when there are no delivery attempts', async ({
    page,
  }) => {
    await signInAs(page, 'ADMIN', {
      'GET /admin/notification-logs': pageOf([]),
    });
    await page.goto('/admin/notification-logs');

    await expect(
      page.getByRole('heading', { name: 'No notification logs yet' }),
    ).toBeVisible();
    await expect(
      page.locator('[data-slot="empty"]').getByRole('button', {
        name: 'Refresh',
      }),
    ).toBeVisible();
  });

  test('a filter with no results shows the filtered empty state', async ({
    page,
  }) => {
    await signInAs(page, 'ADMIN', {
      'GET /admin/notification-logs': pageOf([]),
    });
    await page.goto('/admin/notification-logs');
    await page
      .getByRole('combobox', { name: 'Filter by delivery status' })
      .click();
    await page.getByRole('option', { name: 'FAILED' }).click();

    await expect(
      page.getByRole('heading', { name: 'No failed logs' }),
    ).toBeVisible();
  });

  test('a null subject is shown as No subject', async ({ page }) => {
    await signInAs(page, 'ADMIN', {
      'GET /admin/notification-logs': pageOf([
        { ...log, subject: null, sentAt: null },
      ]),
    });
    await page.goto('/admin/notification-logs');

    const table = page.getByRole('table');
    await expect(table.getByText('No subject')).toBeVisible();
    await expect(table.getByText('—')).toBeVisible();
  });

  test('a dispatcher sees the 403 page inside the shell', async ({ page }) => {
    await signInAs(page, 'DISPATCHER');
    await page.goto('/admin/notification-logs');

    await expect(
      page.getByRole('heading', { name: 'You do not have access' }),
    ).toBeVisible();
  });

  test('uses cards instead of a table on a phone', async ({ page }) => {
    await page.setViewportSize({ width: 360, height: 800 });
    await signInAs(page, 'ADMIN', {
      'GET /admin/notification-logs': pageOf([log]),
    });
    await page.goto('/admin/notification-logs');

    await expect(page.locator('table')).toBeHidden();
    await expect(page.locator('article')).toContainText('n***@example.com');
    await expect(page.locator('article')).toContainText('SENT');
  });
});

test.describe('notification logs in dark mode', () => {
  test.use({ colorScheme: 'dark' });

  test('renders with the dark tokens', async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
    await signInAs(page, 'ADMIN', {
      'GET /admin/notification-logs': pageOf([log]),
    });
    await page.goto('/admin/notification-logs');

    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
    await expect(page.locator('body')).toHaveCSS(
      'background-color',
      'rgb(14, 17, 19)',
    );
    await expect(
      page.getByRole('table').getByText('n***@example.com'),
    ).toBeVisible();
  });
});
