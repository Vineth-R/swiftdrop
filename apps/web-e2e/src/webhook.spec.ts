import { expect, test, type Page } from '@playwright/test';

import {
  fail,
  ok,
  REQUEST_ID,
  SERVER_WORDS,
  type Handler,
  type Reply,
} from './support/mock-api';
import { signInAs } from './support/sign-in';

test.use({ colorScheme: 'light' });

const merchant = {
  id: '3f1c7a52-0000-4000-8000-0000000000a1',
  ownerUserId: '3f1c7a52-0000-4000-8000-0000000000b1',
  businessName: 'ShopZone Lanka',
  contactEmail: 'support@shopzone.lk',
  contactPhone: '+94112223344',
  pickupAddress: '88 Galle Road, Colombo 03',
  pickupZoneId: '7a1e0000-0000-4000-8000-000000000001',
  status: 'APPROVED',
};
const settings = {
  url: 'https://shop.example.com/hooks/swiftdrop',
  eventTypes: ['parcel.status_changed'],
  isActive: true,
  updatedAt: '2026-11-10T08:00:00Z',
};
// Assembled at runtime so secret scanners never treat a deterministic test
// fixture as a real credential. The screens only need a non-empty value.
const SECRET = ['whsec', 'test', 'fixture'].join('_');

const attempt = (over: Record<string, unknown> = {}) => ({
  eventId: 'b7e8c3a2-1d4f-4c6a-9e0b-3f5a7d9c1e2b',
  trackingNumber: 'SDK7M2Q9XP4',
  attemptNumber: 1,
  httpStatus: 200,
  success: true,
  nextRetryAt: null,
  deadLettered: false,
  createdAt: '2026-11-11T04:00:00Z',
  ...over,
});

/** A list reply with pagination, which `ok()` does not carry. */
const listOf = (data: unknown[], totalPages = 1): Reply => ({
  status: 200,
  body: {
    data,
    meta: {
      requestId: REQUEST_ID,
      timestamp: '2026-11-11T04:00:00Z',
      pagination: {
        page: 1,
        pageSize: 20,
        totalItems: data.length,
        totalPages,
      },
    },
  },
});

const notSetUp = fail(404, 'WEBHOOK_NOT_CONFIGURED');

/** The calls every webhook page makes, for a merchant with settings saved. */
const configured: Record<string, Handler> = {
  'GET /merchants/me': ok(merchant),
  'GET /merchants/me/webhook': ok(settings),
  'GET /merchants/me/webhook/deliveries': listOf([]),
};

const address = (page: Page) => page.locator('#webhook-url');
const save = (page: Page) =>
  page.getByRole('button', { name: /^(Save webhook|Saving)$/ });
const testButton = (page: Page) =>
  page.getByRole('button', { name: /^(Send test event|Sending)$/ });
const log = (page: Page) =>
  page.getByRole('table', { name: 'Webhook delivery attempts' });

