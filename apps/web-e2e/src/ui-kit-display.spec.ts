import { test, expect, type Page } from '@playwright/test';

import { computed } from './support/style';

test.use({ colorScheme: 'light' });

test.beforeEach(async ({ page }) => {
  await page.goto('/design');
});

test.describe('IconButton', () => {
  const button = (page: Page, name: string | RegExp) =>
    page.getByRole('button', { name });

  test('its label is its accessible name and its tooltip, and the icon is decorative', async ({
    page,
  }) => {
    const actions = button(page, 'Actions for SDK7M2Q9XP4');
    await expect(actions).toHaveAttribute('title', 'Actions for SDK7M2Q9XP4');
    await expect(actions.locator('svg')).toHaveAttribute('aria-hidden', 'true');
  });

  test('it is 44px by default, 36px small and 48px large', async ({ page }) => {
    const medium = await button(page, 'Actions for SDK7M2Q9XP4').boundingBox();
    const small = await button(page, 'Close notice').first().boundingBox();
    const large = await button(page, 'Close notice, large').boundingBox();
    expect([medium!.width, medium!.height]).toEqual([44, 44]);
    expect(small!.width).toBe(36);
    expect(large!.width).toBe(48);
  });

  test('the variants look different and danger is red', async ({ page }) => {
    const ghost = await computed(
      button(page, 'Actions for SDK7M2Q9XP4'),
      'backgroundColor',
    );
    const secondary = await computed(
      button(page, /Copy tracking number/),
      'backgroundColor',
    );
    const danger = await computed(
      button(page, /Revoke key/),
      'backgroundColor',
    );
    expect(new Set([ghost, secondary, danger]).size).toBe(3);
  });

  test('a loading one is busy, still focusable and cannot be clicked through', async ({
    page,
  }) => {
    const loading = button(page, 'Copying');
    await expect(loading).toHaveAttribute('aria-busy', 'true');
    await expect(loading).toHaveAttribute('aria-disabled', 'true');
    await expect(loading).not.toHaveAttribute('disabled');
    await loading.focus();
    await expect(loading).toBeFocused();
  });

  test('a disabled one uses the disabled attribute', async ({ page }) => {
    await expect(button(page, 'Copy is not available')).toBeDisabled();
  });

  test('it shows the focus ring and is square-cornered like a control', async ({
    page,
  }) => {
    const actions = button(page, 'Actions for SDK7M2Q9XP4');
    await actions.focus();
    await expect(actions).toHaveCSS('outline-width', '2px');
    await expect(actions).toHaveCSS('border-radius', '8px');
  });
});

test.describe('KeyValueList', () => {
  const list = (page: Page, text: string) =>
    page.locator('dl').filter({ hasText: text });

  test('it is a description list with a term for every value', async ({
    page,
  }) => {
    const profile = list(page, 'Business name');
    await expect(profile.locator('dt')).toHaveCount(4);
    await expect(profile.locator('dd')).toHaveCount(4);
    await expect(profile.locator('dt').first()).toHaveText('Business name');
    await expect(profile.locator('dd').first()).toHaveText('ShopZone Lanka');
  });

  test('an empty value shows a dash that screen readers read as not provided', async ({
    page,
  }) => {
    const phone = list(page, 'Business name').locator('dd').nth(2);
    await expect(phone.locator('[aria-hidden="true"]')).toHaveText('—');
    await expect(phone.locator('.sr-only')).toHaveText('Not provided');
  });

  test('identifiers are mono', async ({ page }) => {
    const registration = list(page, 'Business name').locator('dd').nth(1);
    await expect(registration).toHaveCSS('font-family', /mono|JetBrains/i);
  });

  test('inline puts the label beside the value from 768px and stacks below', async ({
    page,
  }) => {
    const row = list(page, 'Order').locator('> div').first();

    await page.setViewportSize({ width: 1280, height: 800 });
    const wideTerm = await row.locator('dt').boundingBox();
    const wideValue = await row.locator('dd').boundingBox();
    expect(wideValue!.y).toBeCloseTo(wideTerm!.y, -1);
    expect(wideValue!.x).toBeGreaterThan(wideTerm!.x + wideTerm!.width - 1);

    await page.setViewportSize({ width: 360, height: 800 });
    const narrowTerm = await row.locator('dt').boundingBox();
    const narrowValue = await row.locator('dd').boundingBox();
    expect(narrowValue!.y).toBeGreaterThan(narrowTerm!.y);
  });
});

