import { expect, test, type Page } from '@playwright/test';

import { SERVER_WORDS, fail, ok, type Reply } from './support/mock-api';
import { signInAs, names } from './support/sign-in';

test.use({ colorScheme: 'light' });

function list(items: unknown[], totalItems = items.length): Reply {
  return {
    status: 200,
    body: {
      data: items,
      meta: {
        requestId: 'req-1',
        pagination: {
          page: 1,
          pageSize: 20,
          totalItems,
          totalPages: Math.max(1, Math.ceil(totalItems / 20)),
        },
      },
    },
  };
}

const parcel = (n: number, status: string, city: string) => ({
  id: `3f1c7a52-0000-4000-8000-00000000010${n}`,
  trackingNumber: `SDK7M2Q9XP${n}`,
  status,
  recipientName: 'Kasun',
  dropoffCity: city,
  total: 550,
  currency: 'LKR',
  createdAt: '2026-11-04T08:00:00Z',
});

const merchant = {
  id: '3f1c7a52-0000-4000-8000-0000000000a1',
  ownerUserId: '3f1c7a52-0000-4000-8000-0000000000b1',
  businessName: 'ShopZone Lanka',
  registrationNumber: 'PV 123456',
  contactEmail: 'support@shopzone.lk',
  contactPhone: '+94112223344',
  pickupAddress: '88 Galle Road, Colombo 03',
  pickupZoneId: '7a1e0000-0000-4000-8000-000000000001',
  status: 'APPROVED',
};
const key = (n: number, status: 'ACTIVE' | 'REVOKED') => ({
  id: `key-${n}`,
  name: `Key ${n}`,
  keyPrefix: `sd_live_${n}`,
  status,
  createdAt: '2026-11-04T08:00:00Z',
});

const heading = (page: Page) =>
  page.getByRole('heading', { level: 1, name: 'Dashboard', exact: true });

test.describe('customer dashboard', () => {
  test('shows who is signed in, the next actions and the latest parcels', async ({
    page,
  }) => {
    await signInAs(page, 'CUSTOMER', {
      'GET /parcels': list(
        [parcel(1, 'IN_TRANSIT', 'Kandy'), parcel(2, 'DELIVERED', 'Galle')],
        12,
      ),
    });
    await page.goto('/dashboard');

    await expect(heading(page)).toBeVisible();
    await expect(page.getByText('Signed in as')).toContainText(names.CUSTOMER);
    const main = page.getByRole('main');
    await expect(
      main.getByRole('link', { name: 'New delivery' }),
    ).toBeVisible();
    await expect(
      main.getByRole('link', { name: 'Track a parcel' }),
    ).toBeVisible();
    await expect(page.getByText('Your latest 5 of 12 parcels.')).toBeVisible();
    const table = page.getByRole('table', { name: 'Your recent parcels' });
    await expect(
      table.getByRole('link', { name: 'SDK7M2Q9XP1' }),
    ).toHaveAttribute('href', '/parcels/3f1c7a52-0000-4000-8000-000000000101');
    await expect(table.getByText('IN TRANSIT')).toBeVisible();
    await expect(table.getByText('Galle')).toBeVisible();
  });

  test('asks for the latest five, newest first', async ({ page }) => {
    const { calls } = await signInAs(page, 'CUSTOMER', {
      'GET /parcels': list([]),
    });
    await page.goto('/dashboard');
    await expect(heading(page)).toBeVisible();

    const request = calls.find((call) => call.key === 'GET /parcels');
    expect(new URL(request?.url ?? '').searchParams.get('pageSize')).toBe('5');
    expect(new URL(request?.url ?? '').searchParams.get('sort')).toBe(
      'createdAt:desc',
    );
  });

  test('a customer with no parcels is invited to create one', async ({
    page,
  }) => {
    await signInAs(page, 'CUSTOMER', { 'GET /parcels': list([]) });
    await page.goto('/dashboard');

    await expect(page.getByText('No parcels yet')).toBeVisible();
    await expect(
      page.getByRole('link', { name: 'Create a delivery' }),
    ).toHaveAttribute('href', '/new');
  });

  test('a failed load says so in plain words and can be retried', async ({
    page,
  }) => {
    let calls = 0;
    await signInAs(page, 'CUSTOMER', {
      'GET /parcels': () => {
        calls += 1;
        return calls === 1
          ? fail(500, 'INTERNAL_ERROR')
          : list([parcel(1, 'CREATED', 'Kandy')]);
      },
    });
    await page.goto('/dashboard');

    await expect(page.getByText('Could not load your parcels')).toBeVisible();
    await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
    await expect(page.getByText('Reference')).toBeVisible();
    await page.getByRole('button', { name: 'Try again' }).click();

    await expect(page.getByRole('link', { name: 'SDK7M2Q9XP1' })).toBeVisible();
  });
});

