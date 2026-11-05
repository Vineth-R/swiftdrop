import { test, expect, type Page } from '@playwright/test';

import { fail, ok, REQUEST_ID, SERVER_WORDS } from './support/mock-api';
import { signInAs } from './support/sign-in';

test.use({ colorScheme: 'light' });

const PASSWORD = 'Sw1ftDrop!Alpha-2026';
const ZONE_ID = '6b1d3f5e-0a9c-4f0e-8d57-2f0f4c1a9b10';

const emptyList = {
  status: 200,
  body: {
    data: [],
    meta: {
      requestId: REQUEST_ID,
      pagination: { page: 1, pageSize: 20, totalItems: 0, totalPages: 1 },
    },
  },
};

/** What the API answers to a create: the new account. */
const created = (changes: Record<string, unknown> = {}) =>
  ok(
    {
      id: 'new-user',
      email: 'new@example.com',
      fullName: 'Kasun Silva',
      phone: '+94712345678',
      role: 'DISPATCHER',
      isActive: true,
      createdAt: '2026-11-05T08:00:00Z',
      ...changes,
    },
    201,
  );

const colombo = { id: ZONE_ID, code: 'COL', name: 'Colombo', districts: [] };

const submit = (page: Page) =>
  page.getByRole('button', { name: /^Create account|^Creating/ });
const role = (page: Page, name: string) =>
  page.getByRole('radio', { name: new RegExp(`^${name}`) });

async function fillValid(page: Page, chosenRole = 'Dispatcher') {
  await page.getByLabel('Full name').fill('Kasun Silva');
  await page.getByLabel('Email').fill('new@example.com');
  await page.getByLabel('Phone').fill('71 234 5678');
  await role(page, chosenRole).check();
  await page.getByLabel('Initial password').fill(PASSWORD);
}

