import { expect, test, type Page } from '@playwright/test';

import {
  SERVER_WORDS,
  fail,
  mockApi,
  ok,
  signedOut,
  type Handler,
} from './support/mock-api';

test.use({ colorScheme: 'light' });

const zone = {
  id: '7a1e0000-0000-4000-8000-000000000001',
  code: 'CMB',
  name: 'Colombo',
};
const created = ok(
  {
    merchantId: '3f1c7a52-0000-4000-8000-0000000000a1',
    userId: '3f1c7a52-0000-4000-8000-0000000000b1',
    status: 'PENDING',
  },
  201,
);

/** Opens the form with the zone list loaded and `register` answering the submit. */
async function open(page: Page, register: Handler) {
  const api = await mockApi(page, {
    ...signedOut,
    'GET /zones': ok([zone]),
    'POST /merchants/register': register,
  });
  await page.goto('/register/merchant');
  await expect(page.getByLabel('Default pickup zone')).toBeEnabled();
  return api;
}

async function fillValid(page: Page) {
  await page.getByLabel('Full name').fill('  Ruwan Jayasuriya ');
  await page.locator('#merchant-email').fill('owner@shop.lk');
  await page.getByLabel('Phone number').fill('71 333 4455');
  await page.locator('#merchant-password').fill('Str0ngPass!');
  await page.getByLabel('Confirm password').fill('Str0ngPass!');
  await page.getByLabel('Business name').fill('ShopZone Lanka');
  await page.getByLabel('Business registration number').fill('PV 123456');
  await page.getByLabel('Business address').fill('88 Galle Road, Colombo 03');
  await page
    .getByLabel('Default pickup address')
    .fill('88 Galle Road, Colombo 03');
  await page.getByLabel('Default pickup zone').selectOption(zone.id);
}

const submit = (page: Page) =>
  page.getByRole('button', { name: /Submit application|Submitting/ });

