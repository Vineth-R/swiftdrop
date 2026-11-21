import { test, expect } from '@playwright/test';

import { computed } from './support/style';

test.use({ colorScheme: 'light' });

test.beforeEach(async ({ page }) => {
  await page.goto('/design');
});

test.describe('Textarea', () => {
  test('its label, helper and count are wired to it', async ({ page }) => {
    const note = page.locator('#demo-note');
    await expect(note).toHaveAttribute('aria-describedby', 'demo-note-helper');
    await expect(page.locator('#demo-note-helper')).toHaveText(
      'Shown to the courier.',
    );
    await expect(page.getByText('32/200')).toBeVisible();
  });

  test('the count follows what is typed and is mono', async ({ page }) => {
    await page.locator('#demo-note').fill('Hello');
    const count = page.getByText('5/200');
    await expect(count).toBeVisible();
    await expect(count).toHaveCSS('font-family', /mono|JetBrains/i);
  });

  test('it stops at its limit', async ({ page }) => {
    const note = page.locator('#demo-note');
    await note.fill('x'.repeat(260));
    await expect(page.getByText('200/200')).toBeVisible();
  });

  test('an error replaces the helper, marks it required and invalid', async ({
    page,
  }) => {
    const reason = page.locator('#demo-note-error');
    await expect(reason).toHaveAttribute('aria-invalid', 'true');
    await expect(reason).toHaveAttribute('aria-required', 'true');
    await expect(reason).toHaveAttribute(
      'aria-describedby',
      'demo-note-error-error',
    );
    await expect(page.locator('#demo-note-error-error')).toContainText(
      'at least 5 characters',
    );
  });

  test('it is a taller, resizable box with the control look', async ({
    page,
  }) => {
    const note = page.locator('#demo-note');
    const box = await note.boundingBox();
    expect(box?.height).toBeGreaterThanOrEqual(96);
    await expect(note).toHaveCSS('border-radius', '8px');
    await expect(note).toHaveCSS('resize', 'vertical');
  });

  test('a disabled one cannot be edited', async ({ page }) => {
    await expect(page.locator('#demo-note-disabled')).toBeDisabled();
  });
});

test.describe('Select', () => {
  test('it has a real label, a required mark and a helper', async ({
    page,
  }) => {
    const zone = page.locator('#demo-zone');
    await expect(page.locator('label[for="demo-zone"]')).toContainText(
      'Pickup zone',
    );
    await expect(zone).toHaveAttribute('aria-required', 'true');
    await expect(zone).toHaveAttribute('aria-describedby', 'demo-zone-helper');
  });

  test('the placeholder is shown first, dimmed and cannot be picked again', async ({
    page,
  }) => {
    const zone = page.locator('#demo-zone');
    await expect(zone).toHaveValue('');
    await expect(zone.locator('option').first()).toHaveText('Choose a zone');
    await expect(zone.locator('option').first()).toBeDisabled();
    const dimmed = await computed(zone, 'color');

    await zone.selectOption('kdy');
    await expect(zone).toHaveValue('kdy');
    // The control eases its colour, so wait for it instead of reading it at once.
    await expect(zone).not.toHaveCSS('color', dimmed);
  });

  test('an option can be unavailable', async ({ page }) => {
    await expect(page.locator('#demo-zone option[value="jaf"]')).toBeDisabled();
  });

  test('it is reached with the keyboard and shows the focus ring', async ({
    page,
  }) => {
    const zone = page.locator('#demo-zone');
    await zone.focus();
    await expect(zone).toBeFocused();
    await expect(zone).toHaveCSS('outline-style', 'solid');
    await expect(zone).toHaveCSS('outline-width', '2px');
  });

  test('the chevron is decorative and leaves room so text never runs under it', async ({
    page,
  }) => {
    const zone = page.locator('#demo-zone');
    await expect(zone).toHaveCSS('padding-right', '40px');
    await expect(zone).toHaveCSS('appearance', 'none');
    const chevron = zone.locator('xpath=following-sibling::*[name()="svg"]');
    await expect(chevron).toHaveAttribute('aria-hidden', 'true');
    await expect(chevron).toHaveCSS('pointer-events', 'none');
  });

  test('an error is announced through the control', async ({ page }) => {
    const zone = page.locator('#demo-zone-error');
    await expect(zone).toHaveAttribute('aria-invalid', 'true');
    await expect(page.locator('#demo-zone-error-error')).toContainText(
      'Choose a destination zone',
    );
  });

  test('it looks editable: a white control, not the grey of a read-only one', async ({
    page,
  }) => {
    const background = (selector: string) =>
      computed(page.locator(selector), 'backgroundColor');
    const zone = await background('#demo-zone');
    // The disabled select is the grey one; the usable select matches an input's white.
    expect(zone).not.toBe(await background('#demo-zone-disabled'));
    expect(zone).toBe(await background('#demo-note'));
  });

  test('a disabled one cannot be changed', async ({ page }) => {
    await expect(page.locator('#demo-zone-disabled')).toBeDisabled();
  });

  test('it is the same height and radius as a text field', async ({ page }) => {
    const zone = page.locator('#demo-zone');
    await expect(zone).toHaveCSS('height', '40px');
    await expect(zone).toHaveCSS('border-radius', '8px');
  });
});
