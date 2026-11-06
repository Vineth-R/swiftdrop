import { test, expect, type Page, type Route } from '@playwright/test';

import {
  fail,
  ok,
  REQUEST_ID,
  SERVER_WORDS,
  user,
  type Reply,
} from './support/mock-api';
import { signInAs } from './support/sign-in';

test.use({ colorScheme: 'light' });

const row = (id: string, changes: Record<string, unknown> = {}) => ({
  id,
  email: `${id}@example.com`,
  fullName: `Person ${id}`,
  phone: '',
  role: 'CUSTOMER',
  isActive: true,
  createdAt: '2026-10-01T08:00:00Z',
  ...changes,
});

/** The signed-in admin, as the list returns them. */
const self = row(user.id, {
  fullName: 'Anusha Nawarathna',
  email: 'admin@example.com',
  role: 'ADMIN',
});
const kasun = row('kasun', { fullName: 'Kasun Silva', role: 'COURIER' });
const ruwan = row('ruwan', {
  fullName: 'Ruwan Jayasuriya',
  role: 'MERCHANT',
  isActive: false,
});

/** A page of the list: the envelope carries the pagination. */
const list = (
  data: unknown[],
  pagination = {
    page: 1,
    pageSize: 20,
    totalItems: data.length,
    totalPages: 1,
  },
): Reply => ({
  status: 200,
  body: {
    data,
    meta: { requestId: REQUEST_ID, pagination },
  },
});

const listUrl = '**/api/v1/admin/users/**';

/** Answers one intercepted list request with a mock reply. */
const answer = (route: Route, reply: Reply) =>
  route.fulfill({
    status: reply.status,
    contentType: 'application/json',
    body: JSON.stringify(reply.body),
  });

/** The `page` query of an intercepted list request. */
const requestedPage = (route: Route) =>
  new URL(route.request().url()).searchParams.get('page');

/**
 * The pages asked for, in order, with a repeat of the same page folded into
 * one: React's development mode runs the first load's effect twice.
 */
const inOrder = (pages: string[]) =>
  pages.filter((value, index) => value !== pages[index - 1]);

// Below 768px the same rows are cards; at the 1440px these tests use it is the table.
const users = (page: Page) => page.getByRole('table', { name: 'Users' });
const person = (page: Page, name: string) =>
  users(page).getByRole('row').filter({ hasText: name });