test.describe('merchant registration form', () => {
  test('an empty form says what is missing and sends nothing', async ({
    page,
  }) => {
    const { calls } = await open(page, created);

    await submit(page).click();

    await expect(
      page.getByText('Enter your email address.', { exact: true }),
    ).toBeVisible();
    await expect(
      page.getByText(
        'Enter 3 to 50 characters with at least one digit, for example PV 123456.',
        { exact: true },
      ),
    ).toBeVisible();
    await expect(
      page.getByText('Choose a pickup zone.', { exact: true }),
    ).toBeVisible();
    // The summary of problems takes focus and links to each field.
    await expect(
      page.getByRole('alert').filter({ hasText: 'Business name' }),
    ).toBeFocused();
    expect(calls.map((call) => call.key)).not.toContain(
      'POST /merchants/register',
    );
  });

  test('a mismatched confirmation is caught on the field, and fixing it clears it', async ({
    page,
  }) => {
    await open(page, created);
    await fillValid(page);
    await page.getByLabel('Confirm password').fill('Different1!');
    await submit(page).click();

    await expect(
      page.getByText('The two passwords do not match.', { exact: true }),
    ).toBeVisible();

    await page.getByLabel('Confirm password').fill('Str0ngPass!');
    await expect(
      page.getByText('The two passwords do not match.', { exact: true }),
    ).toHaveCount(0);
  });

  test('sends tidy values and fills the business contact from the account', async ({
    page,
  }) => {
    const { calls } = await open(page, created);
    await fillValid(page);

    await submit(page).click();

    await expect(page.getByText('Application submitted')).toBeVisible();
    const sent = calls.find((call) => call.key === 'POST /merchants/register');
    expect(sent?.body).toEqual({
      fullName: 'Ruwan Jayasuriya',
      email: 'owner@shop.lk',
      phone: '+94713334455',
      password: 'Str0ngPass!',
      business: {
        name: 'ShopZone Lanka',
        registrationNumber: 'PV 123456',
        contactEmail: 'owner@shop.lk',
        contactPhone: '+94713334455',
        businessAddress: '88 Galle Road, Colombo 03',
        pickupAddress: '88 Galle Road, Colombo 03',
        pickupZoneId: zone.id,
      },
    });
  });

  test('a business phone and email that were typed are sent instead of the defaults', async ({
    page,
  }) => {
    const { calls } = await open(page, created);
    await fillValid(page);
    await page.getByLabel('Business email').fill('support@shop.lk');
    await page.getByLabel('Business phone').fill('011 222 3344');

    await submit(page).click();

    await expect(page.getByText('Application submitted')).toBeVisible();
    const sent = calls.find((call) => call.key === 'POST /merchants/register');
    expect(sent?.body).toMatchObject({
      business: {
        contactEmail: 'support@shop.lk',
        contactPhone: '+94112223344',
      },
    });
  });

  test('a field the API rejects gets our own sentence and keeps what was typed', async ({
    page,
  }) => {
    await open(
      page,
      fail(422, 'VALIDATION_ERROR', {
        details: [
          { field: 'business.registrationNumber', issue: SERVER_WORDS },
        ],
      }),
    );
    await fillValid(page);

    await submit(page).click();

    await expect(
      page.getByText(
        'Enter 3 to 50 characters with at least one digit, for example PV 123456.',
        { exact: true },
      ),
    ).toBeVisible();
    await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
    await expect(page.getByLabel('Business registration number')).toHaveValue(
      'PV 123456',
    );
    await expect(page.getByLabel('Business name')).toHaveValue(
      'ShopZone Lanka',
    );
  });

  test('an email that is already registered points to log in and reset', async ({
    page,
  }) => {
    await open(page, fail(409, 'EMAIL_ALREADY_EXISTS'));
    await fillValid(page);

    await submit(page).click();

    await expect(page.locator('#merchant-email-error')).toContainText(
      'This email is already registered.',
    );
    await expect(
      page.getByRole('link', { name: 'Log in instead' }),
    ).toBeVisible();
    await expect(
      page.getByRole('link', { name: 'reset the password' }),
    ).toBeVisible();
    await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
  });

  test('an application that already exists is explained, not shown as a server fault', async ({
    page,
  }) => {
    await open(page, fail(409, 'DUPLICATE_MERCHANT_APPLICATION'));
    await fillValid(page);

    await submit(page).click();

    await expect(page.getByText('An application already exists')).toBeVisible();
    await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
  });

  test('a rate limit counts down on the button', async ({ page }) => {
    await open(
      page,
      fail(429, 'RATE_LIMITED', { headers: { 'Retry-After': '30' } }),
    );
    await fillValid(page);

    await submit(page).click();

    await expect(
      page.getByRole('button', { name: /Try again in/ }),
    ).toBeDisabled();
  });

  test('a server fault shows a reference and can be retried with the same key', async ({
    page,
  }) => {
    const keys: string[] = [];
    page.on('request', (request) => {
      if (request.url().endsWith('/merchants/register'))
        keys.push(request.headers()['idempotency-key'] ?? '');
    });
    let attempts = 0;
    await open(page, () => {
      attempts += 1;
      return attempts < 3
        ? fail(503, 'SERVICE_UNAVAILABLE')
        : (created as ReturnType<typeof fail>);
    });
    await fillValid(page);

    await submit(page).click();
    await expect(page.getByText('Reference')).toBeVisible();
    await expect(submit(page)).toBeEnabled();
    await submit(page).click();
    await expect.poll(() => keys.length).toBe(2);
    expect(keys[0]).toBeTruthy();
    // The same application, sent again, is the same request.
    expect(keys[1]).toBe(keys[0]);

    // Changing what is sent starts a new application, so it gets a new key.
    await page.getByLabel('Business name').fill('ShopZone Lanka Two');
    await submit(page).click();
    await expect(page.getByText('Application submitted')).toBeVisible();
    expect(keys).toHaveLength(3);
    expect(keys[2]).not.toBe(keys[0]);
  });

  test('pickup zones that fail to load can be retried', async ({ page }) => {
    let zoneCalls = 0;
    await mockApi(page, {
      ...signedOut,
      'GET /zones': () => {
        zoneCalls += 1;
        return zoneCalls === 1
          ? fail(503, 'SERVICE_UNAVAILABLE')
          : (ok([zone]) as ReturnType<typeof fail>);
      },
    });
    await page.goto('/register/merchant');

    await expect(page.getByText('Could not load pickup zones')).toBeVisible();
    await expect(submit(page)).toBeDisabled();
    await page.getByRole('button', { name: 'Retry zones' }).click();

    await expect(page.getByLabel('Default pickup zone')).toBeEnabled();
    await expect(submit(page)).toBeEnabled();
  });

  test('nothing scrolls sideways at 375px', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    await open(page, created);
    await submit(page).click();

    const overflow = await page.evaluate<number>(
      'document.documentElement.scrollWidth - window.innerWidth',
    );
    expect(overflow).toBeLessThanOrEqual(0);
  });
});