test.describe('WaybillStub', () => {
  const stub = (page: Page, number: string) =>
    page
      .locator('div.overflow-hidden.rounded-card')
      .filter({ hasText: number });

  test('it shows the tracking number in mono, the status and the two cities', async ({
    page,
  }) => {
    const parcel = stub(page, 'SDK7M2Q9XP4');
    await expect(parcel.getByText('Tracking number')).toBeVisible();
    await expect(parcel.getByText('SDK7M2Q9XP4')).toHaveCSS(
      'font-family',
      /mono|JetBrains/i,
    );
    await expect(parcel.getByText('IN TRANSIT')).toBeVisible();
    await expect(parcel.getByText('Colombo')).toBeVisible();
    await expect(parcel.getByText('Kandy')).toBeVisible();
  });

  test('a public stub drops every private row and says what is shown', async ({
    page,
  }) => {
    const parcel = stub(page, 'SDK7M2Q9XP4');
    await expect(parcel.getByText('Last update')).toBeVisible();
    await expect(parcel.getByText('Recipient')).toHaveCount(0);
    await expect(parcel.getByText('Nimali Perera')).toHaveCount(0);
    await expect(parcel.getByText('LKR 550.00')).toHaveCount(0);
    await expect(
      parcel.getByText(
        'Only the status, the cities and the times are shown here.',
      ),
    ).toBeVisible();
  });

  test('a private stub shows every row', async ({ page }) => {
    const parcel = stub(page, 'SDN5R2C7WB1');
    await expect(parcel.getByText('Recipient')).toBeVisible();
    await expect(parcel.getByText('Nimali Perera')).toBeVisible();
    await expect(parcel.getByText('LKR 550.00')).toBeVisible();
    await expect(parcel.getByText('Only the status')).toHaveCount(0);
  });

  test('the details sit under a dashed tear line on a second surface', async ({
    page,
  }) => {
    const foot = stub(page, 'SDN5R2C7WB1').locator('> div').nth(1);
    await expect(foot).toHaveCSS('border-top-style', 'dashed');
    const top = await computed(stub(page, 'SDN5R2C7WB1'), 'backgroundColor');
    expect(await computed(foot, 'backgroundColor')).not.toBe(top);
  });

  test('when a stub is stretched beside a taller one, its details fill the card', async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1280, height: 900 });
    const card = stub(page, 'SDK7M2Q9XP4');
    const foot = card.locator('> div').nth(1);
    const cardBox = await card.boundingBox();
    const footBox = await foot.boundingBox();
    // The card has a 1px border, so the foot ends within 2px of the card's bottom.
    expect(
      cardBox!.y + cardBox!.height - (footBox!.y + footBox!.height),
    ).toBeLessThan(2);
  });

  test('a compact stub is one row with the number, the cities and the status', async ({
    page,
  }) => {
    const row = page
      .locator('div.rounded-card.p-3')
      .filter({ hasText: 'SDK3H8L1ZT6' });
    await expect(row).toContainText('Colombo to Galle');
    await expect(row).toContainText('OUT FOR DELIVERY');
    const box = await row.boundingBox();
    expect(box!.height).toBeLessThan(90);
  });

  test('loading shows grey blocks with a spoken label, never data', async ({
    page,
  }) => {
    const loading = page
      .getByRole('status')
      .filter({ hasText: 'Loading parcel details' });
    await expect(loading).toHaveAttribute('aria-busy', 'true');
  });
});