test.describe('admin users list', () => {
  test.beforeEach(async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
  });

  test('shows each user with role and active status', async ({ page }) => {
    await signInAs(page, 'ADMIN', {
      'GET /admin/users/': list([self, kasun, ruwan]),
    });
    await page.goto('/admin/users');

    await expect(
      page.getByRole('heading', { level: 1, name: 'Users' }),
    ).toBeVisible();
    // Three users and the header row.
    await expect(users(page).getByRole('row')).toHaveCount(4);

    const courier = person(page, 'Kasun Silva');
    await expect(courier).toContainText('kasun@example.com');
    await expect(courier).toContainText('COURIER');
    await expect(courier).toContainText('ACTIVE');
    await expect(
      courier.getByRole('button', { name: 'Deactivate Kasun Silva' }),
    ).toBeVisible();

    const merchant = person(page, 'Ruwan Jayasuriya');
    await expect(merchant).toContainText('MERCHANT');
    await expect(merchant).toContainText('DEACTIVATED');
    await expect(
      merchant.getByRole('button', { name: 'Activate Ruwan Jayasuriya' }),
    ).toBeVisible();
    await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
  });

  test('asks for the first page of 20', async ({ page }) => {
    await signInAs(page, 'ADMIN', {
      'GET /admin/users/': list([self]),
    });
    const request = page.waitForRequest(listUrl);
    await page.goto('/admin/users');

    const url = new URL((await request).url());
    expect(url.searchParams.get('page')).toBe('1');
    expect(url.searchParams.get('pageSize')).toBe('20');
    expect(url.searchParams.has('q')).toBe(false);
  });

  test('searches by name or email and can clear the search', async ({
    page,
  }) => {
    await signInAs(page, 'ADMIN', {
      'GET /admin/users/': list([self, kasun]),
    });
    await page.goto('/admin/users');
    await expect(users(page).getByRole('row')).toHaveCount(3);

    await page.route(listUrl, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(list([]).body),
      });
    });
    const request = page.waitForRequest(
      (r) => new URL(r.url()).searchParams.get('q') === 'zzz',
    );
    await page.getByLabel('Search users').fill('  zzz ');
    await page.getByRole('button', { name: 'Search' }).click();

    expect(new URL((await request).url()).searchParams.get('page')).toBe('1');
    await expect(
      page.getByRole('heading', { name: 'No users match' }),
    ).toBeVisible();

    await page
      .getByRole('button', { name: 'Clear search and filters' })
      .click();
    await expect(page.getByLabel('Search users')).toHaveValue('');
  });

  test('pages through the list', async ({ page }) => {
    await signInAs(page, 'ADMIN', {
      'GET /admin/users/': list([self, kasun], {
        page: 1,
        pageSize: 20,
        totalItems: 25,
        totalPages: 2,
      }),
    });
    await page.goto('/admin/users');

    await expect(page.getByText('Page 1 of 2 · 25 results')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Previous' })).toBeDisabled();

    await page.route(listUrl, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(
          list([ruwan], {
            page: 2,
            pageSize: 20,
            totalItems: 25,
            totalPages: 2,
          }).body,
        ),
      });
    });
    const request = page.waitForRequest(
      (r) => new URL(r.url()).searchParams.get('page') === '2',
    );
    await page.getByRole('button', { name: 'Next', exact: true }).click();
    await request;

    await expect(page.getByText('Page 2 of 2 · 25 results')).toBeVisible();
    await expect(person(page, 'Ruwan Jayasuriya')).toBeVisible();
    await expect(
      page.getByRole('button', { name: 'Next', exact: true }),
    ).toBeDisabled();
  });

  test('filters by role and by account status', async ({ page }) => {
    await signInAs(page, 'ADMIN', {
      'GET /admin/users/': list([self, kasun, ruwan]),
    });
    await page.goto('/admin/users');
    await expect(users(page).getByRole('row')).toHaveCount(4);

    const byRole = page.waitForRequest(
      (r) => new URL(r.url()).searchParams.get('role') === 'COURIER',
    );
    await page.locator('#user-role').selectOption('COURIER');
    expect(new URL((await byRole).url()).searchParams.get('page')).toBe('1');

    const byStatus = page.waitForRequest((r) => {
      const params = new URL(r.url()).searchParams;
      return (
        params.get('isActive') === 'false' && params.get('role') === 'COURIER'
      );
    });
    await page.locator('#user-status').selectOption('DEACTIVATED');
    await byStatus;

    const cleared = page.waitForRequest((r) => {
      const params = new URL(r.url()).searchParams;
      return !params.has('isActive') && !params.has('role');
    });
    await page.locator('#user-role').selectOption('');
    await page.locator('#user-status').selectOption('');
    await cleared;
  });

  test('shows a loading state, then the list', async ({ page }) => {
    await signInAs(page, 'ADMIN', {
      'GET /admin/users/': { ...list([self]), delayMs: 400 },
    });
    await page.goto('/admin/users');

    await expect(page.getByText('Loading users')).toBeVisible();
    await expect(users(page)).toBeVisible();
    await expect(page.getByText('Loading users')).toHaveCount(0);
  });

  test('says so when there are no users', async ({ page }) => {
    await signInAs(page, 'ADMIN', { 'GET /admin/users/': list([]) });
    await page.goto('/admin/users');

    await expect(
      page.getByRole('heading', { name: 'No users yet' }),
    ).toBeVisible();
  });

  test('deactivating asks first, then confirms what happened', async ({
    page,
  }) => {
    const { calls } = await signInAs(page, 'ADMIN', {
      'GET /admin/users/': list([self, kasun]),
      'PATCH /admin/users/kasun/status': ok({ id: 'kasun', isActive: false }),
    });
    await page.goto('/admin/users');

    const dialog = page.getByRole('dialog', {
      name: 'Deactivate Kasun Silva?',
    });
    await page.getByRole('button', { name: 'Deactivate Kasun Silva' }).click();
    await expect(dialog).toBeVisible();
    expect(calls.some((c) => c.key.startsWith('PATCH'))).toBe(false);

    // Backing out changes nothing.
    await dialog.getByRole('button', { name: 'Keep active' }).click();
    await expect(dialog).toBeHidden();
    expect(calls.some((c) => c.key.startsWith('PATCH'))).toBe(false);

    await page.getByRole('button', { name: 'Deactivate Kasun Silva' }).click();
    await dialog
      .getByRole('button', { name: 'Deactivate', exact: true })
      .click();

    await expect(
      page
        .getByRole('region', { name: 'Notifications' })
        .getByText(
          'Kasun Silva is deactivated and signed out of every device.',
        ),
    ).toBeVisible();
    await expect(person(page, 'Kasun Silva')).toContainText('DEACTIVATED');
    await expect(
      page.getByRole('button', { name: 'Activate Kasun Silva' }),
    ).toBeVisible();
    expect(calls.find((c) => c.key.startsWith('PATCH'))).toMatchObject({
      key: 'PATCH /admin/users/kasun/status',
      body: { isActive: false },
    });
  });

  test('activating needs no confirmation', async ({ page }) => {
    const { calls } = await signInAs(page, 'ADMIN', {
      'GET /admin/users/': list([self, ruwan]),
      'PATCH /admin/users/ruwan/status': ok({ id: 'ruwan', isActive: true }),
    });
    await page.goto('/admin/users');

    await page
      .getByRole('button', { name: 'Activate Ruwan Jayasuriya' })
      .click();

    await expect(
      page
        .getByRole('region', { name: 'Notifications' })
        .getByText('Ruwan Jayasuriya can sign in again.'),
    ).toBeVisible();
    await expect(person(page, 'Ruwan Jayasuriya')).toContainText('ACTIVE');
    await expect(person(page, 'Ruwan Jayasuriya')).not.toContainText(
      'DEACTIVATED',
    );
    expect(calls.find((c) => c.key.startsWith('PATCH'))?.body).toEqual({
      isActive: true,
    });
  });

  test('an admin cannot deactivate their own account', async ({ page }) => {
    await signInAs(page, 'ADMIN', { 'GET /admin/users/': list([self]) });
    await page.goto('/admin/users');

    await expect(
      page.getByRole('button', { name: 'Deactivate Anusha Nawarathna' }),
    ).toBeDisabled();
    await expect(
      users(page).getByText('You cannot deactivate your own account.'),
    ).toBeVisible();
  });

  test('the server refusing a self-deactivation is explained', async ({
    page,
  }) => {
    // The screen disables the button on the admin's own row, but the server
    // is the authority: if it still answers 409, the screen must explain it.
    await signInAs(page, 'ADMIN', {
      'GET /admin/users/': list([self, kasun]),
      'PATCH /admin/users/kasun/status': fail(409, 'CANNOT_DEACTIVATE_SELF'),
    });
    await page.goto('/admin/users');

    await page.getByRole('button', { name: 'Deactivate Kasun Silva' }).click();
    await page
      .getByRole('dialog', { name: 'Deactivate Kasun Silva?' })
      .getByRole('button', { name: 'Deactivate', exact: true })
      .click();

    await expect(
      page
        .getByRole('alert')
        .or(page.getByRole('status'))
        .getByText('You cannot deactivate your own account', { exact: true }),
    ).toBeVisible();
    await expect(person(page, 'Kasun Silva')).toContainText('ACTIVE');
    await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
  });

  test('a conflict asks for a reload and never shows the server message', async ({
    page,
  }) => {
    await signInAs(page, 'ADMIN', {
      'GET /admin/users/': list([self, kasun]),
      'PATCH /admin/users/kasun/status': fail(409, 'CONFLICT'),
    });
    await page.goto('/admin/users');

    await page.getByRole('button', { name: 'Deactivate Kasun Silva' }).click();
    await page
      .getByRole('dialog', { name: 'Deactivate Kasun Silva?' })
      .getByRole('button', { name: 'Deactivate', exact: true })
      .click();

    await expect(
      page.getByText('This account was changed by someone else'),
    ).toBeVisible();
    await expect(page.getByRole('button', { name: 'Try again' })).toBeVisible();
    await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
  });

  test('a failed list shows a Reference and can be retried', async ({
    page,
  }) => {
    await signInAs(page, 'ADMIN', {
      'GET /admin/users/': fail(500, 'INTERNAL'),
    });
    await page.goto('/admin/users');

    await expect(page.getByText('Something broke on our side')).toBeVisible();
    await expect(page.getByText(`Reference ${REQUEST_ID}`)).toBeVisible();
    await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);

    await page.route(listUrl, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(list([self]).body),
      });
    });
    await page.getByRole('button', { name: 'Try again' }).click();
    await expect(users(page)).toBeVisible();
  });

  test('a 403 from the API shows the no-permission state', async ({ page }) => {
    await signInAs(page, 'ADMIN', {
      'GET /admin/users/': fail(403, 'FORBIDDEN'),
    });
    await page.goto('/admin/users');

    await expect(
      page.getByText('You do not have access to this'),
    ).toBeVisible();
    await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
  });

  test('searching again sends a fresh request even when the words are the same', async ({
    page,
  }) => {
    await signInAs(page, 'ADMIN', {
      'GET /admin/users/': list([self, kasun]),
    });
    await page.goto('/admin/users');
    await expect(person(page, 'Kasun Silva')).toBeVisible();

    await page.getByLabel('Search users').fill('kasun');
    const first = page.waitForRequest(
      (r) => new URL(r.url()).searchParams.get('q') === 'kasun',
    );
    await page.getByRole('button', { name: 'Search' }).click();
    await first;
    await expect(person(page, 'Kasun Silva')).toBeVisible();

    // Same words, same page: still a new request.
    const again = page.waitForRequest(
      (r) => new URL(r.url()).searchParams.get('q') === 'kasun',
    );
    await page.getByRole('button', { name: 'Search' }).click();
    await again;
  });

  test('searching again recovers from a failed list without changing the words', async ({
    page,
  }) => {
    await signInAs(page, 'ADMIN', {
      'GET /admin/users/': fail(500, 'INTERNAL'),
    });
    await page.goto('/admin/users');
    await expect(page.getByText('Something broke on our side')).toBeVisible();

    await page.route(listUrl, (route) => answer(route, list([self, kasun])));
    await page.getByRole('button', { name: 'Search' }).click();

    await expect(person(page, 'Kasun Silva')).toBeVisible();
    await expect(page.getByText('Something broke on our side')).toHaveCount(0);
  });

  test('the search box stops at 100 characters', async ({ page }) => {
    await signInAs(page, 'ADMIN', { 'GET /admin/users/': list([self]) });
    await page.goto('/admin/users');

    const search = page.getByLabel('Search users');
    await expect(search).toHaveAttribute('maxlength', '100');
    await search.fill('a'.repeat(150));
    await expect(search).toHaveValue('a'.repeat(100));
  });

  test('a refused search is explained at the field, in our words', async ({
    page,
  }) => {
    await signInAs(page, 'ADMIN', {
      'GET /admin/users/': fail(422, 'VALIDATION_ERROR', {
        details: [{ field: 'q', issue: SERVER_WORDS }],
      }),
    });
    await page.goto('/admin/users');

    await expect(page.getByText('That search was not accepted')).toBeVisible();
    await expect(
      page.getByText('Use 100 characters or fewer to search.'),
    ).toBeVisible();
    await expect(page.getByLabel('Search users')).toHaveAttribute(
      'aria-invalid',
      'true',
    );
    await expect(
      page.getByText('Use 100 characters or fewer.', { exact: true }),
    ).toBeVisible();
    await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
  });

  test('a refused filter gets a general message and no field error', async ({
    page,
  }) => {
    await signInAs(page, 'ADMIN', {
      'GET /admin/users/': fail(422, 'VALIDATION_ERROR', {
        details: [{ field: 'role', issue: SERVER_WORDS }],
      }),
    });
    await page.goto('/admin/users');

    await expect(page.getByText('That search was not accepted')).toBeVisible();
    await expect(
      page.getByText('Check the search and filters, then try again.'),
    ).toBeVisible();
    await expect(page.getByLabel('Search users')).not.toHaveAttribute(
      'aria-invalid',
      'true',
    );
    await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
  });

  test('the pagination buttons stay, and keep focus, while the next page loads', async ({
    page,
  }) => {
    const pagination = (n: number) => ({
      page: n,
      pageSize: 20,
      totalItems: 45,
      totalPages: 3,
    });
    await signInAs(page, 'ADMIN', {});
    await page.route(listUrl, async (route) => {
      if (requestedPage(route) === '2') {
        await new Promise((resolve) => setTimeout(resolve, 800));
        return answer(route, list([ruwan], pagination(2)));
      }
      return answer(route, list([self, kasun], pagination(1)));
    });
    await page.goto('/admin/users');
    await expect(page.getByText('Page 1 of 3 · 45 results')).toBeVisible();

    const next = page.getByRole('button', { name: 'Next', exact: true });
    await next.focus();
    await next.press('Enter');

    // The rows are replaced by the loading state, the buttons are not.
    await expect(page.getByText('Loading users')).toBeVisible();
    await expect(next).toBeVisible();
    await expect(next).toBeEnabled();
    await expect(next).toBeFocused();

    await expect(page.getByText('Page 2 of 3 · 45 results')).toBeVisible();
    await expect(person(page, 'Ruwan Jayasuriya')).toBeVisible();
    await expect(next).toBeFocused();
  });

  test('a page past the end moves back to the last page there is', async ({
    page,
  }) => {
    const asked: string[] = [];
    await signInAs(page, 'ADMIN', {});
    await page.route(listUrl, (route) => {
      const n = requestedPage(route) ?? '1';
      asked.push(n);
      // The list shrank to two pages while the person was on page 3.
      if (n === '3') {
        return answer(
          route,
          list([], { page: 3, pageSize: 20, totalItems: 25, totalPages: 2 }),
        );
      }
      if (n === '2' && asked.includes('3')) {
        return answer(
          route,
          list([ruwan], {
            page: 2,
            pageSize: 20,
            totalItems: 25,
            totalPages: 2,
          }),
        );
      }
      return answer(
        route,
        list([n === '1' ? kasun : ruwan], {
          page: Number(n),
          pageSize: 20,
          totalItems: 45,
          totalPages: 3,
        }),
      );
    });
    await page.goto('/admin/users');
    await expect(page.getByText('Page 1 of 3 · 45 results')).toBeVisible();

    const next = page.getByRole('button', { name: 'Next', exact: true });
    await next.click();
    await expect(page.getByText('Page 2 of 3 · 45 results')).toBeVisible();
    await next.click();

    await expect(page.getByText(/^Page 2 of 2/)).toBeVisible();
    await expect(person(page, 'Ruwan Jayasuriya')).toBeVisible();
    await expect.poll(() => inOrder(asked)).toEqual(['1', '2', '3', '2']);
  });

  test('no results at all on a later page moves back to page 1', async ({
    page,
  }) => {
    const asked: string[] = [];
    await signInAs(page, 'ADMIN', {});
    await page.route(listUrl, (route) => {
      const n = requestedPage(route) ?? '1';
      asked.push(n);
      if (n === '2') {
        return answer(
          route,
          list([], { page: 2, pageSize: 20, totalItems: 0, totalPages: 0 }),
        );
      }
      return answer(
        route,
        list([kasun], { page: 1, pageSize: 20, totalItems: 25, totalPages: 2 }),
      );
    });
    await page.goto('/admin/users');
    await expect(page.getByText('Page 1 of 2 · 25 results')).toBeVisible();

    await page.getByRole('button', { name: 'Next', exact: true }).click();

    await expect(person(page, 'Kasun Silva')).toBeVisible();
    // Page 1 is still on screen from before, so wait for the request itself.
    await expect.poll(() => inOrder(asked)).toEqual(['1', '2', '1']);
  });

  test('anyone who is not an admin sees the 403 page, and no request is made', async ({
    page,
  }) => {
    const { calls } = await signInAs(page, 'COURIER');
    await page.goto('/admin/users');

    await expect(
      page.getByRole('heading', { name: 'You do not have access' }),
    ).toBeVisible();
    expect(calls.some((c) => c.key === 'GET /admin/users/')).toBe(false);
  });
});
