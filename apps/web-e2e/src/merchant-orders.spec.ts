import { expect, test } from '@playwright/test';

import { fail, ok, type Reply } from './support/mock-api';
import { signInAs } from './support/sign-in';

test.use({ colorScheme: 'light' });

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

const order = (n: number, status: string) => ({
  orderId: `order-${n}`,
  externalOrderId: `SZ-10024${n}`,
  trackingNumber: `SDN4T8W2LC${n}`,
  status,
  recipientName: 'Amali Fernando',
  dropoffCity: 'Kandy',
  total: 550,
  currency: 'LKR',
  createdAt: '2026-11-04T08:00:00Z',
  updatedAt: '2026-11-04T13:00:00Z',
});

const detail = {
  ...order(1, 'IN_TRANSIT'),
  weightKg: 1.5,
  sizeCategory: 'SMALL',
  description: 'Phone case',
  fee: {
    currency: 'LKR',
    baseFee: 450,
    weightCharge: 100,
    sizeSurcharge: 0,
    total: 550,
  },
  timeline: [
    { status: 'CREATED', at: '2026-11-04T08:00:00Z' },
    { status: 'PICKED_UP', at: '2026-11-04T09:00:00Z' },
    { status: 'IN_TRANSIT', at: '2026-11-04T12:00:00Z' },
  ],
};

function paged(items: unknown[], totalItems: number, page = 1): Reply {
  return {
    status: 200,
    body: {
      data: items,
      meta: {
        requestId: 'req-orders',
        pagination: {
          page,
          pageSize: 20,
          totalItems,
          totalPages: Math.max(1, Math.ceil(totalItems / 20)),
        },
      },
    },
  };
}

