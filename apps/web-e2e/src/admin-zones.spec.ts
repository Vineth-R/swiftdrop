import { test, expect } from '@playwright/test';

import {
  fail,
  ok,
  REQUEST_ID,
  SERVER_WORDS,
  type Reply,
} from './support/mock-api';
import { signInAs } from './support/sign-in';

test.use({ colorScheme: 'light' });

const zone = (id: string, changes: Record<string, unknown> = {}) => ({
  id,
  code: id.toUpperCase(),
  name: `Zone ${id}`,
  districts: [`District ${id}`],
  isActive: true,
  ...changes,
});
const colombo = zone('cmb', { name: 'Colombo', districts: ['Colombo'] });
const galle = zone('gle', {
  name: 'Galle',
  districts: ['Galle', 'Matara'],
  isActive: false,
});

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
  body: { data, meta: { requestId: REQUEST_ID, pagination } },
});

const zonesKey = 'GET /admin/zones';

test.describe('admin zones', () => {
  test('lists every zone, active or not, oldest first', async ({ page }) => {
    const { calls } = await signInAs(page, 'ADMIN', {
      [zonesKey]: list([colombo, galle]),
    });
    await page.goto('/admin/zones');

    await expect(
      page.getByRole('heading', { level: 1, name: 'Zones' }),
    ).toBeVisible();
    const row = page.getByRole('row', { name: /Galle/ });
    await expect(row.getByText('GLE')).toHaveClass(/font-mono|^$/);
    await expect(row.getByText('Galle, Matara')).toBeVisible();
    await expect(row.getByText('DEACTIVATED')).toBeVisible();
    await expect(
      page.getByRole('row', { name: /Colombo/ }).getByText('ACTIVE'),
    ).toBeVisible();

    const url = new URL(calls.find((call) => call.key === zonesKey)!.url);
    expect(url.searchParams.get('pageSize')).toBe('20');
    expect(url.searchParams.get('sort')).toBe('createdAt:asc');
  });

  test('shows a loading state, then the list', async ({ page }) => {
    await signInAs(page, 'ADMIN', {
      [zonesKey]: { ...list([colombo]), delayMs: 800 },
    });
    await page.goto('/admin/zones');
    await expect(page.getByText('Loading zones')).toBeVisible();
    await expect(page.getByRole('row', { name: /Colombo/ })).toBeVisible();
  });

  test('an empty list offers to add the first zone', async ({ page }) => {
    await signInAs(page, 'ADMIN', { [zonesKey]: list([]) });
    await page.goto('/admin/zones');
    await expect(
      page.getByRole('heading', { name: 'No zones yet' }),
    ).toBeVisible();
    await page.getByRole('button', { name: 'Add a zone' }).click();
    await expect(
      page.getByRole('dialog', { name: 'Add a zone' }),
    ).toBeVisible();
  });

  test('filters by status and can clear the filter', async ({ page }) => {
    const { calls } = await signInAs(page, 'ADMIN', {
      [zonesKey]: ({ query }) =>
        query.get('isActive') === 'false' ? list([]) : list([colombo, galle]),
    });
    await page.goto('/admin/zones');
    await expect(page.getByRole('row', { name: /Galle/ })).toBeVisible();

    await page.getByLabel('Status').selectOption('DEACTIVATED');
    await expect(
      page.getByRole('heading', { name: 'No zones match' }),
    ).toBeVisible();
    expect(
      calls.some((call) => call.url.includes('isActive=false')),
    ).toBeTruthy();

    await page.getByRole('button', { name: 'Clear the filter' }).click();
    await expect(page.getByRole('row', { name: /Galle/ })).toBeVisible();
  });

  test('pages through the list', async ({ page }) => {
    const pagination = (at: number) => ({
      page: at,
      pageSize: 20,
      totalItems: 21,
      totalPages: 2,
    });
    await signInAs(page, 'ADMIN', {
      [zonesKey]: ({ query }) =>
        query.get('page') === '2'
          ? list([galle], pagination(2))
          : list([colombo], pagination(1)),
    });
    await page.goto('/admin/zones');
    await expect(page.getByRole('row', { name: /Colombo/ })).toBeVisible();
    await page
      .getByRole('navigation', { name: 'Pagination' })
      .getByRole('button', { name: 'Next' })
      .click();
    await expect(page.getByRole('row', { name: /Galle/ })).toBeVisible();
    await expect(page.getByText('Page 2 of 2')).toBeVisible();
  });

  test('adds a zone, sends the districts as a list and says so', async ({
    page,
  }) => {
    const { calls } = await signInAs(page, 'ADMIN', {
      [zonesKey]: list([colombo]),
      'POST /admin/zones': ok(zone('kdy', { name: 'Kandy' }), 201),
    });
    await page.goto('/admin/zones');
    await expect(page.getByRole('row', { name: /Colombo/ })).toBeVisible();
    const listCalls = () =>
      calls.filter((call) => call.key === zonesKey).length;
    const before = listCalls();
    await page.getByRole('button', { name: 'Add zone', exact: true }).click();

    const dialog = page.getByRole('dialog', { name: 'Add a zone' });
    await expect(dialog.getByLabel('Code')).toBeFocused();
    await dialog.getByLabel('Code').fill(' KDY ');
    await dialog.getByLabel('Name').fill('Kandy');
    await dialog.getByLabel('Districts').fill('Kandy, Matale,');
    await dialog.getByRole('button', { name: 'Add zone' }).click();

    await expect(page.getByText('Kandy added.')).toBeVisible();
    await expect(dialog).toBeHidden();
    expect(calls.find((c) => c.key === 'POST /admin/zones')?.body).toEqual({
      code: 'KDY',
      name: 'Kandy',
      districts: ['Kandy', 'Matale'],
    });
    expect(listCalls()).toBeGreaterThan(before);
  });

  test('a missing code or name stops the save and moves focus', async ({
    page,
  }) => {
    const { calls } = await signInAs(page, 'ADMIN', {
      [zonesKey]: list([colombo]),
    });
    await page.goto('/admin/zones');
    await page.getByRole('button', { name: 'Add zone', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: 'Add a zone' });
    await dialog.getByRole('button', { name: 'Add zone' }).click();

    await expect(
      dialog.getByText('Enter a code of up to 20 characters.'),
    ).toBeVisible();
    await expect(
      dialog.getByText('Enter a name of up to 100 characters.'),
    ).toBeVisible();
    await expect(dialog.getByLabel('Code')).toBeFocused();
    expect(calls.some((c) => c.key === 'POST /admin/zones')).toBe(false);
  });

  test('a code in use is refused beside the field, and what was typed stays', async ({
    page,
  }) => {
    await signInAs(page, 'ADMIN', {
      [zonesKey]: list([colombo]),
      'POST /admin/zones': fail(409, 'ZONE_CODE_EXISTS'),
    });
    await page.goto('/admin/zones');
    await page.getByRole('button', { name: 'Add zone', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: 'Add a zone' });
    await dialog.getByLabel('Code').fill('CMB');
    await dialog.getByLabel('Name').fill('Colombo two');
    await dialog.getByRole('button', { name: 'Add zone' }).click();

    await expect(
      dialog.getByText(
        'This code is already used by another zone. Choose a different code.',
      ),
    ).toBeVisible();
    await expect(dialog.getByLabel('Name')).toHaveValue('Colombo two');
    await expect(dialog.getByLabel('Code')).toBeFocused();
    await expect(page.locator('body')).not.toContainText(SERVER_WORDS);
  });

  test('a 422 is shown in our words beside the field', async ({ page }) => {
    await signInAs(page, 'ADMIN', {
      [zonesKey]: list([colombo]),
      'POST /admin/zones': fail(422, 'VALIDATION_ERROR', {
        details: [{ field: 'name', issue: SERVER_WORDS }],
      }),
    });
    await page.goto('/admin/zones');
    await page.getByRole('button', { name: 'Add zone', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: 'Add a zone' });
    await dialog.getByLabel('Code').fill('KDY');
    await dialog.getByLabel('Name').fill('Kandy');
    await dialog.getByRole('button', { name: 'Add zone' }).click();

    await expect(
      dialog.getByText('Enter a name of up to 100 characters.'),
    ).toBeVisible();
    await expect(page.locator('body')).not.toContainText(SERVER_WORDS);
  });

  test('a server fault while saving keeps the form and shows a reference', async ({
    page,
  }) => {
    await signInAs(page, 'ADMIN', {
      [zonesKey]: list([colombo]),
      'POST /admin/zones': fail(500, 'INTERNAL_ERROR'),
    });
    await page.goto('/admin/zones');
    await page.getByRole('button', { name: 'Add zone', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: 'Add a zone' });
    await dialog.getByLabel('Code').fill('KDY');
    await dialog.getByLabel('Name').fill('Kandy');
    await dialog.getByRole('button', { name: 'Add zone' }).click();

    await expect(dialog.getByText('Something broke on our side')).toBeVisible();
    await expect(dialog.getByText(REQUEST_ID)).toBeVisible();
    await expect(dialog.getByLabel('Code')).toHaveValue('KDY');
  });

  test('edits only what changed, and the code cannot change', async ({
    page,
  }) => {
    const { calls } = await signInAs(page, 'ADMIN', {
      [zonesKey]: list([galle]),
      'PATCH /admin/zones/gle': ok({ ...galle, name: 'Galle Fort' }),
    });
    await page.goto('/admin/zones');
    await page.getByRole('button', { name: 'Edit Galle' }).click();

    const dialog = page.getByRole('dialog', { name: 'Edit Galle' });
    await expect(dialog.getByLabel('Code')).toHaveAttribute('readonly', '');
    const save = dialog.getByRole('button', { name: 'Save zone' });
    await expect(save).toBeDisabled();

    await dialog.getByLabel('Name').fill('Galle Fort');
    await save.click();

    await expect(page.getByText('Galle Fort saved.')).toBeVisible();
    expect(calls.find((c) => c.key === 'PATCH /admin/zones/gle')?.body).toEqual(
      { name: 'Galle Fort' },
    );
  });

  test('deactivating asks first, and focus starts on the safe button', async ({
    page,
  }) => {
    const { calls } = await signInAs(page, 'ADMIN', {
      [zonesKey]: list([colombo]),
      'PATCH /admin/zones/cmb': ok({ ...colombo, isActive: false }),
    });
    await page.goto('/admin/zones');
    await page.getByRole('button', { name: 'Deactivate Colombo' }).click();

    const dialog = page.getByRole('dialog', { name: 'Deactivate Colombo?' });
    await expect(
      dialog.getByRole('button', { name: 'Keep active' }),
    ).toBeFocused();
    await dialog.getByRole('button', { name: 'Keep active' }).click();
    await expect(dialog).toBeHidden();
    expect(calls.some((c) => c.key === 'PATCH /admin/zones/cmb')).toBe(false);

    await page.getByRole('button', { name: 'Deactivate Colombo' }).click();
    await dialog.getByRole('button', { name: 'Deactivate' }).click();
    await expect(page.getByText('Colombo is deactivated.')).toBeVisible();
    expect(calls.find((c) => c.key === 'PATCH /admin/zones/cmb')?.body).toEqual(
      { isActive: false },
    );
  });

  test('activating happens straight away and says so', async ({ page }) => {
    const { calls } = await signInAs(page, 'ADMIN', {
      [zonesKey]: list([galle]),
      'PATCH /admin/zones/gle': ok({ ...galle, isActive: true }),
    });
    await page.goto('/admin/zones');
    await page.getByRole('button', { name: 'Activate Galle' }).click();
    await expect(page.getByText('Galle is active again.')).toBeVisible();
    await expect(page.getByRole('dialog')).toHaveCount(0);
    expect(calls.find((c) => c.key === 'PATCH /admin/zones/gle')?.body).toEqual(
      { isActive: true },
    );
  });

  test('a failed list shows why, and Try again works', async ({ page }) => {
    let isDown = true;
    await signInAs(page, 'ADMIN', {
      [zonesKey]: () =>
        isDown ? fail(500, 'INTERNAL_ERROR') : list([colombo]),
    });
    await page.goto('/admin/zones');
    await expect(page.getByText('Something broke on our side')).toBeVisible();
    await expect(page.getByText(REQUEST_ID)).toBeVisible();
    await expect(page.locator('body')).not.toContainText(SERVER_WORDS);

    isDown = false;
    await page.getByRole('button', { name: 'Try again' }).click();
    await expect(page.getByRole('row', { name: /Colombo/ })).toBeVisible();
  });

  test('too many requests says how long to wait', async ({ page }) => {
    await signInAs(page, 'ADMIN', {
      [zonesKey]: fail(429, 'RATE_LIMITED', {
        headers: { 'Retry-After': '30' },
      }),
    });
    await page.goto('/admin/zones');
    await expect(
      page.getByText('Too many attempts from this device'),
    ).toBeVisible();
  });

  test('a 403 from the API shows the no-permission message', async ({
    page,
  }) => {
    await signInAs(page, 'ADMIN', { [zonesKey]: fail(403, 'FORBIDDEN') });
    await page.goto('/admin/zones');
    await expect(
      page.getByText('You do not have access to this'),
    ).toBeVisible();
    await expect(page.getByText(REQUEST_ID)).toHaveCount(0);
  });

  for (const role of ['DISPATCHER', 'CUSTOMER'] as const) {
    test(`a ${role} sees the 403 page and no request is made`, async ({
      page,
    }) => {
      const { calls } = await signInAs(page, role);
      await page.goto('/admin/zones');
      await expect(
        page.getByRole('heading', { name: 'You do not have access' }),
      ).toBeVisible();
      expect(calls.some((call) => call.key === zonesKey)).toBe(false);
    });
  }

  test('there is one primary button, and it never scrolls sideways on a phone', async ({
    page,
  }) => {
    await page.setViewportSize({ width: 375, height: 800 });
    await signInAs(page, 'ADMIN', { [zonesKey]: list([colombo, galle]) });
    await page.goto('/admin/zones');
    await expect(
      page.locator('main :text("Colombo"):visible').first(),
    ).toBeVisible();

    await expect(
      page.locator('main [data-variant="primary"]:visible'),
    ).toHaveCount(1);
    const overflow = await page.evaluate(
      'document.documentElement.scrollWidth - document.documentElement.clientWidth',
    );
    expect(overflow).toBe(0);
    const box = await page
      .getByRole('button', { name: 'Edit Colombo' })
      .boundingBox();
    expect(box?.height).toBeGreaterThanOrEqual(44);
  });

  test('the form works with the keyboard and hands focus back', async ({
    page,
  }) => {
    await signInAs(page, 'ADMIN', { [zonesKey]: list([colombo]) });
    await page.goto('/admin/zones');
    const add = page.getByRole('button', { name: 'Add zone', exact: true });
    await add.focus();
    await page.keyboard.press('Enter');
    const dialog = page.getByRole('dialog', { name: 'Add a zone' });
    await expect(dialog.getByLabel('Code')).toBeFocused();
    await page.keyboard.press('Tab');
    await expect(dialog.getByLabel('Name')).toBeFocused();
    await page.keyboard.press('Escape');
    await expect(dialog).toBeHidden();
    await expect(add).toBeFocused();
  });
});