test.describe('merchant dashboard', () => {
  test('shows the business and how many API keys are active', async ({
    page,
  }) => {
    await signInAs(page, 'MERCHANT', {
      'GET /merchants/me': ok(merchant),
      'GET /merchants/me/keys': ok([
        key(1, 'ACTIVE'),
        key(2, 'REVOKED'),
        key(3, 'ACTIVE'),
      ]),
    });
    await page.goto('/dashboard');

    await expect(heading(page)).toBeVisible();
    await expect(page.getByText('Signed in as')).toContainText(names.MERCHANT);
    await expect(page.getByText('ShopZone Lanka')).toBeVisible();
    await expect(page.getByText('PV 123456')).toBeVisible();
    await expect(page.getByText('APPROVED')).toBeVisible();
    await expect(page.getByText('active keys')).toBeVisible();
    await expect(
      page.getByRole('link', { name: 'Manage API keys' }),
    ).toHaveAttribute('href', '/api-keys');
  });

  test('a merchant with no active key is invited to create one', async ({
    page,
  }) => {
    await signInAs(page, 'MERCHANT', {
      'GET /merchants/me': ok(merchant),
      'GET /merchants/me/keys': ok([key(1, 'REVOKED')]),
    });
    await page.goto('/dashboard');

    await expect(page.getByText('No API keys yet')).toBeVisible();
    await expect(
      page.getByRole('link', { name: 'Create an API key' }),
    ).toHaveAttribute('href', '/api-keys');
  });

  test('the keys failing does not hide the business', async ({ page }) => {
    await signInAs(page, 'MERCHANT', {
      'GET /merchants/me': ok(merchant),
      'GET /merchants/me/keys': fail(503, 'SERVICE_UNAVAILABLE'),
    });
    await page.goto('/dashboard');

    await expect(page.getByText('Could not load your API keys')).toBeVisible();
    await expect(page.getByText('ShopZone Lanka')).toBeVisible();
    await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
  });
});

test.describe('admin dashboard', () => {
  const handlers = {
    'GET /admin/merchants': list([], 3),
    'GET /dispatch/unassigned': list([], 7),
    'GET /admin/users/': list([], 25),
  };

  test('shows what is waiting, each with the screen that acts on it', async ({
    page,
  }) => {
    await signInAs(page, 'ADMIN', handlers);
    await page.goto('/dashboard');

    await expect(heading(page)).toBeVisible();
    await expect(page.getByText('Signed in as')).toContainText(names.ADMIN);
    const card = (label: string) =>
      page.locator('div').filter({ hasText: label }).last();
    await expect(card('Merchant applications')).toContainText('3');
    await expect(card('Unassigned parcels')).toContainText('7');
    await expect(card('Accounts')).toContainText('25');
    await expect(
      page.getByRole('link', { name: 'Review applications' }),
    ).toHaveAttribute('href', '/admin/merchants');
    await expect(
      page.getByRole('link', { name: 'Assign couriers' }),
    ).toHaveAttribute('href', '/unassigned');
    await expect(
      page.getByRole('link', { name: 'Manage accounts' }),
    ).toHaveAttribute('href', '/admin/users');
  });

  test('asks each list for one row and reads the total', async ({ page }) => {
    const { calls } = await signInAs(page, 'ADMIN', handlers);
    await page.goto('/dashboard');
    await expect(heading(page)).toBeVisible();
    await expect(page.getByText('25')).toBeVisible();

    const applications = calls.find((c) => c.key === 'GET /admin/merchants');
    const params = new URL(applications?.url ?? '').searchParams;
    expect(params.get('status')).toBe('PENDING');
    expect(params.get('pageSize')).toBe('1');
  });

  test('zero says what it means', async ({ page }) => {
    await signInAs(page, 'ADMIN', {
      ...handlers,
      'GET /admin/merchants': list([], 0),
    });
    await page.goto('/dashboard');

    await expect(page.getByText('No applications are waiting.')).toBeVisible();
  });

  test('one card failing keeps the others, and retrying fills it in', async ({
    page,
  }) => {
    let calls = 0;
    await signInAs(page, 'ADMIN', {
      ...handlers,
      'GET /dispatch/unassigned': () => {
        calls += 1;
        return calls === 1 ? fail(500, 'INTERNAL_ERROR') : list([], 7);
      },
    });
    await page.goto('/dashboard');

    await expect(
      page.getByText('This could not be shown right now.'),
    ).toBeVisible();
    await expect(page.getByText('25')).toBeVisible();
    await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
    await page.getByRole('button', { name: 'Try again' }).click();

    await expect(page.getByText('7', { exact: true })).toBeVisible();
    await expect(
      page.getByText('This could not be shown right now.'),
    ).toHaveCount(0);
  });

  test('links to the places an administrator manages', async ({ page }) => {
    await signInAs(page, 'ADMIN', handlers);
    await page.goto('/dashboard');

    for (const [name, href] of [
      ['Users', '/admin/users'],
      ['Merchants', '/admin/merchants'],
      ['Zones', '/admin/zones'],
      ['Pricing', '/admin/pricing'],
    ]) {
      await expect(
        page.getByRole('main').getByRole('link', { name, exact: true }),
      ).toHaveAttribute('href', href);
    }
  });
});

test.describe('other roles', () => {
  test('a dispatcher who opens the dashboard is sent to the unassigned list', async ({
    page,
  }) => {
    await signInAs(page, 'DISPATCHER', {
      'GET /dispatch/unassigned': list([]),
      'GET /zones': ok([]),
    });
    await page.goto('/dashboard');

    await expect(page).toHaveURL(/\/unassigned$/);
  });
});

test.describe('on a phone and in the dark', () => {
  for (const role of ['CUSTOMER', 'MERCHANT', 'ADMIN'] as const) {
    test(`${role.toLowerCase()} dashboard fits 375px in dark mode`, async ({
      page,
    }) => {
      await page.emulateMedia({ colorScheme: 'dark' });
      await page.setViewportSize({ width: 375, height: 812 });
      await signInAs(page, role, {
        'GET /parcels': list([parcel(1, 'CREATED', 'Kandy')]),
        'GET /merchants/me': ok(merchant),
        'GET /merchants/me/keys': ok([key(1, 'ACTIVE')]),
        'GET /admin/merchants': list([], 3),
        'GET /dispatch/unassigned': list([], 7),
        'GET /admin/users/': list([], 25),
      });
      await page.goto('/dashboard');

      await expect(heading(page)).toBeVisible();
      const overflow = await page.evaluate<number>(
        'document.documentElement.scrollWidth - window.innerWidth',
      );
      expect(overflow).toBeLessThanOrEqual(0);
    });
  }
});
