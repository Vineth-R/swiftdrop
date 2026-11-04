import { test, expect } from '@playwright/test';

import { computed } from './support/style';

test.use({ colorScheme: 'light' });

test.beforeEach(async ({ page }) => {
  await page.goto('/design');
});

test.describe('StatusChip', () => {
  test('a parcel status is its name in capitals with spaces', async ({
    page,
  }) => {
    for (const label of [
      'CREATED',
      'PICKED UP',
      'IN TRANSIT',
      'OUT FOR DELIVERY',
      'DELIVERED',
      'FAILED',
    ]) {
      await expect(
        page.getByText(label, { exact: true }).first(),
      ).toBeVisible();
    }
  });

  test('every chip has a shape as well as colour and a name', async ({
    page,
  }) => {
    const chip = page
      .locator('span.rounded-chip')
      .filter({ hasText: /^OUT FOR DELIVERY$/ })
      .first();
    await expect(chip.locator('svg')).toHaveAttribute('aria-hidden', 'true');
    await expect(chip).toHaveCSS('border-radius', '6px');
    await expect(chip).toHaveCSS('height', '24px');
  });

  test('statuses are told apart by colour and by shape', async ({ page }) => {
    const chips = ['DELIVERED', 'FAILED', 'IN TRANSIT'].map((label) =>
      page
        .locator('span.rounded-chip')
        .filter({ hasText: new RegExp(`^${label}$`) })
        .first(),
    );
    const colours = await Promise.all(
      chips.map((chip) => computed(chip, 'color')),
    );
    expect(new Set(colours).size).toBe(3);

    const shapes = await Promise.all(
      chips.map((chip) => chip.locator('svg').innerHTML()),
    );
    expect(new Set(shapes).size).toBe(3);
  });

  test('the compact chip keeps the name as a tooltip and for screen readers', async ({
    page,
  }) => {
    const compact = page.locator('span[title="DELIVERED"]');
    await expect(compact).toHaveCount(1);
    await expect(compact.locator('.sr-only')).toHaveText('DELIVERED');
    await expect(compact).toHaveCSS('width', '24px');
  });

  test('merchant and account states use the same chip', async ({ page }) => {
    for (const label of [
      'PENDING',
      'APPROVED',
      'REJECTED',
      'SUSPENDED',
      'ACTIVE',
      'DEACTIVATED',
    ]) {
      await expect(
        page.getByText(label, { exact: true }).first(),
      ).toBeVisible();
    }
  });

  test('the colours follow the dark theme', async ({ page }) => {
    const chip = page
      .locator('span.rounded-chip')
      .filter({ hasText: /^DELIVERED$/ })
      .first();
    const light = await computed(chip, 'backgroundColor');
    await page.evaluate(
      `document.documentElement.setAttribute('data-theme', 'dark')`,
    );
    const dark = await computed(chip, 'backgroundColor');
    expect(dark).not.toBe(light);
  });
});

test.describe('DataTable', () => {
  // A CSS locator, because a hidden table is out of the accessibility tree and
  // the card-list tests need to find it while it is hidden.
  const table = (page: import('@playwright/test').Page) =>
    page.locator('table:has(caption:text-is("Parcels"))');
  // The card list is the table's sibling, so other lists on the page never match.
  const cardList = (page: import('@playwright/test').Page) =>
    table(page).locator('xpath=../following-sibling::ul');

  test('it is a real table, named, with column headers', async ({ page }) => {
    await expect(table(page)).toBeVisible();
    await expect(table(page).getByRole('columnheader')).toHaveCount(5);
    await expect(
      table(page).getByRole('columnheader', { name: 'Fee (LKR)' }),
    ).toBeVisible();
    await expect(table(page).getByRole('row')).toHaveCount(5);
  });

  test('it sits in a hairline box with no shadow', async ({ page }) => {
    const box = table(page).locator('xpath=..');
    await expect(box).toHaveCSS('border-top-width', '1px');
    await expect(box).toHaveCSS('border-radius', '12px');
    await expect(box).toHaveCSS('box-shadow', 'none');
    await expect(box).toHaveCSS('overflow-x', 'auto');
  });

  test('identifiers are mono and amounts are mono, right aligned', async ({
    page,
  }) => {
    const row = table(page).getByRole('row').nth(1);
    await expect(row.getByRole('cell').first()).toHaveCSS(
      'font-family',
      /mono|JetBrains/i,
    );
    const fee = row.getByRole('cell').nth(3);
    await expect(fee).toHaveCSS('text-align', 'right');
    await expect(fee).toHaveCSS('font-family', /mono|JetBrains/i);
    await expect(
      table(page).getByRole('columnheader', { name: 'Fee (LKR)' }),
    ).toHaveCSS('text-align', 'right');
  });

  test('a sortable header sorts, flips and says which way', async ({
    page,
  }) => {
    const header = table(page).getByRole('columnheader', {
      name: 'Tracking number',
    });
    const first = () =>
      table(page).getByRole('row').nth(1).getByRole('cell').first();

    await expect(header).toHaveAttribute('aria-sort', 'ascending');
    await expect(first()).toHaveText('SDK3H8L1ZT6');

    await header.getByRole('button').click();
    await expect(header).toHaveAttribute('aria-sort', 'descending');
    await expect(first()).toHaveText('SDN5R2C7WB1');

    await table(page)
      .getByRole('columnheader', { name: 'Fee (LKR)' })
      .getByRole('button')
      .click();
    await expect(header).not.toHaveAttribute('aria-sort', /.+/);
    await expect(first()).toHaveText('SDK9D4F6QY2');
  });

  test('a column that is not sortable has no button, and an actions column hides its header', async ({
    page,
  }) => {
    await expect(
      table(page)
        .getByRole('columnheader', { name: 'Destination' })
        .getByRole('button'),
    ).toHaveCount(0);
    const open = table(page).getByRole('columnheader', { name: 'Open' });
    await expect(open.locator('.sr-only')).toHaveText('Open');
  });

  test('a row is highlighted on hover', async ({ page }) => {
    const row = table(page).getByRole('row').nth(2);
    const before = await computed(row, 'backgroundColor');
    await row.hover();
    const after = await computed(row, 'backgroundColor');
    expect(after).not.toBe(before);
  });

  test('at 1280px the table shows and the card list is hidden', async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await expect(table(page)).toBeVisible();
    await expect(cardList(page)).toBeHidden();
  });

  test('at 360px the same rows become cards with labelled values and the page does not scroll sideways', async ({
    page,
  }) => {
    await page.setViewportSize({ width: 360, height: 800 });
    await expect(table(page)).toBeHidden();
    await expect(cardList(page).getByRole('listitem')).toHaveCount(4);

    const card = cardList(page).getByRole('listitem').filter({
      hasText: 'SDK7M2Q9XP4',
    });
    await expect(card.getByText('Destination')).toBeVisible();
    await expect(card.getByText('Kandy')).toBeVisible();
    await expect(
      card.getByRole('button', { name: 'View SDK7M2Q9XP4' }),
    ).toBeVisible();
    const overflow = await page.evaluate(
      'document.documentElement.scrollWidth - document.documentElement.clientWidth',
    );
    expect(overflow).toBe(0);
  });

  test('loading shows grey rows with a spoken label, never real data', async ({
    page,
  }) => {
    const loading = page
      .getByRole('status')
      .filter({ hasText: 'Loading merchants' })
      .first();
    await expect(loading).toHaveAttribute('aria-busy', 'true');
  });

  test('empty shows what is missing and what happens next', async ({
    page,
  }) => {
    await expect(
      page.getByRole('heading', { level: 3, name: 'No merchants waiting' }),
    ).toBeVisible();
  });
});

