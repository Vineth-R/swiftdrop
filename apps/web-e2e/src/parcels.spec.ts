import { expect, test, type Page } from '@playwright/test';
import {
  fail,
  ok,
  SERVER_WORDS,
  signedOut,
  mockApi,
  type Reply,
} from './support/mock-api';
import { signInAs } from './support/sign-in';

const id = 'e5a1b7c2-6d3f-4a8b-9c10-2d3e4f5a6b7c';
const origin = '7a1e0c10-2b3d-4c5e-8f90-a1b2c3d4e5f6';
const destination = '9b2f1d21-3c4e-4d6f-9a01-b2c3d4e5f607';
const fee = {
  currency: 'LKR',
  baseFee: 450,
  weightCharge: 100,
  sizeSurcharge: 0,
  total: 550,
};
const parcel = {
  id,
  trackingNumber: 'SDK7M2Q9XP4',
  status: 'CREATED',
  sender: {
    name: 'Nimal Perera',
    phone: '+94771234567',
    address: '12 Temple Road',
    city: 'Colombo',
    zoneId: origin,
  },
  recipient: {
    name: 'Amali Fernando',
    phone: '+94712345678',
    address: '45 Lake Drive',
    city: 'Kandy',
    zoneId: destination,
  },
  weightKg: 2,
  sizeCategory: 'SMALL',
  description: 'Books',
  fee,
  courier: null,
  failureReason: null,
  createdAt: '2026-11-02T10:00:00Z',
  updatedAt: '2026-11-02T10:00:00Z',
  deliveredAt: null,
};
const summary = {
  id,
  trackingNumber: parcel.trackingNumber,
  status: parcel.status,
  recipientName: parcel.recipient.name,
  dropoffCity: 'Kandy',
  total: 550,
  currency: 'LKR',
  createdAt: parcel.createdAt,
};
const zones = [
  { id: origin, code: 'CMB', name: 'Colombo', districts: [] },
  { id: destination, code: 'KDY', name: 'Kandy', districts: [] },
];
const wizard = {
  'GET /zones': ok(zones),
  'POST /pricing/estimate': ok(fee),
  'POST /parcels': ok(parcel, 201),
};

async function routeStep(page: Page) {
  await page.getByLabel('Pickup zone', { exact: false }).selectOption(origin);
  await page
    .getByLabel('Destination zone', { exact: false })
    .selectOption(destination);
  await page.getByLabel('Weight', { exact: false }).fill('2');
  await expect(page.getByText('LKR 550.00')).toBeVisible();
  await page.getByRole('button', { name: 'Continue', exact: true }).click();
}
async function contactStep(page: Page) {
  for (const [label, value] of Object.entries({
    'Sender name': 'Nimal Perera',
    'Sender phone': '+94771234567',
    'Sender address': '12 Temple Road',
    'Sender city': 'Colombo',
    'Recipient name': 'Amali Fernando',
    'Recipient phone': '+94712345678',
    'Recipient address': '45 Lake Drive',
    'Recipient city': 'Kandy',
  }))
    await page.getByLabel(label, { exact: false }).fill(value);
  await page
    .getByLabel('Description (optional)', { exact: false })
    .fill('Books');
  await page.getByRole('button', { name: 'Continue', exact: true }).click();
}
function paged(items: unknown[], page = 1, totalPages = 1): Reply {
  return {
    status: 200,
    body: {
      data: items,
      meta: {
        pagination: {
          page,
          pageSize: 20,
          totalItems: totalPages === 1 ? items.length : 21,
          totalPages,
        },
      },
    },
  };
}

function lastParcelQuery(calls: { key: string; url: string }[]) {
  const last = calls.filter((call) => call.key === 'GET /parcels').at(-1);
  if (!last) throw new Error('Expected a parcel list request');
  return new URL(last.url).searchParams;
}

test.use({ channel: process.env['PLAYWRIGHT_CHANNEL'] });