test.describe('admin create user', () => {
  test.beforeEach(async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
  });

  test('the users list offers Create user, which opens the form', async ({
    page,
  }) => {
    await signInAs(page, 'ADMIN', { 'GET /admin/users/': emptyList });
    await page.goto('/admin/users');

    await page.getByRole('link', { name: 'Create user' }).click();

    await expect(page).toHaveURL(/\/admin\/users\/new$/);
    await expect(
      page.getByRole('heading', { level: 1, name: 'Create user' }),
    ).toBeVisible();
    await expect(page.getByLabel('Full name')).toBeVisible();
    await expect(page.getByLabel('Email')).toBeVisible();
    await expect(page.getByLabel('Phone')).toBeVisible();
    await expect(page.getByLabel('Initial password')).toBeVisible();
    await expect(role(page, 'Courier')).toBeVisible();
    await expect(role(page, 'Dispatcher')).toBeVisible();
    await expect(role(page, 'Administrator')).toBeVisible();
    // Customers and merchants register themselves.
    await expect(role(page, 'Customer')).toHaveCount(0);
    await expect(role(page, 'Merchant')).toHaveCount(0);
    await expect(
      page.getByText('The email we send does not include it.'),
    ).toBeVisible();
  });

  test('courier details appear only for the Courier role', async ({ page }) => {
    await signInAs(page, 'ADMIN', { 'GET /zones': ok([colombo]) });
    await page.goto('/admin/users/new');

    await expect(page.getByLabel('Vehicle type')).toHaveCount(0);
    await role(page, 'Courier').check();
    await expect(page.getByLabel('Vehicle type')).toBeVisible();
    await expect(page.getByLabel('Vehicle number')).toBeVisible();
    await expect(page.getByLabel('Home zone')).toBeVisible();
    await role(page, 'Administrator').check();
    await expect(page.getByLabel('Vehicle type')).toHaveCount(0);
  });

  test('an empty submit explains each problem in a summary and sends nothing', async ({
    page,
  }) => {
    const { calls } = await signInAs(page, 'ADMIN');
    await page.goto('/admin/users/new');

    await submit(page).click();

    // The error summary takes focus, then links to each field.
    await expect(
      page.getByRole('alert').filter({ hasText: 'fields need attention' }),
    ).toBeFocused();
    await expect(
      page.getByText('Enter between 2 and 150 characters.').first(),
    ).toBeVisible();
    await expect(
      page.getByText('Enter your email address.').first(),
    ).toBeVisible();
    await expect(
      page.getByText('Enter a full number, for example 71 234 5678.').first(),
    ).toBeVisible();
    await expect(page.getByText('Choose a role.').first()).toBeVisible();
    await expect(
      page.getByText('Use at least 8 characters', { exact: false }).first(),
    ).toBeVisible();
    expect(calls.some((c) => c.key === 'POST /admin/users/')).toBe(false);
  });

  test('a weak password is refused before anything is sent', async ({
    page,
  }) => {
    const { calls } = await signInAs(page, 'ADMIN');
    await page.goto('/admin/users/new');

    await fillValid(page);
    await page.getByLabel('Initial password').fill('short');
    await submit(page).click();

    await expect(
      page.getByRole('alert').filter({ hasText: 'One field needs attention' }),
    ).toBeFocused();
    expect(calls.some((c) => c.key === 'POST /admin/users/')).toBe(false);
  });

  test('creating a dispatcher sends the right body, confirms and returns to the list', async ({
    page,
  }) => {
    const { calls } = await signInAs(page, 'ADMIN', {
      'POST /admin/users/': created(),
      'GET /admin/users/': emptyList,
    });
    await page.goto('/admin/users/new');

    await fillValid(page);
    await submit(page).click();

    await expect(page).toHaveURL(/\/admin\/users$/);
    await expect(
      page.getByText("Kasun Silva's account was created."),
    ).toBeVisible();
    const call = calls.find((c) => c.key === 'POST /admin/users/');
    expect(call?.body).toEqual({
      fullName: 'Kasun Silva',
      email: 'new@example.com',
      phone: '+94712345678',
      role: 'DISPATCHER',
      password: PASSWORD,
    });
  });

  test('creating a courier sends the courier profile and the chosen zone', async ({
    page,
  }) => {
    const { calls } = await signInAs(page, 'ADMIN', {
      'POST /admin/users/': created({ role: 'COURIER' }),
      'GET /admin/users/': emptyList,
      'GET /zones': ok([colombo]),
    });
    await page.goto('/admin/users/new');

    await fillValid(page, 'Courier');
    await page.getByLabel('Vehicle type').fill('Motorbike');
    await page.getByLabel('Vehicle number').fill('BHK-4821');
    await expect(page.getByLabel('Home zone')).toBeEnabled();
    await page.getByLabel('Home zone').selectOption({ label: 'Colombo' });
    await submit(page).click();

    await expect(page).toHaveURL(/\/admin\/users$/);
    const call = calls.find((c) => c.key === 'POST /admin/users/');
    expect(call?.body).toMatchObject({
      role: 'COURIER',
      courierProfile: {
        vehicleType: 'Motorbike',
        vehicleNumber: 'BHK-4821',
        homeZoneId: ZONE_ID,
      },
    });
  });

  test('a courier with no vehicle or zone sends an empty profile', async ({
    page,
  }) => {
    const { calls } = await signInAs(page, 'ADMIN', {
      'POST /admin/users/': created({ role: 'COURIER' }),
      'GET /admin/users/': emptyList,
      'GET /zones': ok([colombo]),
    });
    await page.goto('/admin/users/new');

    await fillValid(page, 'Courier');
    await submit(page).click();

    await expect(page).toHaveURL(/\/admin\/users$/);
    const call = calls.find((c) => c.key === 'POST /admin/users/');
    expect((call?.body as { courierProfile: unknown }).courierProfile).toEqual(
      {},
    );
  });

  test('a zones failure does not stop a courier being created', async ({
    page,
  }) => {
    const { calls } = await signInAs(page, 'ADMIN', {
      'POST /admin/users/': created({ role: 'COURIER' }),
      'GET /admin/users/': emptyList,
      'GET /zones': fail(500, 'INTERNAL'),
    });
    await page.goto('/admin/users/new');

    await fillValid(page, 'Courier');
    await expect(
      page.getByText('We could not load the zones.', { exact: false }),
    ).toBeVisible();
    await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
    await submit(page).click();

    await expect(page).toHaveURL(/\/admin\/users$/);
    const call = calls.find((c) => c.key === 'POST /admin/users/');
    expect(call?.body).toMatchObject({ role: 'COURIER', courierProfile: {} });
  });

  test('a registered email is explained at the email field and the input stays', async ({
    page,
  }) => {
    await signInAs(page, 'ADMIN', {
      'POST /admin/users/': fail(409, 'EMAIL_ALREADY_EXISTS'),
    });
    await page.goto('/admin/users/new');

    await fillValid(page);
    await submit(page).click();

    await expect(
      page.getByText('An account with this email already exists.').first(),
    ).toBeVisible();
    await expect(page.getByLabel('Email')).toHaveValue('new@example.com');
    await expect(page.getByLabel('Full name')).toHaveValue('Kasun Silva');
    await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
    await expect(page).toHaveURL(/\/admin\/users\/new$/);
  });

  test('a 422 is put beside its fields, in our words, and the input stays', async ({
    page,
  }) => {
    await signInAs(page, 'ADMIN', {
      'POST /admin/users/': fail(422, 'VALIDATION_ERROR', {
        details: [
          { field: 'phone', issue: 'must be in E.164 format' },
          { field: 'courierProfile.vehicleType', issue: 'too long' },
        ],
      }),
      'GET /zones': ok([colombo]),
    });
    await page.goto('/admin/users/new');

    await fillValid(page, 'Courier');
    await submit(page).click();

    await expect(
      page.getByText('Enter a full number, for example 71 234 5678.').first(),
    ).toBeVisible();
    await expect(
      page.getByText('Use 30 characters or fewer.').first(),
    ).toBeVisible();
    await expect(page.getByLabel('Phone')).toHaveValue('71 234 5678');
    await expect(page.getByText('must be in E.164 format')).toHaveCount(0);
    await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
  });

  test('a rate limit asks the person to wait and keeps what was typed', async ({
    page,
  }) => {
    await signInAs(page, 'ADMIN', {
      'POST /admin/users/': fail(429, 'RATE_LIMITED', {
        headers: { 'Retry-After': '30' },
      }),
    });
    await page.goto('/admin/users/new');

    await fillValid(page);
    await submit(page).click();

    await expect(
      page.getByRole('status').filter({ hasText: 'Too many attempts' }),
    ).toBeVisible();
    await expect(submit(page)).toBeDisabled();
    await expect(page.getByLabel('Email')).toHaveValue('new@example.com');
  });

  test('a fault on our side shows a Reference and keeps what was typed', async ({
    page,
  }) => {
    await signInAs(page, 'ADMIN', {
      'POST /admin/users/': fail(500, 'INTERNAL'),
    });
    await page.goto('/admin/users/new');

    await fillValid(page);
    await submit(page).click();

    await expect(page.getByText(`Reference ${REQUEST_ID}`)).toBeVisible();
    await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
    await expect(page.getByLabel('Email')).toHaveValue('new@example.com');
    await expect(submit(page)).toBeEnabled();
  });

  test('a lost connection says so, with no Reference', async ({ page }) => {
    await signInAs(page, 'ADMIN');
    await page.route('**/api/v1/admin/users/', (route) => route.abort());
    await page.goto('/admin/users/new');

    await fillValid(page);
    await submit(page).click();

    await expect(
      page.getByText('We could not reach SwiftDrop', { exact: false }),
    ).toBeVisible();
    await expect(page.getByText('Reference')).toHaveCount(0);
    await expect(page.getByLabel('Email')).toHaveValue('new@example.com');
  });

  test('the API refusing the role shows the no-access message', async ({
    page,
  }) => {
    await signInAs(page, 'ADMIN', {
      'POST /admin/users/': fail(403, 'FORBIDDEN'),
    });
    await page.goto('/admin/users/new');

    await fillValid(page);
    await submit(page).click();

    await expect(
      page.getByText('You do not have access to this'),
    ).toBeVisible();
    await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
  });

  test('the button is busy and cannot send twice while creating', async ({
    page,
  }) => {
    const { calls } = await signInAs(page, 'ADMIN', {
      'POST /admin/users/': { ...created(), delayMs: 600 },
      'GET /admin/users/': emptyList,
    });
    await page.goto('/admin/users/new');

    await fillValid(page);
    await submit(page).click();
    await expect(page.getByRole('button', { name: 'Creating' })).toBeVisible();
    await page.getByLabel('Full name').press('Enter');

    await expect(page).toHaveURL(/\/admin\/users$/);
    expect(calls.filter((c) => c.key === 'POST /admin/users/')).toHaveLength(1);
  });

  test('Cancel returns to the list without sending anything', async ({
    page,
  }) => {
    const { calls } = await signInAs(page, 'ADMIN', {
      'GET /admin/users/': emptyList,
    });
    await page.goto('/admin/users/new');

    await page.getByRole('link', { name: 'Cancel' }).click();

    await expect(page).toHaveURL(/\/admin\/users$/);
    expect(calls.some((c) => c.key === 'POST /admin/users/')).toBe(false);
  });

  test('anyone who is not an admin sees the 403 page', async ({ page }) => {
    await signInAs(page, 'DISPATCHER');
    await page.goto('/admin/users/new');

    await expect(
      page.getByRole('heading', { name: 'You do not have access' }),
    ).toBeVisible();
    await expect(page.getByLabel('Full name')).toHaveCount(0);
  });

  test('on a 375px phone nothing scrolls sideways and the courier fields fit', async ({
    page,
  }) => {
    await page.setViewportSize({ width: 375, height: 800 });
    await signInAs(page, 'ADMIN', { 'GET /zones': ok([colombo]) });
    await page.goto('/admin/users/new');
    await role(page, 'Courier').check();
    await expect(page.getByLabel('Home zone')).toBeVisible();

    const overflow = await page.evaluate(
      'document.documentElement.scrollWidth - document.documentElement.clientWidth',
    );
    expect(overflow).toBe(0);
    const box = await submit(page).boundingBox();
    expect(box?.height ?? 0).toBeGreaterThanOrEqual(40);
  });
});
