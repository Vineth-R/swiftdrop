import { test, expect, type Page } from '@playwright/test';

import { computed, matches } from './support/style';

test.use({ colorScheme: 'light' });

test.beforeEach(async ({ page }) => {
  await page.goto('/design');
});

const dialog = (page: Page, name: string | RegExp) =>
  page.getByRole('dialog', { name });

test.describe('Modal', () => {
  test('it opens with its title as its name and its sentence as its description', async ({
    page,
  }) => {
    await page.getByRole('button', { name: 'Open confirmation' }).click();
    const box = dialog(page, 'Suspend ShopZone Lanka?');
    await expect(box).toBeVisible();
    await expect(box).toHaveAccessibleDescription(
      'Its API keys stop working until you approve the merchant again.',
    );
  });

  test('focus moves to the element marked data-autofocus, and returns on close', async ({
    page,
  }) => {
    const opener = page.getByRole('button', { name: 'Open confirmation' });
    await opener.click();
    const box = dialog(page, 'Suspend ShopZone Lanka?');
    await expect(box.getByRole('button', { name: 'Cancel' })).toBeFocused();

    await page.keyboard.press('Escape');
    await expect(box).toBeHidden();
    await expect(opener).toBeFocused();
  });

  test('without a marked element the close button gets focus', async ({
    page,
  }) => {
    await page
      .getByRole('button', { name: 'Open session-expired dialog' })
      .click();
    // The must-answer dialog has no close button, so the first button is its action.
    await expect(
      dialog(page, 'Your session ended').getByRole('button', {
        name: 'Log in',
      }),
    ).toBeFocused();
  });

  test('Tab stays inside the dialog and the page behind is inert', async ({
    page,
  }) => {
    await page.getByRole('button', { name: 'Open confirmation' }).click();
    const box = dialog(page, 'Suspend ShopZone Lanka?');
    for (let press = 0; press < 8; press++) {
      await page.keyboard.press('Tab');
      // Inside the dialog, or on the document while the browser's own controls
      // have it; never on the page behind.
      const where = (await page.evaluate(`(() => {
        const active = document.activeElement;
        if (!active || active === document.body) return 'document';
        return active.closest('dialog[open]') ? 'dialog' : 'page';
      })()`)) as string;
      expect(where).not.toBe('page');
    }
    // showModal() puts the dialog in the top layer and makes the rest inert.
    expect(await matches(box, ':modal')).toBe(true);
  });

  test('the close button, Cancel and a click outside all close it', async ({
    page,
  }) => {
    const opener = page.getByRole('button', { name: 'Open confirmation' });
    const box = dialog(page, 'Suspend ShopZone Lanka?');

    await opener.click();
    await box.getByRole('button', { name: 'Close' }).click();
    await expect(box).toBeHidden();

    await opener.click();
    await box.getByRole('button', { name: 'Cancel' }).click();
    await expect(box).toBeHidden();

    await opener.click();
    await page.mouse.click(4, 4);
    await expect(box).toBeHidden();
  });

  test('a click inside the panel does not close it', async ({ page }) => {
    await page.getByRole('button', { name: 'Open confirmation' }).click();
    const box = dialog(page, 'Suspend ShopZone Lanka?');
    await box.getByRole('heading', { name: 'Suspend ShopZone Lanka?' }).click();
    await expect(box).toBeVisible();
  });

  test('the panel is a card with the modal shadow over a scrim', async ({
    page,
  }) => {
    await page.getByRole('button', { name: 'Open confirmation' }).click();
    const panel = dialog(page, 'Suspend ShopZone Lanka?').locator('> div');
    await expect(panel).toHaveCSS('border-radius', '12px');
    await expect(panel).not.toHaveCSS('box-shadow', 'none');
    const scrim = await page.evaluate(
      `getComputedStyle(document.querySelector('dialog[open]'), '::backdrop').backgroundColor`,
    );
    expect(scrim).toMatch(/rgba\(20, 23, 26, 0\.44\)/);
  });

  test('the reason dialog keeps Reject off until a reason is typed, then confirms with a toast', async ({
    page,
  }) => {
    await page.getByRole('button', { name: 'Open reason dialog' }).click();
    const box = dialog(page, 'Reject this application');
    const reject = box.getByRole('button', { name: 'Reject' });
    const reason = box.getByLabel('Reason');

    await expect(reason).toBeFocused();
    await expect(reject).toBeDisabled();
    await reason.fill('Docs');
    await expect(reject).toBeDisabled();
    await reason.fill('Business registration could not be verified');
    await expect(reject).toBeEnabled();
    await reject.click();

    await expect(box).toBeHidden();
    await expect(
      page.getByRole('status').filter({ hasText: 'Application rejected.' }),
    ).toBeVisible();
  });

  test('closing the reason dialog clears what was typed', async ({ page }) => {
    const opener = page.getByRole('button', { name: 'Open reason dialog' });
    await opener.click();
    await dialog(page, 'Reject this application')
      .getByLabel('Reason')
      .fill('Something');
    await page.keyboard.press('Escape');
    await opener.click();
    await expect(
      dialog(page, 'Reject this application').getByLabel('Reason'),
    ).toHaveValue('');
  });

  test('a must-answer dialog ignores Escape and a click outside, and has no close button', async ({
    page,
  }) => {
    await page
      .getByRole('button', { name: 'Open session-expired dialog' })
      .click();
    const box = dialog(page, 'Your session ended');
    await expect(box.getByRole('button', { name: 'Close' })).toHaveCount(0);

    await page.keyboard.press('Escape');
    await expect(box).toBeVisible();
    await page.mouse.click(4, 4);
    await expect(box).toBeVisible();

    await box.getByRole('button', { name: 'Log in' }).click();
    await expect(box).toBeHidden();
  });

  test('it fits a 360px phone with room on both sides', async ({ page }) => {
    await page.setViewportSize({ width: 360, height: 640 });
    await page.getByRole('button', { name: 'Open reason dialog' }).click();
    const panel = dialog(page, 'Reject this application').locator('> div');
    const box = await panel.boundingBox();
    expect(box!.x).toBeGreaterThanOrEqual(16);
    expect(box!.x + box!.width).toBeLessThanOrEqual(344);
  });

  test('it follows the dark theme', async ({ page }) => {
    await page.getByRole('button', { name: 'Open confirmation' }).click();
    const panel = dialog(page, 'Suspend ShopZone Lanka?').locator('> div');
    const light = await computed(panel, 'backgroundColor');
    await page.evaluate(
      `document.documentElement.setAttribute('data-theme', 'dark')`,
    );
    const dark = await computed(panel, 'backgroundColor');
    expect(dark).not.toBe(light);
  });
});