test.describe('webhook settings', () => {
  test.beforeEach(async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
  });

  test('a new merchant saves an address and sees the secret once', async ({
    page,
  }) => {
    const { calls } = await signInAs(page, 'MERCHANT', {
      ...configured,
      'GET /merchants/me/webhook': notSetUp,
      'PUT /merchants/me/webhook': ok({ ...settings, signingSecret: SECRET }),
    });
    await page.goto('/webhook');

    await expect(
      page.getByRole('heading', { level: 1, name: 'Webhook' }),
    ).toBeVisible();
    await expect(address(page)).toHaveValue('');
    await expect(testButton(page)).toBeDisabled();

    await address(page).fill(settings.url);
    await save(page).click();

    await expect(page.getByText(SECRET)).toBeVisible();
    const put = calls.find((c) => c.key === 'PUT /merchants/me/webhook');
    expect(put?.body).toEqual({
      url: settings.url,
      eventTypes: ['parcel.status_changed'],
      isActive: true,
      rotateSecret: false,
    });

    await page
      .getByRole('button', { name: 'I have saved this secret' })
      .click();
    await expect(page.getByText(SECRET)).toHaveCount(0);
    await expect(page.getByText('Webhook saved')).toBeVisible();
    await expect(testButton(page)).toBeEnabled();
  });

  test('an address that is not https is stopped before any request', async ({
    page,
  }) => {
    const { calls } = await signInAs(page, 'MERCHANT', {
      ...configured,
      'GET /merchants/me/webhook': notSetUp,
    });
    await page.goto('/webhook');
    await address(page).fill('http://shop.example.com/hooks');
    await save(page).click();

    await expect(
      page.getByText('The address must start with https://.'),
    ).toBeVisible();
    expect(calls.map((c) => c.key)).not.toContain('PUT /merchants/me/webhook');
  });

  test('says it in our own words when the server refuses the address', async ({
    page,
  }) => {
    await signInAs(page, 'MERCHANT', {
      ...configured,
      'GET /merchants/me/webhook': notSetUp,
      'PUT /merchants/me/webhook': fail(422, 'WEBHOOK_URL_NOT_ALLOWED'),
    });
    await page.goto('/webhook');
    await address(page).fill('https://internal.example.com/hook');
    await save(page).click();

    await expect(
      page.getByText('SwiftDrop cannot send to this address.'),
    ).toBeVisible();
    await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
    await expect(address(page)).toHaveValue(
      'https://internal.example.com/hook',
    );
  });

  test('a 422 that is not about the address stays general', async ({
    page,
  }) => {
    await signInAs(page, 'MERCHANT', {
      ...configured,
      'GET /merchants/me/webhook': notSetUp,
      'PUT /merchants/me/webhook': fail(422, 'VALIDATION_ERROR'),
    });
    await page.goto('/webhook');
    await address(page).fill('https://shop.example.com/hooks');
    await save(page).click();

    await expect(
      page.getByText('Check the webhook settings and try again.'),
    ).toBeVisible();
    await expect(
      page.getByText('SwiftDrop cannot send to this address.'),
    ).toHaveCount(0);
    await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
  });

  test('saved settings load, and a change is saved without a new secret', async ({
    page,
  }) => {
    const { calls } = await signInAs(page, 'MERCHANT', {
      ...configured,
      'PUT /merchants/me/webhook': ({ body }) =>
        ok({ ...settings, isActive: (body as { isActive: boolean }).isActive }),
    });
    await page.goto('/webhook');

    await expect(address(page)).toHaveValue(settings.url);
    await expect(save(page)).toBeDisabled();
    await expect(page.getByText('Change something to save.')).toBeVisible();

    await page.getByLabel('Send events to this address').uncheck();
    await save(page).click();

    await expect(page.getByText('Webhook saved')).toBeVisible();
    await expect(page.getByText('Signing secret: copy it now')).toHaveCount(0);
    const put = calls.find((c) => c.key === 'PUT /merchants/me/webhook');
    expect(put?.body).toMatchObject({ isActive: false, rotateSecret: false });
  });

  test('asking for a new secret sends rotateSecret and shows it once', async ({
    page,
  }) => {
    const { calls } = await signInAs(page, 'MERCHANT', {
      ...configured,
      'PUT /merchants/me/webhook': ok({ ...settings, signingSecret: SECRET }),
    });
    await page.goto('/webhook');

    await page.getByLabel('Generate a new signing secret').check();
    await save(page).click();

    await expect(page.getByText(SECRET)).toBeVisible();
    const put = calls.find((c) => c.key === 'PUT /merchants/me/webhook');
    expect(put?.body).toMatchObject({ rotateSecret: true });
    await expect(
      page.getByLabel('Generate a new signing secret'),
    ).not.toBeChecked();
  });

  test('a rate limit tells the merchant to wait and keeps the address', async ({
    page,
  }) => {
    await signInAs(page, 'MERCHANT', {
      ...configured,
      'PUT /merchants/me/webhook': fail(429, 'RATE_LIMITED', {
        headers: { 'Retry-After': '30' },
      }),
    });
    await page.goto('/webhook');
    await address(page).fill('https://shop.example.com/new');
    await save(page).click();

    await expect(page.getByText('Too many attempts')).toBeVisible();
    await expect(address(page)).toHaveValue('https://shop.example.com/new');
  });

  test('a server fault on load offers another try', async ({ page }) => {
    // A flag, not a call count: the dev server runs effects twice.
    let isDown = true;
    await signInAs(page, 'MERCHANT', {
      ...configured,
      'GET /merchants/me/webhook': () =>
        isDown ? fail(500, 'INTERNAL') : ok(settings),
    });
    await page.goto('/webhook');

    await expect(
      page.getByText('Could not load your webhook settings'),
    ).toBeVisible();
    await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
    isDown = false;
    await page.getByRole('button', { name: 'Try again' }).first().click();
    await expect(address(page)).toHaveValue(settings.url);
  });
});