for (const width of [360, 1280]) {
  test(`customer completes delivery wizard at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 });
    const { calls } = await signInAs(page, 'CUSTOMER', wizard);
    await page.goto('/new');
    await routeStep(page);
    await contactStep(page);
    await expect(
      page.getByText('Review delivery', { exact: false }).first(),
    ).toBeVisible();
    await expect(page.getByText('12 Temple Road, Colombo')).toBeVisible();
    await page
      .getByRole('button', { name: 'Create delivery', exact: true })
      .click();
    await expect(
      page.getByRole('heading', { name: 'Delivery created' }),
    ).toBeVisible();
    await expect(page.getByText('SDK7M2Q9XP4', { exact: true })).toBeVisible();
    expect(calls.filter((call) => call.key === 'POST /parcels')).toHaveLength(
      1,
    );
    expect(
      calls.find((call) => call.key === 'POST /parcels')?.body,
    ).toMatchObject({
      expectedFee: 550,
      sender: { zoneId: origin },
      recipient: { zoneId: destination },
    });
    expect(
      await page.evaluate<boolean>(
        'document.documentElement.scrollWidth <= window.innerWidth',
      ),
    ).toBe(true);
  });
}

test('wizard validates contacts and keeps input after a server failure', async ({
  page,
}) => {
  await signInAs(page, 'CUSTOMER', {
    ...wizard,
    'POST /parcels': fail(500, 'INTERNAL_ERROR'),
  });
  await page.goto('/new');
  await routeStep(page);
  await expect(
    page.getByLabel('Sender name', { exact: false }),
  ).not.toHaveAttribute('aria-invalid', 'true');
  await page.getByRole('button', { name: 'Continue', exact: true }).click();
  await expect(page.getByLabel('Sender name', { exact: false })).toBeFocused();
  await contactStep(page);
  await page
    .getByRole('button', { name: 'Create delivery', exact: true })
    .click();
  await expect(page.getByText('Something broke on our side')).toBeVisible();
  await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
  await expect(
    page.getByRole('button', { name: 'Back', exact: true }),
  ).toBeDisabled();
  await expect(
    page.getByText('Your delivery may already be saved'),
  ).toBeVisible();
  await expect(page.getByText('Amali Fernando · +94712345678')).toBeVisible();
});

test('retry preserves the idempotency key and blocks duplicate submissions', async ({
  page,
}) => {
  await signInAs(page, 'CUSTOMER', wizard);
  const keys: string[] = [];
  let finish!: () => void;
  const finished = new Promise<void>((resolve) => {
    finish = resolve;
  });
  await page.route('**/api/v1/parcels', async (route) => {
    if (route.request().method() !== 'POST') return route.fallback();
    keys.push(route.request().headers()['idempotency-key']);
    if (keys.length === 1) return route.abort('failed');
    await finished;
    return route.fulfill({
      status: 201,
      contentType: 'application/json',
      body: JSON.stringify(ok(parcel).body),
    });
  });
  await page.goto('/new');
  await routeStep(page);
  await contactStep(page);
  await page
    .getByRole('button', { name: 'Create delivery', exact: true })
    .click();
  await expect(page.getByText('We could not reach SwiftDrop')).toBeVisible();
  await page
    .getByRole('button', { name: 'Create delivery', exact: true })
    .click();
  await expect(
    page.getByRole('button', { name: 'Creating delivery', exact: true }),
  ).toBeDisabled();
  finish();
  await expect(
    page.getByRole('heading', { name: 'Delivery created' }),
  ).toBeVisible();
  expect(keys).toHaveLength(2);
  expect(keys[0]).toMatch(/^[0-9a-f-]{36}$/);
  expect(keys[1]).toBe(keys[0]);
});

test('price changes require explicit review and a second creation action', async ({
  page,
}) => {
  let attempts = 0;
  let currentFee = fee;
  const { calls } = await signInAs(page, 'CUSTOMER', {
    ...wizard,
    'POST /pricing/estimate': () => ok(currentFee),
    'POST /parcels': () => {
      attempts++;
      if (attempts === 1) {
        currentFee = { ...fee, baseFee: 500, total: 600 };
        return {
          status: 409,
          body: {
            error: {
              code: 'PRICE_CHANGED',
              currentFee: 600,
              message: SERVER_WORDS,
            },
          },
        };
      }
      return ok({ ...parcel, fee: currentFee }, 201);
    },
  });
  await page.goto('/new');
  await routeStep(page);
  await contactStep(page);
  await page
    .getByRole('button', { name: 'Create delivery', exact: true })
    .click();
  const dialog = page.getByRole('dialog');
  await expect(
    dialog.getByRole('heading', { name: 'The delivery fee changed' }),
  ).toBeVisible();
  await dialog.getByRole('button', { name: 'Review new fee' }).click();
  await expect(page.getByText('LKR 600.00')).toBeVisible();
  expect(attempts).toBe(1);
  await page
    .getByRole('button', { name: 'Create delivery', exact: true })
    .click();
  await expect(
    page.getByRole('heading', { name: 'Delivery created' }),
  ).toBeVisible();
  expect(
    calls.filter((call) => call.key === 'POST /parcels').at(-1)?.body,
  ).toMatchObject({ expectedFee: 600 });
});

test('list filters, sorts and paginates with documented query fields', async ({
  page,
}) => {
  const { calls } = await signInAs(page, 'CUSTOMER', {
    'GET /parcels': ({ query }) =>
      paged(
        [
          {
            ...summary,
            trackingNumber:
              query.get('page') === '2'
                ? 'SDKSECOND22'
                : summary.trackingNumber,
          },
        ],
        Number(query.get('page') ?? 1),
        2,
      ),
  });
  await page.goto('/parcels');
  await expect(
    page.getByRole('link', { name: `View parcel ${summary.trackingNumber}` }),
  ).toBeVisible();
  await page.getByLabel('Status', { exact: true }).selectOption('CREATED');
  await page.getByRole('button', { name: 'Go to next page' }).click();
  await expect(
    page.getByRole('link', { name: 'View parcel SDKSECOND22' }),
  ).toBeVisible();
  expect(lastParcelQuery(calls).get('page')).toBe('2');
  expect(lastParcelQuery(calls).get('status')).toBe('CREATED');
  await page.getByLabel('Order', { exact: true }).selectOption('createdAt:asc');
  await expect(
    page.getByRole('link', { name: `View parcel ${summary.trackingNumber}` }),
  ).toBeVisible();
});

test('list provides empty and safe error states', async ({ page }) => {
  let isError = false;
  await signInAs(page, 'CUSTOMER', {
    'GET /parcels': () => (isError ? fail(500, 'INTERNAL_ERROR') : paged([])),
  });
  await page.goto('/parcels');
  await expect(
    page.getByRole('heading', { name: 'No parcels yet' }),
  ).toBeVisible();
  isError = true;
  await page.getByLabel('Status', { exact: true }).selectOption('FAILED');
  await expect(page.getByText('Something broke on our side')).toBeVisible();
  await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
});

test('zone loading and unavailable routes have clear states', async ({
  page,
}) => {
  await signInAs(page, 'CUSTOMER', {
    ...wizard,
    'POST /pricing/estimate': fail(422, 'ROUTE_NOT_SERVICEABLE'),
  });
  let release!: () => void;
  const ready = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route('**/api/v1/zones', async (route) => {
    await ready;
    return route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(ok(zones).body),
    });
  });
  await page.goto('/new');
  await expect(
    page.getByRole('status').filter({ hasText: 'Loading delivery zones' }),
  ).toBeVisible();
  release();
  await page.getByLabel('Pickup zone', { exact: false }).selectOption(origin);
  await page
    .getByLabel('Destination zone', { exact: false })
    .selectOption(destination);
  await page.getByLabel('Weight', { exact: false }).fill('2');
  await expect(
    page.getByText(
      'Delivery is not available between these zones. Choose another route.',
    ),
  ).toBeVisible();
  await expect(
    page.getByRole('button', { name: 'Continue', exact: true }),
  ).toBeDisabled();
});

test('server field validation returns focus to the entered contact', async ({
  page,
}) => {
  await signInAs(page, 'CUSTOMER', {
    ...wizard,
    'POST /parcels': fail(422, 'VALIDATION_ERROR', {
      details: [{ field: 'recipient.phone', issue: 'invalid' }],
    }),
  });
  await page.goto('/new');
  await routeStep(page);
  await contactStep(page);
  await page
    .getByRole('button', { name: 'Create delivery', exact: true })
    .click();
  await expect(
    page.getByLabel('Recipient phone', { exact: false }),
  ).toBeFocused();
  await expect(
    page.getByLabel('Recipient phone', { exact: false }),
  ).toHaveAttribute('aria-invalid', 'true');
  await expect(
    page.getByLabel('Recipient phone', { exact: false }),
  ).toHaveValue('+94712345678');
  await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
});

test('wizard fields work with keyboard focus in dark mode', async ({
  page,
}) => {
  await signInAs(page, 'CUSTOMER', wizard);
  await page.goto('/new');
  await page.evaluate(
    "document.documentElement.setAttribute('data-theme', 'dark')",
  );
  const pickup = page.getByLabel('Pickup zone', { exact: false });
  await pickup.focus();
  await page.keyboard.press('ArrowDown');
  await page.keyboard.press('Tab');
  await expect(
    page.getByLabel('Destination zone', { exact: false }),
  ).toBeFocused();
  await page.getByLabel('Weight', { exact: false }).fill('0');
  await page.getByLabel('Size category', { exact: false }).focus();
  await expect(page.getByLabel('Weight', { exact: false })).toHaveAttribute(
    'aria-invalid',
    'true',
  );
});

test('editing an explicitly rejected request starts a new intended submission', async ({
  page,
}) => {
  await signInAs(page, 'CUSTOMER', wizard);
  const keys: string[] = [];
  await page.route('**/api/v1/parcels', (route) => {
    if (route.request().method() !== 'POST') return route.fallback();
    keys.push(route.request().headers()['idempotency-key']);
    const reply =
      keys.length === 1
        ? fail(422, 'VALIDATION_ERROR', {
            details: [{ field: 'recipient.phone', issue: 'invalid' }],
          })
        : ok(parcel, 201);
    return route.fulfill({
      status: reply.status,
      contentType: 'application/json',
      body: JSON.stringify(reply.body),
    });
  });
  await page.goto('/new');
  await routeStep(page);
  await contactStep(page);
  await page
    .getByRole('button', { name: 'Create delivery', exact: true })
    .click();
  await expect(
    page.getByLabel('Recipient phone', { exact: false }),
  ).toBeFocused();
  await page
    .getByLabel('Recipient phone', { exact: false })
    .fill('+94712223344');
  await page.getByRole('button', { name: 'Continue', exact: true }).click();
  await page
    .getByRole('button', { name: 'Create delivery', exact: true })
    .click();
  await expect(
    page.getByRole('heading', { name: 'Delivery created' }),
  ).toBeVisible();
  expect(keys).toHaveLength(2);
  expect(keys[1]).not.toBe(keys[0]);
});

for (const width of [360, 1280]) {
  test(`detail timeline and PDF receipt at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 });
    await signInAs(page, 'CUSTOMER', {
      [`GET /parcels/${id}`]: ok(parcel),
      [`GET /parcels/${id}/history`]: ok([
        {
          previousStatus: null,
          status: 'CREATED',
          changedAt: parcel.createdAt,
          note: null,
        },
        {
          previousStatus: 'CREATED',
          status: 'PICKED_UP',
          changedAt: '2026-11-03T10:00:00Z',
          note: 'Collected from sender',
        },
      ]),
    });
    await page.route(`**/api/v1/parcels/${id}/receipt`, (route) =>
      route.fulfill({
        status: 200,
        contentType: 'application/pdf',
        body: '%PDF-1.4\nreceipt',
      }),
    );
    await page.goto(`/parcels/${id}`);
    await expect(
      page.getByText(parcel.trackingNumber, { exact: true }),
    ).toBeVisible();
    const timeline = page.getByRole('list', { name: 'Delivery history' });
    await expect(timeline.getByRole('listitem').first()).toContainText(
      'CREATED',
    );
    await expect(timeline.getByRole('listitem').last()).toContainText(
      'PICKED UP',
    );
    const download = page.waitForEvent('download');
    await page.getByRole('button', { name: 'Download PDF receipt' }).click();
    expect((await download).suggestedFilename()).toBe(
      'receipt_SDK7M2Q9XP4.pdf',
    );
    expect(
      await page.evaluate<boolean>(
        'document.documentElement.scrollWidth <= window.innerWidth',
      ),
    ).toBe(true);
  });
}