test.describe('merchant orders', () => {
  test('lists real orders, paginates and links to detail', async ({ page }) => {
    const { calls } = await signInAs(page, 'MERCHANT', {
      'GET /merchants/me': ok(merchant),
      'GET /merchants/me/keys': ok([]),
      'GET /merchant/orders': ({ query }) =>
        paged([order(1, 'IN_TRANSIT')], 45, Number(query.get('page') || 1)),
    });
    await page.goto('/merchant/orders');

    await expect(
      page.getByRole('heading', { level: 1, name: 'Orders', exact: true }),
    ).toBeVisible();
    await expect(page.getByText('45 orders in this account.')).toBeVisible();
    await expect(page.getByText('SZ-100241')).toBeVisible();
    await expect(page.getByText('SDN4T8W2LC1')).toBeVisible();
    await expect(page.getByRole('link', { name: 'SZ-100241' })).toHaveAttribute(
      'href',
      '/merchant/orders/order-1',
    );
    await expect(page.getByText('Page 1 of 3')).toBeVisible();

    await page.getByRole('button', { name: 'Next' }).click();
    await expect(page.getByText('Page 2 of 3')).toBeVisible();
    expect(
      calls.some(
        (call) =>
          call.key === 'GET /merchant/orders' &&
          new URL(call.url).searchParams.get('page') === '2',
      ),
    ).toBe(true);
  });

  test('applies status and date filters and validates the date order', async ({
    page,
  }) => {
    const { calls } = await signInAs(page, 'MERCHANT', {
      'GET /merchants/me': ok(merchant),
      'GET /merchants/me/keys': ok([]),
      'GET /merchant/orders': paged([], 0),
    });
    await page.goto('/merchant/orders');

    await page.getByLabel('Status').selectOption('IN_TRANSIT');
    await page.getByLabel('External order ID').fill('SZ-100241');
    await page.getByLabel('From date').fill('2026-11-01');
    await page.getByLabel('To date').fill('2026-11-05');
    await page.getByRole('button', { name: 'Apply filters' }).click();

    await expect
      .poll(() =>
        calls.some((call) => {
          if (call.key !== 'GET /merchant/orders') return false;
          const query = new URL(call.url).searchParams;
          return (
            query.get('status') === 'IN_TRANSIT' &&
            query.get('externalOrderId') === 'SZ-100241' &&
            query.get('from') === '2026-11-01' &&
            query.get('to') === '2026-11-05'
          );
        }),
      )
      .toBe(true);
    await expect(page.getByText('No orders found')).toBeVisible();

    await page.getByLabel('From date').fill('2026-11-06');
    await page.getByLabel('To date').fill('2026-11-05');
    await page.getByRole('button', { name: 'Apply filters' }).click();
    await expect(
      page.getByText('The end date must be on or after the start date.'),
    ).toBeVisible();
  });

  test('shows the Create via API panel without exposing a key', async ({
    page,
  }) => {
    await signInAs(page, 'MERCHANT', {
      'GET /merchants/me': ok(merchant),
      'GET /merchants/me/keys': ok([]),
      'GET /merchant/orders': paged([], 0),
    });
    await page.goto('/merchant/orders');

    await expect(
      page.getByRole('heading', { name: 'Create orders from your system' }),
    ).toBeVisible();
    await expect(page.getByText('POST', { exact: true })).toBeVisible();
    await expect(page.getByText('/api/v1/merchant/orders')).toBeVisible();
    await expect(page.getByText('X-API-Key: sdk_live_...')).toBeVisible();
    await expect(
      page.getByText('Idempotency-Key: up to 100 characters'),
    ).toBeVisible();
    await expect(page.getByText('201 Created')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Copy cURL' })).toBeVisible();
    await expect(
      page.getByRole('link', { name: 'Manage API keys' }),
    ).toHaveAttribute('href', '/api-keys');
    await expect(
      page.getByRole('link', { name: /API documentation/ }),
    ).toHaveAttribute(
      'href',
      'https://github.com/Vineth-R/swiftdrop/blob/development/docs/API_Documentation_v1.1.md#816-post-merchantorders',
    );
  });

  test('shows the synchronized status timeline in detail', async ({ page }) => {
    await signInAs(page, 'MERCHANT', {
      'GET /merchants/me': ok(merchant),
      'GET /merchants/me/keys': ok([]),
      'GET /merchant/orders/order-1': ok(detail),
    });
    await page.goto('/merchant/orders/order-1');

    await expect(
      page.getByRole('heading', { level: 1, name: 'Order details' }),
    ).toBeVisible();
    await expect(page.getByText('SZ-100241')).toBeVisible();
    await expect(page.getByText('SDN4T8W2LC1')).toBeVisible();
    await expect(page.getByText('Phone case')).toBeVisible();
    await expect(page.getByText(/550\.00/).first()).toBeVisible();
    await expect(page.getByText('CREATED').first()).toBeVisible();
    await expect(page.getByText('PICKED UP')).toBeVisible();
    await expect(page.getByText('IN TRANSIT').first()).toBeVisible();
  });

  test('a pending merchant sees the approval notice and no orders request', async ({
    page,
  }) => {
    const pending = { ...merchant, status: 'PENDING' };
    const { calls } = await signInAs(page, 'MERCHANT', {
      'GET /merchants/me': ok(pending),
      'GET /merchants/me/keys': fail(403, 'MERCHANT_NOT_APPROVED'),
    });
    await page.goto('/merchant/orders');

    await expect(
      page.getByText('Merchant account pending approval'),
    ).toBeVisible();
    expect(calls.some((call) => call.key === 'GET /merchant/orders')).toBe(
      false,
    );
    await expect(
      page.getByRole('heading', { name: 'Create orders from your system' }),
    ).toHaveCount(0);
  });

  test('a suspended merchant cannot open the orders screen', async ({
    page,
  }) => {
    const suspended = { ...merchant, status: 'SUSPENDED' };
    const { calls } = await signInAs(page, 'MERCHANT', {
      'GET /merchants/me': ok(suspended),
      'GET /merchants/me/keys': fail(403, 'MERCHANT_NOT_APPROVED'),
    });
    await page.goto('/merchant/orders');

    await expect(page.getByText('Merchant API access suspended')).toBeVisible();
    expect(calls.some((call) => call.key === 'GET /merchant/orders')).toBe(
      false,
    );
  });
});