test.describe('test event', () => {
  test.beforeEach(async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
  });

  test('reports a delivered, an unanswered and a refused test', async ({
    page,
  }) => {
    const replies = [
      ok({ delivered: true, httpStatus: 200, durationMs: 120 }),
      ok({ delivered: false, httpStatus: 500, durationMs: 340 }),
      ok({ delivered: false, httpStatus: null, durationMs: 10000 }),
    ];
    let next = 0;
    const { calls } = await signInAs(page, 'MERCHANT', {
      ...configured,
      'POST /merchants/me/webhook/test': () => replies[next++],
    });
    await page.goto('/webhook');
    await expect(address(page)).toHaveValue(settings.url);

    await testButton(page).click();
    await expect(page.getByText('Test event delivered')).toBeVisible();
    await expect(
      page.getByText('Your server answered HTTP 200 in 120 ms.'),
    ).toBeVisible();

    await testButton(page).click();
    await expect(page.getByText('Your server answered HTTP 500')).toBeVisible();
    await expect(page.getByText('Test event delivered')).toHaveCount(0);

    await testButton(page).click();
    await expect(page.getByText('No answer from your server')).toBeVisible();
    expect(
      calls.filter((c) => c.key === 'POST /merchants/me/webhook/test'),
    ).toHaveLength(3);
  });

  test('a test with no saved webhook is explained, not shown as a fault', async ({
    page,
  }) => {
    await signInAs(page, 'MERCHANT', {
      ...configured,
      'POST /merchants/me/webhook/test': notSetUp,
    });
    await page.goto('/webhook');
    await testButton(page).click();

    await expect(page.getByText('Save your webhook first')).toBeVisible();
    await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
  });
});