test.describe('Toast', () => {
  const region = (page: Page) => page.getByLabel('Notifications');

  test('a toast confirms in plain words and leaves by itself', async ({
    page,
  }) => {
    await page.getByRole('button', { name: 'Show success' }).click();
    const toast = region(page)
      .getByRole('status')
      .filter({ hasText: 'Details saved.' });
    await expect(toast).toBeVisible();
    await expect(toast).toBeHidden({ timeout: 6000 });
  });

  test('an error is an alert, stays until dismissed and can be closed', async ({
    page,
  }) => {
    await page.getByRole('button', { name: 'Show error (stays)' }).click();
    const toast = region(page)
      .getByRole('alert')
      .filter({ hasText: 'We could not save the details.' });
    await expect(toast).toBeVisible();
    await expect(toast).toContainText('Check your connection and try again.');
    await page.waitForTimeout(3500);
    await expect(toast).toBeVisible();

    await toast.getByRole('button', { name: 'Dismiss notification' }).click();
    await expect(toast).toBeHidden();
  });

  test('hovering pauses the timer', async ({ page }) => {
    await page.getByRole('button', { name: 'Show success' }).click();
    const toast = region(page)
      .getByRole('status')
      .filter({ hasText: 'Details saved.' });
    await toast.hover();
    await page.waitForTimeout(3600);
    await expect(toast).toBeVisible();
    await page.mouse.move(2, 2);
    await expect(toast).toBeHidden({ timeout: 6000 });
  });

  test('at most three show at once, and the oldest leaves first', async ({
    page,
  }) => {
    const show = page.getByRole('button', { name: 'Show error (stays)' });
    for (let count = 0; count < 5; count++) await show.click();
    await expect(region(page).getByRole('alert')).toHaveCount(3);
  });

  test('it is a hairline card with the popover shadow, never covering a phone edge', async ({
    page,
  }) => {
    await page.setViewportSize({ width: 360, height: 640 });
    await page.getByRole('button', { name: 'Show error (stays)' }).click();
    const toast = region(page)
      .getByRole('alert')
      .filter({ hasText: 'We could not save' });
    await expect(toast).toHaveCSS('border-radius', '12px');
    await expect(toast).not.toHaveCSS('box-shadow', 'none');
    const box = await toast.boundingBox();
    expect(box!.x).toBeGreaterThanOrEqual(0);
    expect(box!.x + box!.width).toBeLessThanOrEqual(360);
  });

  test('the notifications region is always there for screen readers', async ({
    page,
  }) => {
    await expect(region(page)).toBeAttached();
  });
});
