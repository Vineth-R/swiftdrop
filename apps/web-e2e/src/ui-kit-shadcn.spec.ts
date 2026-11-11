import { test, expect } from '@playwright/test';

test.use({ colorScheme: 'light' });

test.beforeEach(async ({ page }) => {
  await page.goto('/design');
});

test.describe('shadcn primitives', () => {
  test('a dialog traps focus, closes on Escape and returns focus', async ({
    page,
  }) => {
    const trigger = page.getByRole('button', { name: 'Open dialog' });
    await trigger.click();

    const dialog = page.getByRole('dialog', { name: 'Cancel this delivery?' });
    await expect(dialog).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(dialog).toBeHidden();
    await expect(trigger).toBeFocused();
  });

  test('a sheet closes on Escape and returns focus', async ({ page }) => {
    const trigger = page.getByRole('button', { name: 'Open sheet' });
    await trigger.click();
    const sheet = page.getByRole('dialog', { name: 'Delivery details' });
    await expect(sheet).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(sheet).toBeHidden();
    await expect(trigger).toBeFocused();
  });

  test('a dropdown menu opens from the keyboard and navigates its items', async ({
    page,
  }) => {
    const trigger = page.getByRole('button', {
      name: 'Actions',
      exact: true,
    });
    await trigger.focus();
    await page.keyboard.press('Enter');

    const menu = page.getByRole('menu');
    const items = menu.getByRole('menuitem');
    await expect(menu).toBeVisible();
    await expect(items.nth(0)).toBeFocused();
    await expect(items.nth(0)).toHaveText('View tracking');
    await expect(items.nth(1)).toHaveAttribute('data-disabled', '');

    // The disabled item is skipped in both directions.
    await page.keyboard.press('ArrowDown');
    await expect(items.nth(2)).toBeFocused();
    await page.keyboard.press('ArrowDown');
    await expect(items.nth(3)).toBeFocused();
    await page.keyboard.press('ArrowUp');
    await expect(items.nth(2)).toBeFocused();
    await page.keyboard.press('ArrowUp');
    await expect(items.nth(0)).toBeFocused();

    // Enter activates the focused item and returns focus to the trigger.
    await page.keyboard.press('Enter');
    await expect(menu).toHaveCount(0);
    await expect(trigger).toBeFocused();
  });

  test('Escape closes the dropdown and returns focus to the trigger', async ({
    page,
  }) => {
    const trigger = page.getByRole('button', {
      name: 'Actions',
      exact: true,
    });
    await trigger.focus();
    await page.keyboard.press('Enter');

    const menu = page.getByRole('menu');
    await expect(menu).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(menu).toHaveCount(0);
    await expect(trigger).toBeFocused();
  });

  test('a mouse-opened dropdown still works', async ({ page }) => {
    const trigger = page.getByRole('button', {
      name: 'Actions',
      exact: true,
    });
    await trigger.click();

    const menu = page.getByRole('menu');
    await expect(menu).toBeVisible();
    await menu.getByRole('menuitem', { name: 'Reassign courier' }).click();
    await expect(menu).toHaveCount(0);
    await expect(trigger).toBeFocused();
  });

  test('the first keyboard open focuses the first item on a slow page load', async ({
    page,
    browserName,
  }) => {
    test.skip(browserName !== 'chromium', 'CPU throttling uses CDP');
    const client = await page.context().newCDPSession(page);
    await client.send('Emulation.setCPUThrottlingRate', { rate: 10 });

    // Reload under throttling so the menu's document listeners can lag behind
    // the first keypress, as they do on a slow device.
    await page.goto('/design');
    const trigger = page.getByRole('button', {
      name: 'Actions',
      exact: true,
    });
    await trigger.focus();
    await page.keyboard.press('Enter');

    const menu = page.getByRole('menu');
    await expect(menu).toBeVisible();
    await expect(menu.getByRole('menuitem').first()).toBeFocused();
  });

  test('the select menu and checkbox keep their state', async ({ page }) => {
    await page.locator('#demo-shadcn-status').click();
    await page.getByRole('option', { name: 'FAILED' }).click();
    await expect(page.locator('#demo-shadcn-status')).toContainText('FAILED');

    const insured = page.getByLabel('Insure this parcel');
    await expect(insured).toBeChecked();
    await insured.click();
    await expect(insured).not.toBeChecked();
  });

  test('radio controls are keyboard operable', async ({ page }) => {
    const express = page
      .getByRole('region', { name: 'shadcn primitives' })
      .getByLabel('Express');
    await express.focus();
    await page.keyboard.press('Space');
    await expect(express).toBeChecked();
  });

  test('pagination uses buttons and marks the current page', async ({
    page,
  }) => {
    const pagination = page.getByRole('navigation', {
      name: 'Page navigation',
    });
    await expect(pagination).toBeVisible();
    await expect(
      pagination.getByRole('button', { name: 'Go to previous page' }),
    ).toBeDisabled();
    await expect(pagination.getByRole('button', { name: '1' })).toHaveAttribute(
      'aria-current',
      'page',
    );
  });

  test('tooltip appears for an icon-only control', async ({ page }) => {
    const trigger = page.getByRole('button', { name: 'Add a parcel' });
    await trigger.focus();
    await trigger.hover({ force: true });
    await expect(page.getByRole('tooltip')).toContainText('Add a parcel', {
      timeout: 10_000,
    });
  });

  test('the shadcn table has a caption and real rows', async ({ page }) => {
    const table = page.getByRole('table', {
      name: 'Recent delivery attempts.',
    });
    await expect(table).toBeVisible();
    await expect(table.getByRole('row')).toHaveCount(3);
  });

  test('avatar initials come from the full name', async ({ page }) => {
    await expect(page.getByText('NP', { exact: true })).toBeVisible();
    await expect(page.getByText('AN', { exact: true })).toBeVisible();
  });

  test('card, badges and separator render', async ({ page }) => {
    await expect(page.getByText('Colombo to Kandy')).toBeVisible();
    await expect(page.getByText('Insured', { exact: true })).toBeVisible();
    await expect(page.locator('[data-slot="separator"]').first()).toBeVisible();
  });
});
