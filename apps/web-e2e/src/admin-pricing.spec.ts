import { test, expect, type Page } from '@playwright/test';

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

const zone = (id: string, name: string, isActive = true) => ({
  id,
  code: id.toUpperCase(),
  name,
  districts: [name],
  isActive,
});
const colombo = zone('cmb', 'Colombo');
const kandy = zone('kdy', 'Kandy');
const jaffna = zone('jfn', 'Jaffna', false);

const rule = {
  id: 'r1',
  originZoneId: 'cmb',
  destinationZoneId: 'kdy',
  baseFee: 450,
  currency: 'LKR',
  isActive: true,
  updatedAt: '2026-10-20T08:00:00Z',
};
const inactiveRule = {
  ...rule,
  id: 'r2',
  destinationZoneId: 'jfn',
  isActive: false,
};
const band = {
  id: 'b1',
  minWeightKg: 1,
  maxWeightKg: 5,
  charge: 100,
  isActive: true,
};
const surcharges = [
  { sizeCategory: 'SMALL', surcharge: 0 },
  { sizeCategory: 'MEDIUM', surcharge: 150 },
  { sizeCategory: 'LARGE', surcharge: 400 },
  { sizeCategory: 'OVERSIZED', surcharge: 900 },
];

const list = (data: unknown[]): Reply => ({
  status: 200,
  body: {
    data,
    meta: {
      requestId: REQUEST_ID,
      pagination: {
        page: 1,
        pageSize: 20,
        totalItems: data.length,
        totalPages: 1,
      },
    },
  },
});

const handlers = (extra: Record<string, Handler> = {}) => ({
  'GET /admin/zones': list([colombo, kandy, jaffna]),
  'GET /zones': ok([colombo, kandy]),
  'GET /admin/pricing/rules': list([rule, inactiveRule]),
  'GET /admin/pricing/bands': list([band]),
  'GET /admin/pricing/surcharges': ok(surcharges),
  ...extra,
});

async function open(page: Page, extra: Record<string, Handler> = {}) {
  const mock = await signInAs(page, 'ADMIN', handlers(extra));
  await page.goto('/admin/pricing');
  return mock;
}