test.describe('delivery log', () => {
  test.beforeEach(async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
  });

  test('lists attempts with a label for each outcome', async ({ page }) => {
    await signInAs(page, 'MERCHANT', {
      ...configured,
      'GET /merchants/me/webhook/deliveries': listOf([
        attempt(),
        attempt({
          eventId: 'c1111111-1d4f-4c6a-9e0b-3f5a7d9c1e2b',
          attemptNumber: 2,
          success: false,
          httpStatus: 500,
          nextRetryAt: '2026-11-11T04:06:00Z',
        }),
        attempt({
          eventId: 'd2222222-1d4f-4c6a-9e0b-3f5a7d9c1e2b',
          attemptNumber: 6,
          success: false,
          httpStatus: null,
          deadLettered: true,
        }),
      ]),
    });
    await page.goto('/webhook');

    await expect(log(page).getByText('Delivered')).toBeVisible();
    await expect(log(page).getByText('Will retry')).toBeVisible();
    await expect(log(page).getByText('Gave up')).toBeVisible();
    await expect(log(page).getByText('2 of 6')).toBeVisible();
    await expect(log(page).getByText('No response')).toBeVisible();
    await expect(
      log(page).getByText('11 Nov 2026, 09:30').first(),
    ).toBeVisible();
    await expect(
      log(page).getByText('Next try 11 Nov 2026, 09:36'),
    ).toBeVisible();
    await expect(
      log(page).getByText('b7e8c3a2-1d4f-4c6a-9e0b-3f5a7d9c1e2b'),
    ).toBeVisible();
  });

  test('filters by result and tracking number, then clears them', async ({
    page,
  }) => {
    const { calls } = await signInAs(page, 'MERCHANT', configured);
    await page.goto('/webhook');

    await page.getByLabel('Result').selectOption('false');
    await page.getByLabel('Tracking number').fill('SDK7M2Q9XP4');
    await page.getByRole('button', { name: 'Apply filters' }).click();

    await expect(page.getByText('No attempts match')).toBeVisible();
    const filtered = calls
      .filter((c) => c.key === 'GET /merchants/me/webhook/deliveries')
      .at(-1);
    const query = new URL(filtered?.url ?? '').searchParams;
    expect(query.get('success')).toBe('false');
    expect(query.get('trackingNumber')).toBe('SDK7M2Q9XP4');
    expect(query.get('page')).toBe('1');

    await page.getByRole('button', { name: 'Clear' }).click();
    await expect(page.getByText('No deliveries yet')).toBeVisible();
    const cleared = calls
      .filter((c) => c.key === 'GET /merchants/me/webhook/deliveries')
      .at(-1);
    expect(new URL(cleared?.url ?? '').searchParams.get('success')).toBeNull();
  });

  test('an empty log says what makes attempts appear', async ({ page }) => {
    await signInAs(page, 'MERCHANT', configured);
    await page.goto('/webhook');

    await expect(page.getByText('No deliveries yet')).toBeVisible();
    await expect(
      page.getByText(
        'Attempts appear here once one of your parcels changes status.',
      ),
    ).toBeVisible();
  });

  test('pages through the log and blocks a second click while loading', async ({
    page,
  }) => {
    const { calls } = await signInAs(page, 'MERCHANT', {
      ...configured,
      'GET /merchants/me/webhook/deliveries': ({ query }) => ({
        ...listOf([attempt({ attemptNumber: Number(query.get('page')) })], 3),
        delayMs: 300,
      }),
    });
    await page.goto('/webhook');

    await expect(page.getByText('Page 1 of 3')).toBeVisible();
    const next = page.getByRole('button', { name: 'Next', exact: true });
    await next.click();
    // The page is changing: the control stays, but cannot be pressed again.
    await expect(next).toBeDisabled();
    await expect(page.getByText('Page 2 of 3')).toBeVisible();
    await expect(next).toBeEnabled();

    const pageTwo = calls.filter(
      (c) =>
        c.key === 'GET /merchants/me/webhook/deliveries' &&
        new URL(c.url).searchParams.get('page') === '2',
    );
    expect(pageTwo).toHaveLength(1);
  });

  test('a failed load shows our words and recovers on retry', async ({
    page,
  }) => {
    let isDown = true;
    await signInAs(page, 'MERCHANT', {
      ...configured,
      'GET /merchants/me/webhook/deliveries': () =>
        isDown ? fail(500, 'INTERNAL') : listOf([attempt()]),
    });
    await page.goto('/webhook');

    await expect(
      page.getByText('Could not load the delivery log'),
    ).toBeVisible();
    await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
    isDown = false;
    await page.getByRole('button', { name: 'Try again' }).click();
    await expect(log(page).getByText('SDK7M2Q9XP4')).toBeVisible();
  });

  test('does not scroll sideways at 375px', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 800 });
    await signInAs(page, 'MERCHANT', {
      ...configured,
      'GET /merchants/me/webhook/deliveries': listOf([attempt()]),
    });
    await page.goto('/webhook');

    await expect(page.getByText('SDK7M2Q9XP4').last()).toBeVisible();
    expect(
      await page.evaluate<boolean>(
        'document.documentElement.scrollWidth <= window.innerWidth',
      ),
    ).toBe(true);
  });
});

test.describe('who can open it', () => {
  test('a merchant that is not approved sees the notice and no form', async ({
    page,
  }) => {
    const { calls } = await signInAs(page, 'MERCHANT', {
      ...configured,
      'GET /merchants/me': ok({ ...merchant, status: 'PENDING' }),
    });
    await page.goto('/webhook');

    await expect(
      page.getByText('Merchant account pending approval'),
    ).toBeVisible();
    await expect(address(page)).toHaveCount(0);
    expect(calls.map((c) => c.key)).not.toContain('GET /merchants/me/webhook');
  });

  test('the API refusing an unapproved merchant shows the notice, not a fault', async ({
    page,
  }) => {
    await signInAs(page, 'MERCHANT', {
      ...configured,
      'GET /merchants/me/webhook': fail(403, 'MERCHANT_NOT_APPROVED'),
    });
    await page.goto('/webhook');

    await expect(page.getByText('Webhook is not available yet')).toBeVisible();
    await expect(address(page)).toHaveCount(0);
    await expect(
      page.getByText('Could not load your webhook settings'),
    ).toHaveCount(0);
    await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
  });

  test('a customer gets the no-access page', async ({ page }) => {
    const { calls } = await signInAs(page, 'CUSTOMER');
    await page.goto('/webhook');

    await expect(page.getByText('You do not have access')).toBeVisible();
    expect(calls.map((c) => c.key)).not.toContain('GET /merchants/me/webhook');
  });
});
