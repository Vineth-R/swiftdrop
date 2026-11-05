import { test, expect, type Page } from '@playwright/test';

import {
  fail,
  ok,
  REQUEST_ID,
  SERVER_WORDS,
  signedOut,
  user as signedInUser,
} from './support/mock-api';
import { signInAs } from './support/sign-in';

test.use({ colorScheme: 'light' });

const ZONE_ID = '6b1d3f5e-0a9c-4f0e-8d57-2f0f4c1a9b10';
const OTHER_ZONE_ID = '0e9d2a44-3b1f-4c7a-9d10-5a6b7c8d9e01';
const colombo = { id: ZONE_ID, code: 'COL', name: 'Colombo', districts: [] };
const kandy = { id: OTHER_ZONE_ID, code: 'KDY', name: 'Kandy', districts: [] };

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

/** One user as GET /admin/users/{id} returns them (API v1.1 section 2.12). */
const detail = (id: string, changes: Record<string, unknown> = {}) => ({
  id,
  email: `${id}@example.com`,
  fullName: 'Kasun Silva',
  phone: '+94712345678',
  role: 'DISPATCHER',
  isActive: true,
  createdAt: '2026-10-01T08:00:00Z',
  lastLoginAt: null,
  updatedAt: '2026-10-02T08:00:00Z',
  ...changes,
});

const courier = (id: string, changes: Record<string, unknown> = {}) =>
  detail(id, {
    role: 'COURIER',
    courierProfile: {
      vehicleType: 'Motorbike',
      vehicleNumber: 'BHK-4821',
      homeZoneId: ZONE_ID,
      isAvailable: true,
    },
    ...changes,
  });

const editUrl = (id: string) => `/admin/users/${id}/edit`;
const getKey = (id: string) => `GET /admin/users/${id}`;
const patchKey = (id: string) => `PATCH /admin/users/${id}`;

const save = (page: Page) =>
  page.getByRole('button', { name: /^Save changes|^Saving/ });
const role = (page: Page, name: string) =>
  page.getByRole('radio', { name: new RegExp(`^${name}`) });
const patches = (calls: { key: string; body: unknown }[], id: string) =>
  calls.filter((c) => c.key === patchKey(id));