test.describe('admin pricing', () => {
  test('stacks rules, weight bands and size surcharges, with one primary button', async ({
    page,
  }) => {
    await open(page);
    await expect(
      page.getByRole('heading', { level: 1, name: 'Pricing' }),
    ).toBeVisible();
    await expect(page.getByRole('heading', { level: 2 })).toHaveText([
      'Pricing rules',
      'Weight bands',
      'Size surcharges',
    ]);
    await expect(
      page.locator('main [data-variant="primary"]:visible'),
    ).toHaveText('Add rule');
  });

  test('the sections load on their own, so one failing leaves the others', async ({
    page,
  }) => {
    await open(page, {
      'GET /admin/pricing/bands': fail(500, 'INTERNAL_ERROR'),
    });
    await expect(page.getByText('Something broke on our side')).toBeVisible();
    await expect(page.getByRole('row', { name: /Medium/ })).toBeVisible();
    await expect(
      page.getByRole('row', { name: /Colombo.*Kandy/ }),
    ).toBeVisible();
  });

  test('a CUSTOMER sees the 403 page and no request is made', async ({
    page,
  }) => {
    const { calls } = await signInAs(page, 'CUSTOMER');
    await page.goto('/admin/pricing');
    await expect(
      page.getByRole('heading', { name: 'You do not have access' }),
    ).toBeVisible();
    expect(calls.some((c) => c.key.includes('/admin/pricing'))).toBe(false);
  });

  test.describe('pricing rules', () => {
    test('shows zones by name, never by id, including inactive zones', async ({
      page,
    }) => {
      await open(page);
      const row = page.getByRole('row', { name: /Colombo.*Jaffna/ });
      await expect(row.getByText('LKR 450.00')).toBeVisible();
      await expect(row.getByText('DEACTIVATED')).toBeVisible();
      await expect(
        page.getByRole('table', { name: 'Pricing rules' }),
      ).not.toContainText('jfn');
    });

    test('filters by zone and status', async ({ page }) => {
      const { calls } = await open(page);
      await expect(
        page.getByRole('row', { name: /Colombo.*Kandy/ }),
      ).toBeVisible();
      await page.locator('#rule-origin-filter').selectOption('cmb');
      await page.locator('#rule-status-filter').selectOption('DEACTIVATED');
      await expect
        .poll(() =>
          calls.some(
            (c) =>
              c.url.includes('originZoneId=cmb') &&
              c.url.includes('isActive=false'),
          ),
        )
        .toBe(true);
    });

    test('an empty list offers a rule; filters offer to clear', async ({
      page,
    }) => {
      await open(page, { 'GET /admin/pricing/rules': list([]) });
      await expect(
        page.getByRole('heading', { name: 'No pricing rules yet' }),
      ).toBeVisible();
      await page.locator('#rule-status-filter').selectOption('ACTIVE');
      await expect(
        page.getByRole('heading', { name: 'No rules match' }),
      ).toBeVisible();
      await page.getByRole('button', { name: 'Clear the filters' }).click();
      await expect(
        page.getByRole('heading', { name: 'No pricing rules yet' }),
      ).toBeVisible();
    });

    test('adds a rule, offering active zones only', async ({ page }) => {
      const { calls } = await open(page, {
        'POST /admin/pricing/rules': ok(rule, 201),
      });
      await page.getByRole('button', { name: 'Add rule', exact: true }).click();
      const dialog = page.getByRole('dialog', { name: 'Add a rule' });
      await expect(
        dialog.getByLabel('From').getByRole('option', { name: 'Jaffna' }),
      ).toHaveCount(0);

      await dialog.getByLabel('From').selectOption('cmb');
      await dialog.getByLabel('To').selectOption('kdy');
      await dialog.getByLabel('Base fee').fill('450');
      await dialog.getByRole('button', { name: 'Add rule' }).click();

      await expect(
        page.getByText('Rule added for Colombo to Kandy.'),
      ).toBeVisible();
      expect(
        calls.find((c) => c.key === 'POST /admin/pricing/rules')?.body,
      ).toEqual({
        originZoneId: 'cmb',
        destinationZoneId: 'kdy',
        baseFee: 450,
      });
    });

    for (const [typed, reason] of [
      ['0', 'zero'],
      ['-5', 'negative'],
      ['1.234', 'three decimals'],
      ['abc', 'not a number'],
    ]) {
      test(`a base fee of ${typed} is refused (${reason})`, async ({
        page,
      }) => {
        const { calls } = await open(page);
        await page
          .getByRole('button', { name: 'Add rule', exact: true })
          .click();
        const dialog = page.getByRole('dialog', { name: 'Add a rule' });
        await dialog.getByLabel('From').selectOption('cmb');
        await dialog.getByLabel('To').selectOption('kdy');
        await dialog.getByLabel('Base fee').fill(typed);
        await dialog.getByRole('button', { name: 'Add rule' }).click();
        await expect(
          dialog.getByText('Enter an amount above 0, with up to two decimals.'),
        ).toBeVisible();
        await expect(dialog.getByLabel('Base fee')).toBeFocused();
        expect(calls.some((c) => c.key === 'POST /admin/pricing/rules')).toBe(
          false,
        );
      });
    }

    test('a route that already has a price is refused beside the field', async ({
      page,
    }) => {
      await open(page, {
        'POST /admin/pricing/rules': fail(409, 'RULE_EXISTS'),
      });
      await page.getByRole('button', { name: 'Add rule', exact: true }).click();
      const dialog = page.getByRole('dialog', { name: 'Add a rule' });
      await dialog.getByLabel('From').selectOption('cmb');
      await dialog.getByLabel('To').selectOption('kdy');
      await dialog.getByLabel('Base fee').fill('450');
      await dialog.getByRole('button', { name: 'Add rule' }).click();
      await expect(
        dialog.getByText(
          'This route already has an active price. Edit that price instead.',
        ),
      ).toBeVisible();
      await expect(dialog.getByLabel('Base fee')).toHaveValue('450');
      await expect(page.locator('body')).not.toContainText(SERVER_WORDS);
    });

    test('a zone that has gone says so inside the dialog', async ({ page }) => {
      await open(page, {
        'POST /admin/pricing/rules': fail(404, 'ZONE_NOT_FOUND'),
      });
      await page.getByRole('button', { name: 'Add rule', exact: true }).click();
      const dialog = page.getByRole('dialog', { name: 'Add a rule' });
      await dialog.getByLabel('From').selectOption('cmb');
      await dialog.getByLabel('To').selectOption('kdy');
      await dialog.getByLabel('Base fee').fill('450');
      await dialog.getByRole('button', { name: 'Add rule' }).click();
      await expect(
        dialog.getByText('One of these zones no longer exists'),
      ).toBeVisible();
    });

    test('editing says stored parcels keep their fee, and sends only the fee', async ({
      page,
    }) => {
      const { calls } = await open(page, {
        'PATCH /admin/pricing/rules/r1': ok({ ...rule, baseFee: 500 }),
      });
      await page.getByRole('button', { name: 'Edit Colombo to Kandy' }).click();
      const dialog = page.getByRole('dialog', {
        name: 'Edit Colombo to Kandy',
      });
      await expect(
        dialog.getByText(
          'Editing a rule never changes the fee stored on existing parcels.',
        ),
      ).toBeVisible();
      await expect(dialog.getByLabel('Base fee')).toHaveValue('450.00');
      await expect(dialog.getByRole('combobox')).toHaveCount(0);
      const save = dialog.getByRole('button', { name: 'Save rule' });
      await expect(save).toBeDisabled();

      await dialog.getByLabel('Base fee').fill('500');
      await save.click();
      await expect(page.getByText('Colombo to Kandy saved.')).toBeVisible();
      expect(
        calls.find((c) => c.key === 'PATCH /admin/pricing/rules/r1')?.body,
      ).toEqual({ baseFee: 500 });
    });

    test('deactivating asks first; activating does not', async ({ page }) => {
      const { calls } = await open(page, {
        'PATCH /admin/pricing/rules/r1': ok({ ...rule, isActive: false }),
        'PATCH /admin/pricing/rules/r2': ok({
          ...inactiveRule,
          isActive: true,
        }),
      });
      await page
        .getByRole('button', { name: 'Deactivate Colombo to Kandy' })
        .click();
      const dialog = page.getByRole('dialog', {
        name: 'Deactivate Colombo to Kandy?',
      });
      await dialog.getByRole('button', { name: 'Deactivate' }).click();
      await expect(
        page.getByText('Colombo to Kandy is deactivated.'),
      ).toBeVisible();

      await page
        .getByRole('button', { name: 'Activate Colombo to Jaffna' })
        .click();
      await expect(
        page.getByText('Colombo to Jaffna is active again.'),
      ).toBeVisible();
      expect(
        calls.find((c) => c.key === 'PATCH /admin/pricing/rules/r2')?.body,
      ).toEqual({ isActive: true });
    });

    test('activating a rule whose route is priced says what to do', async ({
      page,
    }) => {
      await open(page, {
        'PATCH /admin/pricing/rules/r2': fail(409, 'RULE_EXISTS'),
      });
      await page
        .getByRole('button', { name: 'Activate Colombo to Jaffna' })
        .click();
      await expect(
        page.getByText('This route already has an active price'),
      ).toBeVisible();
      await expect(page.getByText('Deactivate that rule first.')).toBeVisible();
    });
  });

  test.describe('weight bands', () => {
    test('shows weights and charges in mono, with units', async ({ page }) => {
      await open(page);
      const row = page.getByRole('row', { name: /1\.00 kg/ });
      await expect(row.getByText('Over 1.00 kg')).toBeVisible();
      await expect(row.getByText('5.00 kg')).toBeVisible();
      await expect(row.getByText('LKR 100.00')).toBeVisible();
    });

    test('asks for the oldest first, a page at a time', async ({ page }) => {
      const { calls } = await open(page);
      await expect(page.getByRole('row', { name: /1\.00 kg/ })).toBeVisible();
      const url = new URL(
        calls.find((c) => c.key === 'GET /admin/pricing/bands')!.url,
      );
      expect(url.searchParams.get('sort')).toBe('createdAt:asc');
      expect(url.searchParams.get('pageSize')).toBe('20');
    });

    test('adds a band with a charge of 0, which is allowed', async ({
      page,
    }) => {
      const { calls } = await open(page, {
        'POST /admin/pricing/bands': ok(
          { ...band, id: 'b2', minWeightKg: 5, maxWeightKg: 10, charge: 0 },
          201,
        ),
      });
      await page.getByRole('button', { name: 'Add band' }).first().click();
      const dialog = page.getByRole('dialog', { name: 'Add a weight band' });
      await dialog.getByLabel('Over').fill('5');
      await dialog.getByLabel('Up to').fill('10');
      await dialog.getByLabel('Charge').fill('0');
      await dialog.getByRole('button', { name: 'Add band' }).click();
      await expect(
        page.getByText('The 5.00 kg to 10.00 kg band is added.'),
      ).toBeVisible();
      expect(
        calls.find((c) => c.key === 'POST /admin/pricing/bands')?.body,
      ).toEqual({ minWeightKg: 5, maxWeightKg: 10, charge: 0 });
    });

    test('the top of a band must be above its bottom', async ({ page }) => {
      await open(page);
      await page.getByRole('button', { name: 'Add band' }).first().click();
      const dialog = page.getByRole('dialog', { name: 'Add a weight band' });
      await dialog.getByLabel('Over').fill('5');
      await dialog.getByLabel('Up to').fill('5');
      await dialog.getByLabel('Charge').fill('10');
      await dialog.getByRole('button', { name: 'Add band' }).click();
      await expect(
        dialog.getByText('Enter a weight above the "over" weight.'),
      ).toBeVisible();
      await expect(dialog.getByLabel('Up to')).toBeFocused();
    });

    test('an overlap names the band on screen, as a plain sentence', async ({
      page,
    }) => {
      await open(page, {
        'POST /admin/pricing/bands': fail(409, 'BAND_OVERLAP'),
      });
      await page.getByRole('button', { name: 'Add band' }).first().click();
      const dialog = page.getByRole('dialog', { name: 'Add a weight band' });
      await dialog.getByLabel('Over').fill('4');
      await dialog.getByLabel('Up to').fill('6');
      await dialog.getByLabel('Charge').fill('10');
      await dialog.getByRole('button', { name: 'Add band' }).click();
      await expect(
        dialog.getByText(
          'This band overlaps 1.00 kg to 5.00 kg. Change the weights so they do not overlap.',
        ),
      ).toBeVisible();
      await expect(page.locator('body')).not.toContainText(SERVER_WORDS);
    });

    test('an overlap with a band not on screen still reads plainly', async ({
      page,
    }) => {
      await open(page, {
        'POST /admin/pricing/bands': fail(409, 'BAND_OVERLAP'),
      });
      await page.getByRole('button', { name: 'Add band' }).first().click();
      const dialog = page.getByRole('dialog', { name: 'Add a weight band' });
      await dialog.getByLabel('Over').fill('20');
      await dialog.getByLabel('Up to').fill('30');
      await dialog.getByLabel('Charge').fill('10');
      await dialog.getByRole('button', { name: 'Add band' }).click();
      await expect(
        dialog.getByText('This band overlaps another active band.'),
      ).toBeVisible();
    });

    test('edits only what changed', async ({ page }) => {
      const { calls } = await open(page, {
        'PATCH /admin/pricing/bands/b1': ok({ ...band, charge: 120 }),
      });
      await page
        .getByRole('button', { name: 'Edit the 1.00 kg to 5.00 kg band' })
        .click();
      const dialog = page.getByRole('dialog', { name: 'Edit weight band' });
      await dialog.getByLabel('Charge').fill('120');
      await dialog.getByRole('button', { name: 'Save band' }).click();
      await expect(
        page.getByText('The 1.00 kg to 5.00 kg band is saved.'),
      ).toBeVisible();
      expect(
        calls.find((c) => c.key === 'PATCH /admin/pricing/bands/b1')?.body,
      ).toEqual({ charge: 120 });
    });
  });

  test.describe('size surcharges', () => {
    test('shows the four sizes', async ({ page }) => {
      await open(page);
      const table = page.getByRole('table', { name: 'Size surcharges' });
      await expect(table.getByRole('row')).toHaveCount(5);
      await expect(
        table.getByRole('row', { name: /Small/ }).getByText('LKR 0.00'),
      ).toBeVisible();
    });

    test('saves one size at a time with PUT, and 0 is allowed', async ({
      page,
    }) => {
      const { calls } = await open(page, {
        'PUT /admin/pricing/surcharges/MEDIUM': ok({
          sizeCategory: 'MEDIUM',
          surcharge: 0,
        }),
      });
      await page
        .getByRole('button', { name: 'Edit the medium surcharge' })
        .click();
      const dialog = page.getByRole('dialog', {
        name: 'Edit the medium surcharge',
      });
      await dialog.getByLabel('Surcharge').fill('0');
      await dialog.getByRole('button', { name: 'Save surcharge' }).click();
      await expect(page.getByText('Medium surcharge saved.')).toBeVisible();
      await expect(
        page
          .getByRole('table', { name: 'Size surcharges' })
          .getByRole('row', { name: /Medium/ })
          .getByText('LKR 0.00'),
      ).toBeVisible();
      expect(
        calls.find((c) => c.key === 'PUT /admin/pricing/surcharges/MEDIUM')
          ?.body,
      ).toEqual({ surcharge: 0 });
    });

    test('a bad amount is refused beside the field', async ({ page }) => {
      await open(page);
      await page
        .getByRole('button', { name: 'Edit the large surcharge' })
        .click();
      const dialog = page.getByRole('dialog', {
        name: 'Edit the large surcharge',
      });
      await dialog.getByLabel('Surcharge').fill('-1');
      await dialog.getByRole('button', { name: 'Save surcharge' }).click();
      await expect(
        dialog.getByText(
          'Enter an amount of 0 or more, with up to two decimals.',
        ),
      ).toBeVisible();
    });
  });

  test('on a phone, a weight band card still says what its title means', async ({
    page,
  }) => {
    await page.setViewportSize({ width: 375, height: 800 });
    await open(page);
    await expect(
      page.locator('main :text("Over 1.00 kg"):visible'),
    ).toHaveCount(1);
  });

  test('nothing scrolls sideways at 375px, and row buttons are big enough', async ({
    page,
  }) => {
    await page.setViewportSize({ width: 375, height: 800 });
    await open(page);
    await expect(
      page.locator('main :text("LKR 450.00"):visible').first(),
    ).toBeVisible();
    const overflow = await page.evaluate(
      'document.documentElement.scrollWidth - document.documentElement.clientWidth',
    );
    expect(overflow).toBe(0);
    for (const name of [
      'Edit Colombo to Kandy',
      'Edit the 1.00 kg to 5.00 kg band',
      'Edit the small surcharge',
    ]) {
      const box = await page.getByRole('button', { name }).boundingBox();
      expect(box?.height, name).toBeGreaterThanOrEqual(44);
    }
  });
});