test('another customer parcel has an ownership-safe not-found state', async ({
  page,
}) => {
  await signInAs(page, 'CUSTOMER', {
    [`GET /parcels/${id}`]: fail(404, 'NOT_FOUND'),
    [`GET /parcels/${id}/history`]: fail(404, 'NOT_FOUND'),
  });
  await page.goto(`/parcels/${id}`);
  await expect(
    page.getByRole('heading', { name: 'Parcel not found' }),
  ).toBeVisible();
  await expect(page.getByText('Amali Fernando')).toHaveCount(0);
});

test('signed-out users return to login and other roles cannot create parcels', async ({
  page,
}) => {
  await mockApi(page, signedOut);
  await page.goto('/new');
  await expect(page).toHaveURL(/\/login/);
  await signInAs(page, 'COURIER', wizard);
  await page.goto('/new');
  await expect(
    page.getByRole('heading', { name: 'You do not have access' }),
  ).toBeVisible();
});

test('dispatcher route never requests the customer parcel list', async ({
  page,
}) => {
  const { calls } = await signInAs(page, 'DISPATCHER', {
    'GET /admin/parcels': ok([]),
  });
  await page.goto('/parcels');
  await expect(
    page.getByRole('heading', { name: 'All parcels', exact: true }),
  ).toBeVisible();
  await expect
    .poll(() => calls.some((call) => call.key === 'GET /admin/parcels'))
    .toBe(true);
  expect(calls.some((call) => call.key === 'GET /parcels')).toBe(false);
});

test('customer opens the private proof photo for a delivered parcel', async ({
  page,
  context,
}) => {
  const delivered = {
    ...parcel,
    status: 'DELIVERED',
    deliveredAt: '2026-11-04T10:00:00Z',
  };
  await context.route('https://storage.example/**', (route) =>
    route.fulfill({ status: 200, contentType: 'image/png', body: 'photo' }),
  );
  await signInAs(page, 'CUSTOMER', {
    [`GET /parcels/${id}`]: ok(delivered),
    [`GET /parcels/${id}/history`]: ok([
      {
        previousStatus: 'OUT_FOR_DELIVERY',
        status: 'DELIVERED',
        changedAt: '2026-11-04T10:00:00Z',
        note: 'Delivered',
      },
    ]),
    [`GET /parcels/${id}/proof/photo`]: ok({
      url: 'https://storage.example/proof.png',
      expiresAt: '2026-11-04T11:00:00Z',
      mimeType: 'image/png',
      stream: false,
    }),
  });
  await page.goto(`/parcels/${id}`);
  const popupPromise = page.waitForEvent('popup');
  await page.getByRole('button', { name: 'View proof photo' }).click();
  const popup = await popupPromise;
  await popup.waitForURL('https://storage.example/proof.png');
});