test.describe('admin edit user', () => {
  test.beforeEach(async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
  });

  test('the users list offers Edit, which opens that user', async ({
    page,
  }) => {
    const listReply = {
      status: 200,
      body: {
        data: [
          {
            id: 'kasun',
            email: 'kasun@example.com',
            fullName: 'Kasun Silva',
            phone: '+94712345678',
            role: 'DISPATCHER',
            isActive: true,
            createdAt: '2026-10-01T08:00:00Z',
          },
        ],
        meta: {
          requestId: REQUEST_ID,
          pagination: { page: 1, pageSize: 20, totalItems: 1, totalPages: 1 },
        },
      },
    };
    await signInAs(page, 'ADMIN', {
      'GET /admin/users/': listReply,
      [getKey('kasun')]: ok(detail('kasun')),
    });
    await page.goto('/admin/users');

    await page.getByRole('link', { name: 'Edit Kasun Silva' }).click();

    await expect(page).toHaveURL(/\/admin\/users\/kasun\/edit$/);
    await expect(
      page.getByRole('heading', { level: 1, name: 'Edit user' }),
    ).toBeVisible();
    await expect(page.getByLabel('Full name')).toHaveValue('Kasun Silva');
  });

  test("the form is filled with the user's current details", async ({
    page,
  }) => {
    await signInAs(page, 'ADMIN', { [getKey('kasun')]: ok(detail('kasun')) });
    await page.goto(editUrl('kasun'));

    await expect(page.getByLabel('Full name')).toHaveValue('Kasun Silva');
    await expect(page.getByLabel('Email')).toHaveValue('kasun@example.com');
    await expect(page.getByLabel('Email')).toHaveAttribute('readonly', '');
    await expect(page.getByLabel('Phone')).toHaveValue('712345678');
    await expect(role(page, 'Dispatcher')).toBeChecked();
    await expect(role(page, 'Courier')).not.toBeChecked();
    // Nothing to save until something changes, and no password field exists.
    await expect(save(page)).toBeDisabled();
    await expect(page.getByText('Change a field to save.')).toBeVisible();
    await expect(page.getByLabel('Initial password')).toHaveCount(0);
    await expect(page.getByLabel('Vehicle type')).toHaveCount(0);
  });

  test('a loading state shows while the user is fetched', async ({ page }) => {
    await signInAs(page, 'ADMIN', {
      [getKey('kasun')]: { ...ok(detail('kasun')), delayMs: 500 },
    });
    await page.goto(editUrl('kasun'));

    await expect(
      page.getByRole('status').filter({ hasText: 'Loading user' }),
    ).toBeVisible();
    await expect(page.getByLabel('Full name')).toBeVisible();
  });

  test('saving sends only what changed, confirms and returns to the list', async ({
    page,
  }) => {
    const { calls } = await signInAs(page, 'ADMIN', {
      [getKey('kasun')]: ok(detail('kasun')),
      [patchKey('kasun')]: ok(detail('kasun', { fullName: 'Kasun S. Silva' })),
      'GET /admin/users/': emptyList,
    });
    await page.goto(editUrl('kasun'));

    await page.getByLabel('Full name').fill('Kasun S. Silva');
    await expect(save(page)).toBeEnabled();
    await save(page).click();

    await expect(page).toHaveURL(/\/admin\/users$/);
    await expect(page.getByText('Kasun S. Silva was updated.')).toBeVisible();
    expect(patches(calls, 'kasun')).toHaveLength(1);
    expect(patches(calls, 'kasun')[0].body).toEqual({
      fullName: 'Kasun S. Silva',
    });
  });

  test('a changed phone is sent as E.164', async ({ page }) => {
    const { calls } = await signInAs(page, 'ADMIN', {
      [getKey('kasun')]: ok(detail('kasun')),
      [patchKey('kasun')]: ok(detail('kasun', { phone: '+94771112223' })),
      'GET /admin/users/': emptyList,
    });
    await page.goto(editUrl('kasun'));

    await page.getByLabel('Phone').fill('77 111 2223');
    await save(page).click();

    await expect(page).toHaveURL(/\/admin\/users$/);
    expect(patches(calls, 'kasun')[0].body).toEqual({ phone: '+94771112223' });
  });

  test('making someone a courier shows the courier fields and sends a profile', async ({
    page,
  }) => {
    const { calls } = await signInAs(page, 'ADMIN', {
      [getKey('kasun')]: ok(detail('kasun')),
      [patchKey('kasun')]: ok(courier('kasun')),
      'GET /admin/users/': emptyList,
      'GET /zones': ok([colombo, kandy]),
    });
    await page.goto(editUrl('kasun'));

    await role(page, 'Courier').check();
    await expect(page.getByLabel('Vehicle type')).toBeVisible();
    await expect(page.getByLabel('Home zone')).toBeEnabled();
    await page.getByLabel('Vehicle type').fill('Van');
    await page.getByLabel('Home zone').selectOption({ label: 'Kandy' });
    await save(page).click();

    await expect(page).toHaveURL(/\/admin\/users$/);
    expect(patches(calls, 'kasun')[0].body).toEqual({
      role: 'COURIER',
      courierProfile: { vehicleType: 'Van', homeZoneId: OTHER_ZONE_ID },
    });
  });

  test('becoming a courier with no details still sends the profile the API requires', async ({
    page,
  }) => {
    const { calls } = await signInAs(page, 'ADMIN', {
      [getKey('kasun')]: ok(detail('kasun')),
      [patchKey('kasun')]: ok(courier('kasun')),
      'GET /admin/users/': emptyList,
      'GET /zones': ok([colombo]),
    });
    await page.goto(editUrl('kasun'));

    await role(page, 'Courier').check();
    await save(page).click();

    await expect(page).toHaveURL(/\/admin\/users$/);
    expect(patches(calls, 'kasun')[0].body).toEqual({
      role: 'COURIER',
      courierProfile: {},
    });
  });

  test("a courier's vehicle, zone and availability are loaded and editable", async ({
    page,
  }) => {
    const { calls } = await signInAs(page, 'ADMIN', {
      [getKey('kasun')]: ok(courier('kasun')),
      [patchKey('kasun')]: ok(courier('kasun')),
      'GET /admin/users/': emptyList,
      'GET /zones': ok([colombo, kandy]),
    });
    await page.goto(editUrl('kasun'));

    await expect(role(page, 'Courier')).toBeChecked();
    await expect(page.getByLabel('Vehicle type')).toHaveValue('Motorbike');
    await expect(page.getByLabel('Vehicle number')).toHaveValue('BHK-4821');
    await expect(page.getByLabel('Home zone')).toBeEnabled();
    await expect(page.getByLabel('Home zone')).toHaveValue(ZONE_ID);
    const available = page.getByRole('checkbox', {
      name: /Available for assignments/,
    });
    await expect(available).toBeChecked();

    await page.getByLabel('Vehicle number').fill('CAB-1234');
    await available.uncheck();
    await save(page).click();

    await expect(page).toHaveURL(/\/admin\/users$/);
    expect(patches(calls, 'kasun')[0].body).toEqual({
      courierProfile: { vehicleNumber: 'CAB-1234', isAvailable: false },
    });
  });

  test('moving a courier to another role sends only the role', async ({
    page,
  }) => {
    const { calls } = await signInAs(page, 'ADMIN', {
      [getKey('kasun')]: ok(courier('kasun')),
      [patchKey('kasun')]: ok(courier('kasun', { role: 'DISPATCHER' })),
      'GET /admin/users/': emptyList,
      'GET /zones': ok([colombo]),
    });
    await page.goto(editUrl('kasun'));

    await role(page, 'Dispatcher').check();
    await expect(page.getByLabel('Vehicle type')).toHaveCount(0);
    await expect(
      page.getByText('Their courier details are kept', { exact: false }),
    ).toBeVisible();
    await save(page).click();

    await expect(page).toHaveURL(/\/admin\/users$/);
    expect(patches(calls, 'kasun')[0].body).toEqual({ role: 'DISPATCHER' });
  });

  test('a vehicle detail that was set cannot be cleared', async ({ page }) => {
    const { calls } = await signInAs(page, 'ADMIN', {
      [getKey('kasun')]: ok(courier('kasun')),
      'GET /zones': ok([colombo]),
    });
    await page.goto(editUrl('kasun'));

    await page.getByLabel('Vehicle type').fill('');
    await page.getByLabel('Full name').fill('Kasun S. Silva');
    await save(page).click();

    await expect(
      page.getByText('This cannot be cleared. Enter a new value.').first(),
    ).toBeVisible();
    expect(patches(calls, 'kasun')).toHaveLength(0);
  });

  test('a zone that is no longer listed stays as it is', async ({ page }) => {
    await signInAs(page, 'ADMIN', {
      [getKey('kasun')]: ok(courier('kasun')),
      'GET /zones': ok([kandy]),
    });
    await page.goto(editUrl('kasun'));

    await expect(page.getByLabel('Home zone')).toBeEnabled();
    await expect(
      page.getByLabel('Home zone').locator('option:checked'),
    ).toHaveText('Current zone (not listed)');
  });

  test('a zones failure does not stop a courier being edited', async ({
    page,
  }) => {
    const { calls } = await signInAs(page, 'ADMIN', {
      [getKey('kasun')]: ok(courier('kasun')),
      [patchKey('kasun')]: ok(courier('kasun', { fullName: 'Kasun S. Silva' })),
      'GET /admin/users/': emptyList,
      'GET /zones': fail(500, 'INTERNAL'),
    });
    await page.goto(editUrl('kasun'));

    await expect(
      page.getByText('We could not load the zones.', { exact: false }),
    ).toBeVisible();
    await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
    await page.getByLabel('Full name').fill('Kasun S. Silva');
    await save(page).click();

    await expect(page).toHaveURL(/\/admin\/users$/);
    expect(patches(calls, 'kasun')[0].body).toEqual({
      fullName: 'Kasun S. Silva',
    });
  });

  test('an empty name is explained in a summary and nothing is sent', async ({
    page,
  }) => {
    const { calls } = await signInAs(page, 'ADMIN', {
      [getKey('kasun')]: ok(detail('kasun')),
    });
    await page.goto(editUrl('kasun'));

    await page.getByLabel('Full name').fill('');
    await save(page).click();

    await expect(
      page.getByRole('alert').filter({ hasText: 'One field needs attention' }),
    ).toBeFocused();
    await expect(
      page.getByText('Enter between 2 and 150 characters.').first(),
    ).toBeVisible();
    expect(patches(calls, 'kasun')).toHaveLength(0);
  });

  test('a phone that is too short is refused before anything is sent', async ({
    page,
  }) => {
    const { calls } = await signInAs(page, 'ADMIN', {
      [getKey('kasun')]: ok(detail('kasun')),
    });
    await page.goto(editUrl('kasun'));

    await page.getByLabel('Phone').fill('123');
    await save(page).click();

    await expect(
      page.getByText('Enter a full number, for example 71 234 5678.').first(),
    ).toBeVisible();
    expect(patches(calls, 'kasun')).toHaveLength(0);
  });

  test('a 422 is put beside its fields, in our words, and the input stays', async ({
    page,
  }) => {
    await signInAs(page, 'ADMIN', {
      [getKey('kasun')]: ok(detail('kasun')),
      [patchKey('kasun')]: fail(422, 'VALIDATION_ERROR', {
        details: [{ field: 'phone', issue: 'must be in E.164 format' }],
      }),
    });
    await page.goto(editUrl('kasun'));

    await page.getByLabel('Phone').fill('77 111 2223');
    await save(page).click();

    await expect(
      page.getByText('Enter a full number, for example 71 234 5678.').first(),
    ).toBeVisible();
    await expect(page.getByLabel('Phone')).toHaveValue('77 111 2223');
    await expect(page.getByText('must be in E.164 format')).toHaveCount(0);
    await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
    await expect(page).toHaveURL(/\/edit$/);
  });

  test('an administrator cannot change their own role', async ({ page }) => {
    const { calls } = await signInAs(page, 'ADMIN', {
      [getKey(signedInUser.id)]: ok(
        detail(signedInUser.id, { role: 'ADMIN', fullName: 'Nimal Perera' }),
      ),
      [patchKey(signedInUser.id)]: ok(
        detail(signedInUser.id, { role: 'ADMIN', fullName: 'Nimal P. Perera' }),
      ),
      'GET /admin/users/': emptyList,
    });
    await page.goto(editUrl(signedInUser.id));

    await expect(role(page, 'Administrator')).toBeChecked();
    await expect(role(page, 'Courier')).toBeDisabled();
    await expect(
      page.getByText('You cannot change your own role.'),
    ).toBeVisible();

    // Their name can still be edited.
    await page.getByLabel('Full name').fill('Nimal P. Perera');
    await save(page).click();
    await expect(page).toHaveURL(/\/admin\/users$/);
    expect(patches(calls, signedInUser.id)[0].body).toEqual({
      fullName: 'Nimal P. Perera',
    });
  });

  test('the API refusing a role change is explained at the role', async ({
    page,
  }) => {
    await signInAs(page, 'ADMIN', {
      [getKey('kasun')]: ok(detail('kasun')),
      [patchKey('kasun')]: fail(409, 'CONFLICT'),
    });
    await page.goto(editUrl('kasun'));

    await role(page, 'Customer').check();
    await save(page).click();

    await expect(
      page.getByText('You cannot change your own role.').first(),
    ).toBeVisible();
    await expect(role(page, 'Customer')).toBeChecked();
    await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
  });

  test("a merchant's role is shown but cannot be changed", async ({ page }) => {
    const { calls } = await signInAs(page, 'ADMIN', {
      [getKey('ruwan')]: ok(detail('ruwan', { role: 'MERCHANT' })),
      [patchKey('ruwan')]: ok(
        detail('ruwan', { role: 'MERCHANT', fullName: 'Ruwan J.' }),
      ),
      'GET /admin/users/': emptyList,
    });
    await page.goto(editUrl('ruwan'));

    await expect(page.getByRole('radio')).toHaveCount(0);
    await expect(page.getByLabel('Role')).toHaveValue('MERCHANT');
    await page.getByLabel('Full name').fill('Ruwan J.');
    await save(page).click();

    await expect(page).toHaveURL(/\/admin\/users$/);
    expect(patches(calls, 'ruwan')[0].body).toEqual({ fullName: 'Ruwan J.' });
  });

  test('a user that does not exist shows a not-found state', async ({
    page,
  }) => {
    await signInAs(page, 'ADMIN', {
      [getKey('ghost')]: fail(404, 'NOT_FOUND'),
    });
    await page.goto(editUrl('ghost'));

    await expect(page.getByText('This account does not exist')).toBeVisible();
    await expect(
      page.getByRole('link', { name: 'Back to users' }),
    ).toBeVisible();
    await expect(page.getByLabel('Full name')).toHaveCount(0);
    await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
  });

  test('a user removed while editing is reported and nothing is saved', async ({
    page,
  }) => {
    await signInAs(page, 'ADMIN', {
      [getKey('kasun')]: ok(detail('kasun')),
      [patchKey('kasun')]: fail(404, 'NOT_FOUND'),
    });
    await page.goto(editUrl('kasun'));

    await page.getByLabel('Full name').fill('Kasun S. Silva');
    await save(page).click();

    await expect(page.getByText('This account no longer exists')).toBeVisible();
    await expect(page.getByLabel('Full name')).toHaveValue('Kasun S. Silva');
  });

  test('the API refusing the load shows the no-access message', async ({
    page,
  }) => {
    await signInAs(page, 'ADMIN', {
      [getKey('kasun')]: fail(403, 'FORBIDDEN'),
    });
    await page.goto(editUrl('kasun'));

    await expect(
      page.getByText('You do not have access to this'),
    ).toBeVisible();
    await expect(page.getByLabel('Full name')).toHaveCount(0);
  });

  test('the API refusing the save shows the no-access message and keeps the input', async ({
    page,
  }) => {
    await signInAs(page, 'ADMIN', {
      [getKey('kasun')]: ok(detail('kasun')),
      [patchKey('kasun')]: fail(403, 'FORBIDDEN'),
    });
    await page.goto(editUrl('kasun'));

    await page.getByLabel('Full name').fill('Kasun S. Silva');
    await save(page).click();

    await expect(
      page.getByText('You do not have access to this'),
    ).toBeVisible();
    await expect(page.getByLabel('Full name')).toHaveValue('Kasun S. Silva');
  });

  test('anyone who is not an admin sees the 403 page and nothing is requested', async ({
    page,
  }) => {
    const { calls } = await signInAs(page, 'DISPATCHER');
    await page.goto(editUrl('kasun'));

    await expect(
      page.getByRole('heading', { name: 'You do not have access' }),
    ).toBeVisible();
    await expect(page.getByLabel('Full name')).toHaveCount(0);
    expect(calls.some((c) => c.key === getKey('kasun'))).toBe(false);
  });

  test('someone who is signed out is sent to log in', async ({ page }) => {
    const { calls } = await signInAs(page, 'ADMIN', { ...signedOut });
    await page.goto(editUrl('kasun'));

    await expect(page).toHaveURL(/\/login/);
    expect(calls.some((c) => c.key === patchKey('kasun'))).toBe(false);
  });

  test('a rate limit asks the person to wait and keeps what was typed', async ({
    page,
  }) => {
    await signInAs(page, 'ADMIN', {
      [getKey('kasun')]: ok(detail('kasun')),
      [patchKey('kasun')]: fail(429, 'RATE_LIMITED', {
        headers: { 'Retry-After': '30' },
      }),
    });
    await page.goto(editUrl('kasun'));

    await page.getByLabel('Full name').fill('Kasun S. Silva');
    await save(page).click();

    await expect(
      page.getByRole('status').filter({ hasText: 'Too many attempts' }),
    ).toBeVisible();
    await expect(save(page)).toBeDisabled();
    await expect(page.getByLabel('Full name')).toHaveValue('Kasun S. Silva');
  });

  test('a fault on our side shows a Reference and keeps what was typed', async ({
    page,
  }) => {
    await signInAs(page, 'ADMIN', {
      [getKey('kasun')]: ok(detail('kasun')),
      [patchKey('kasun')]: fail(500, 'INTERNAL'),
    });
    await page.goto(editUrl('kasun'));

    await page.getByLabel('Full name').fill('Kasun S. Silva');
    await save(page).click();

    await expect(page.getByText(`Reference ${REQUEST_ID}`)).toBeVisible();
    await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
    await expect(page.getByLabel('Full name')).toHaveValue('Kasun S. Silva');
    await expect(save(page)).toBeEnabled();
  });

  test('a lost connection while saving says so, with no Reference', async ({
    page,
  }) => {
    await signInAs(page, 'ADMIN', { [getKey('kasun')]: ok(detail('kasun')) });
    await page.route('**/api/v1/admin/users/kasun', (route) =>
      route.request().method() === 'PATCH' ? route.abort() : route.fallback(),
    );
    await page.goto(editUrl('kasun'));

    await page.getByLabel('Full name').fill('Kasun S. Silva');
    await save(page).click();

    await expect(
      page.getByText('We could not reach SwiftDrop', { exact: false }),
    ).toBeVisible();
    await expect(page.getByText('Reference')).toHaveCount(0);
    await expect(page.getByLabel('Full name')).toHaveValue('Kasun S. Silva');
  });

  test('a failed load shows a Reference and can be retried', async ({
    page,
  }) => {
    await signInAs(page, 'ADMIN', { [getKey('kasun')]: fail(500, 'INTERNAL') });
    await page.goto(editUrl('kasun'));

    await expect(page.getByText('Something broke on our side')).toBeVisible();
    await expect(page.getByText(`Reference ${REQUEST_ID}`)).toBeVisible();
    await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);

    await page.route('**/api/v1/admin/users/kasun', (route) =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(ok(detail('kasun')).body),
      }),
    );
    await page.getByRole('button', { name: 'Try again' }).click();
    await expect(page.getByLabel('Full name')).toHaveValue('Kasun Silva');
  });

  test('the button is busy and cannot send twice while saving', async ({
    page,
  }) => {
    const { calls } = await signInAs(page, 'ADMIN', {
      [getKey('kasun')]: ok(detail('kasun')),
      [patchKey('kasun')]: {
        ...ok(detail('kasun', { fullName: 'Kasun S. Silva' })),
        delayMs: 600,
      },
      'GET /admin/users/': emptyList,
    });
    await page.goto(editUrl('kasun'));

    await page.getByLabel('Full name').fill('Kasun S. Silva');
    await save(page).click();
    await expect(page.getByRole('button', { name: 'Saving' })).toBeVisible();
    await page.getByLabel('Full name').press('Enter');

    await expect(page).toHaveURL(/\/admin\/users$/);
    expect(patches(calls, 'kasun')).toHaveLength(1);
  });

  test('Cancel returns to the list without sending anything', async ({
    page,
  }) => {
    const { calls } = await signInAs(page, 'ADMIN', {
      [getKey('kasun')]: ok(detail('kasun')),
      'GET /admin/users/': emptyList,
    });
    await page.goto(editUrl('kasun'));

    await page.getByRole('link', { name: 'Cancel' }).click();

    await expect(page).toHaveURL(/\/admin\/users$/);
    expect(patches(calls, 'kasun')).toHaveLength(0);
  });

  test('on a 375px phone nothing scrolls sideways and the courier fields fit', async ({
    page,
  }) => {
    await page.setViewportSize({ width: 375, height: 800 });
    await signInAs(page, 'ADMIN', {
      [getKey('kasun')]: ok(courier('kasun')),
      'GET /zones': ok([colombo]),
    });
    await page.goto(editUrl('kasun'));
    await expect(page.getByLabel('Home zone')).toBeEnabled();

    const overflow = await page.evaluate(
      'document.documentElement.scrollWidth - document.documentElement.clientWidth',
    );
    expect(overflow).toBe(0);
    const box = await save(page).boundingBox();
    expect(box?.height ?? 0).toBeGreaterThanOrEqual(40);
  });
});