test.describe('TablePagination', () => {
  const nav = (page: import('@playwright/test').Page) =>
    page.getByRole('navigation', { name: 'Pagination' });

  test('it says where you are and how many results there are', async ({
    page,
  }) => {
    await expect(nav(page)).toContainText('Page 2 of 5');
    await expect(nav(page)).toContainText('42 results');
  });

  test('Previous and Next move, and stop at both ends', async ({ page }) => {
    const previous = nav(page).getByRole('button', { name: 'Previous' });
    const next = nav(page).getByRole('button', { name: 'Next' });

    await expect(previous).toBeEnabled();
    await previous.click();
    await expect(nav(page)).toContainText('Page 1 of 5');
    await expect(previous).toBeDisabled();

    for (let step = 0; step < 4; step++) await next.click();
    await expect(nav(page)).toContainText('Page 5 of 5');
    await expect(next).toBeDisabled();
  });
});

test.describe('Timeline', () => {
  const list = (page: import('@playwright/test').Page) =>
    page.getByRole('list', { name: 'Tracking history' });

  test('it is an ordered list with its events in order', async ({ page }) => {
    await expect(list(page).getByRole('listitem')).toHaveCount(5);
    await expect(list(page).getByRole('listitem').first()).toContainText(
      'CREATED',
    );
    await expect(list(page).getByRole('listitem').last()).toContainText(
      'DELIVERED',
    );
  });

  test('the current event is marked for screen readers and stands out', async ({
    page,
  }) => {
    const current = list(page).locator('[aria-current="step"]');
    await expect(current).toHaveCount(1);
    await expect(current).toContainText('IN TRANSIT');
    await expect(current.locator('p').first()).toHaveCSS('font-weight', '600');
  });

  test('times are real time elements shown in Sri Lanka time', async ({
    page,
  }) => {
    const time = list(page).locator('time').nth(2);
    await expect(time).toHaveAttribute('datetime', '2026-11-03T09:25:00Z');
    await expect(time).toHaveText('3 Nov 2026, 14:55');
    await expect(time).toHaveCSS('font-family', /mono|JetBrains/i);
  });

  test('events not reached yet have no time and are dimmed', async ({
    page,
  }) => {
    const upcoming = list(page).getByRole('listitem').nth(3);
    await expect(upcoming.locator('time')).toHaveCount(0);
    const done = list(page).getByRole('listitem').first().locator('p').first();
    const colour = await computed(upcoming.locator('p').first(), 'color');
    const doneColour = await computed(done, 'color');
    expect(colour).not.toBe(doneColour);
  });

  test('it also reads as an approval history', async ({ page }) => {
    const history = page.getByRole('list', {
      name: 'Merchant approval history',
    });
    await expect(history.getByRole('listitem')).toHaveCount(2);
    await expect(history).toContainText('Approved by an administrator');
  });

  test('the dotted line joins the events but does not hang below the last one', async ({
    page,
  }) => {
    const items = list(page).getByRole('listitem');
    await expect(items.first().locator('span.border-dotted')).toHaveCount(1);
    await expect(items.last().locator('span.border-dotted')).toHaveCount(0);
  });
});
