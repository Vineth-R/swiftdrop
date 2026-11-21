import { test, expect } from '@playwright/test';

import { computed } from './support/style';

test.use({ colorScheme: 'light' });

test.beforeEach(async ({ page }) => {
  await page.goto('/design');
});

test.describe('Checkbox', () => {
  test('it has a real label, and a required mark that screen readers hear', async ({
    page,
  }) => {
    const terms = page.locator('#demo-terms');
    await expect(page.locator('label[for="demo-terms"]')).toContainText(
      'I accept the terms',
    );
    await expect(page.locator('label[for="demo-terms"] .sr-only')).toHaveText(
      '(required)',
    );
    await expect(terms).toHaveAttribute('aria-required', 'true');
    await expect(terms).not.toBeChecked();
  });

  test('clicking the label text toggles it', async ({ page }) => {
    const email = page.locator('#demo-email');
    await expect(email).toBeChecked();
    await page.getByText('Email me when a parcel is delivered').click();
    await expect(email).not.toBeChecked();
  });

  test('the whole row is a target at least 44px high', async ({ page }) => {
    const row = page.locator('label[for="demo-terms"]');
    const box = await row.boundingBox();
    expect(box!.height).toBeGreaterThanOrEqual(44);
  });

  test('a description sits under the label inside the row', async ({
    page,
  }) => {
    await expect(
      page
        .locator('label[for="demo-email"]')
        .getByText('One email per status change.'),
    ).toBeVisible();
  });

  test('Space toggles it and the focus ring shows', async ({ page }) => {
    const terms = page.locator('#demo-terms');
    await terms.focus();
    await expect(terms).toHaveCSS('outline-width', '2px');
    await page.keyboard.press('Space');
    await expect(terms).toBeChecked();
    await page.keyboard.press('Space');
    await expect(terms).not.toBeChecked();
  });

  test('an error is announced through the box', async ({ page }) => {
    const terms = page.locator('#demo-terms-error');
    await expect(terms).toHaveAttribute('aria-invalid', 'true');
    await expect(terms).toHaveAttribute(
      'aria-describedby',
      'demo-terms-error-error',
    );
    await expect(page.locator('#demo-terms-error-error')).toHaveText(
      'Accept the terms to continue.',
    );
  });

  test('a disabled one cannot be changed and says so with its cursor', async ({
    page,
  }) => {
    const notify = page.locator('#demo-notify');
    await expect(notify).toBeDisabled();
    await expect(notify).toBeChecked();
    await expect(notify).toHaveCSS('cursor', 'not-allowed');
  });

  test('it is the primary colour, so it follows the theme', async ({
    page,
  }) => {
    const terms = page.locator('#demo-terms');
    const light = await computed(terms, 'accentColor');
    await page.evaluate(
      `document.documentElement.setAttribute('data-theme', 'dark')`,
    );
    await expect(terms).not.toHaveCSS('accent-color', light);
  });

  test('select all is indeterminate when only some are ticked, and ticks every box', async ({
    page,
  }) => {
    const all = page.locator('#demo-all');
    await expect(all).toHaveJSProperty('indeterminate', true);
    await expect(all).not.toBeChecked();

    await all.check();
    await expect(all).toHaveJSProperty('indeterminate', false);
    await expect(all).toBeChecked();
    await expect(page.locator('input[name="demo-events"]:checked')).toHaveCount(
      6,
    );

    await all.uncheck();
    await expect(page.locator('input[name="demo-events"]:checked')).toHaveCount(
      0,
    );
  });
});

test.describe('CheckboxGroup', () => {
  const group = (page: import('@playwright/test').Page) =>
    page.getByRole('group', { name: 'Send a webhook when a parcel is' });

  test('it is a fieldset named by its legend, described by its helper', async ({
    page,
  }) => {
    await expect(group(page)).toHaveAccessibleDescription(
      'Choose at least one.',
    );
    await expect(group(page).getByRole('checkbox')).toHaveCount(6);
  });

  test('it keeps its own list of ticked values', async ({ page }) => {
    await expect(
      group(page).getByRole('checkbox', { name: 'Created' }),
    ).toBeChecked();
    await expect(
      group(page).getByRole('checkbox', { name: 'Delivered' }),
    ).toBeChecked();
    await expect(
      group(page).getByRole('checkbox', { name: 'In transit' }),
    ).not.toBeChecked();

    await group(page).getByRole('checkbox', { name: 'In transit' }).check();
    await group(page).getByRole('checkbox', { name: 'Created' }).uncheck();
    await expect(group(page).locator('input:checked')).toHaveCount(2);
  });

  test('an option can carry a description', async ({ page }) => {
    await expect(group(page).getByText('Includes the reason.')).toBeVisible();
  });

  test('an error names the group and marks every box invalid', async ({
    page,
  }) => {
    const errored = page.getByRole('group', { name: /Notify me by/ });
    await expect(errored).toHaveAccessibleDescription(
      'Choose at least one way.',
    );
    await expect(errored.getByRole('checkbox').first()).toHaveAttribute(
      'aria-invalid',
      'true',
    );
    await expect(errored.locator('legend .sr-only')).toHaveText('(required)');
  });
});

test.describe('RadioGroup', () => {
  const group = (
    page: import('@playwright/test').Page,
    name: string | RegExp,
  ) => page.getByRole('group', { name });

  test('it is a fieldset with a legend and nothing chosen yet', async ({
    page,
  }) => {
    const size = group(page, /Parcel size/);
    await expect(size).toHaveAccessibleDescription('The size sets the fee.');
    await expect(size.getByRole('radio')).toHaveCount(3);
    await expect(size.locator('input:checked')).toHaveCount(0);
  });

  test('choosing one unchooses the other', async ({ page }) => {
    const size = group(page, /Parcel size/);
    await size.getByRole('radio', { name: /Small/ }).check();
    await expect(size.getByRole('radio', { name: /Small/ })).toBeChecked();
    await size.getByRole('radio', { name: /Medium/ }).check();
    await expect(size.getByRole('radio', { name: /Small/ })).not.toBeChecked();
    await expect(size.getByRole('radio', { name: /Medium/ })).toBeChecked();
  });

  test('the arrow keys move the choice, as for any radio', async ({ page }) => {
    const size = group(page, /Parcel size/);
    await size.getByRole('radio', { name: /Small/ }).check();
    await page.keyboard.press('ArrowDown');
    await expect(size.getByRole('radio', { name: /Medium/ })).toBeChecked();
  });

  test('a disabled option cannot be chosen and the arrows skip it', async ({
    page,
  }) => {
    const size = group(page, /Parcel size/);
    await expect(size.getByRole('radio', { name: /Large/ })).toBeDisabled();
    await size.getByRole('radio', { name: /Medium/ }).check();
    await page.keyboard.press('ArrowDown');
    await expect(size.getByRole('radio', { name: /Large/ })).not.toBeChecked();
  });

  test('options show their description and each row is 44px or more', async ({
    page,
  }) => {
    const size = group(page, /Parcel size/);
    await expect(size.getByText('Up to 5 kg, fits a shoebox.')).toBeVisible();
    const box = await size.locator('label').first().boundingBox();
    expect(box!.height).toBeGreaterThanOrEqual(44);
  });

  test('a horizontal group sits in a row from 768px and stacks on a phone', async ({
    page,
  }) => {
    const speed = group(page, 'Delivery speed');
    const labels = speed.locator('label');

    await page.setViewportSize({ width: 1280, height: 800 });
    const wide = [
      await labels.nth(0).boundingBox(),
      await labels.nth(1).boundingBox(),
    ];
    expect(wide[0]!.y).toBeCloseTo(wide[1]!.y, 0);

    await page.setViewportSize({ width: 360, height: 800 });
    const narrow = [
      await labels.nth(0).boundingBox(),
      await labels.nth(1).boundingBox(),
    ];
    expect(narrow[1]!.y).toBeGreaterThan(narrow[0]!.y);
  });

  test('an error describes the group', async ({ page }) => {
    const errored = group(page, 'Pickup window');
    await expect(errored).toHaveAccessibleDescription(
      'Choose a pickup window.',
    );
    await expect(errored.getByRole('radio').first()).toHaveAttribute(
      'aria-invalid',
      'true',
    );
  });
});

test.describe('Date and time fields', () => {
  test('a date field is a labelled TextField with the browser picker', async ({
    page,
  }) => {
    const date = page.locator('#demo-date');
    await expect(page.locator('label[for="demo-date"]')).toContainText(
      'Pickup date',
    );
    await expect(date).toHaveAttribute('type', 'date');
    await expect(date).toHaveAttribute('min', '2026-11-04');
    await expect(date).toHaveValue('2026-11-05');
    await expect(date).toHaveAttribute('aria-describedby', 'demo-date-helper');
    await date.fill('2026-12-01');
    await expect(date).toHaveValue('2026-12-01');
  });

  test('date and time fields are the height and radius of a text field', async ({
    page,
  }) => {
    for (const id of ['#demo-date', '#demo-time', '#demo-datetime']) {
      await expect(page.locator(id)).toHaveCSS('height', '40px');
      await expect(page.locator(id)).toHaveCSS('border-radius', '8px');
    }
  });

  test('a time field takes a time and an error marks a date-time field', async ({
    page,
  }) => {
    const time = page.locator('#demo-time');
    await expect(time).toHaveAttribute('type', 'time');
    await time.fill('14:15');
    await expect(time).toHaveValue('14:15');

    const datetime = page.locator('#demo-datetime');
    await expect(datetime).toHaveAttribute('type', 'datetime-local');
    await expect(datetime).toHaveAttribute('aria-invalid', 'true');
  });
});

test.describe('FileField', () => {
  test('the input is named by its label and hints at the camera on a phone', async ({
    page,
  }) => {
    const photo = page.locator('#demo-photo');
    await expect(page.locator('label[for="demo-photo"]')).toContainText(
      'Proof photo',
    );
    await expect(photo).toHaveAttribute('accept', 'image/jpeg,image/png');
    await expect(photo).toHaveAttribute('capture', 'environment');
    await expect(photo).toHaveAttribute('type', 'file');
  });

  test('before a choice it says so, and the button has a clear word', async ({
    page,
  }) => {
    await expect(page.getByText('No photo yet')).toBeVisible();
    await expect(page.getByText('Take or choose a photo')).toBeVisible();
  });

  test('choosing a file shows its name and tells the screen', async ({
    page,
  }) => {
    await page.locator('#demo-photo').setInputFiles({
      name: 'delivered.jpg',
      mimeType: 'image/jpeg',
      buffer: Buffer.from('not really a photo'),
    });
    await expect(
      page.getByText('delivered.jpg', { exact: true }),
    ).toBeVisible();
    await expect(page.getByText('Chosen: delivered.jpg')).toBeVisible();
    await expect(page.getByText('No photo yet')).toBeHidden();
  });

  test('the real input covers the button, so a click anywhere on it works', async ({
    page,
  }) => {
    const photo = page.locator('#demo-photo');
    const button = page.locator('#demo-photo + span');
    const input = await photo.boundingBox();
    const look = await button.boundingBox();
    expect(Math.abs(input!.width - look!.width)).toBeLessThan(2);
    expect(Math.abs(input!.height - look!.height)).toBeLessThan(2);
    await expect(button).toHaveAttribute('aria-hidden', 'true');
  });

  test('the keyboard focus ring shows on the button', async ({ page }) => {
    await page.locator('#demo-photo').focus();
    await expect(page.locator('#demo-photo + span')).toHaveCSS(
      'outline-width',
      '2px',
    );
  });

  test('the courier size is 48px', async ({ page }) => {
    await expect(page.locator('#demo-photo + span')).toHaveCSS(
      'height',
      '48px',
    );
    await expect(page.locator('#demo-photo-error + span')).toHaveCSS(
      'height',
      '40px',
    );
  });

  test('an error and a disabled field are announced', async ({ page }) => {
    const errored = page.locator('#demo-photo-error');
    await expect(errored).toHaveAttribute('aria-invalid', 'true');
    await expect(errored).toHaveAttribute(
      'aria-describedby',
      'demo-photo-error-error',
    );
    await expect(page.locator('#demo-photo-error-error')).toContainText(
      'Add a photo',
    );
    await expect(page.locator('#demo-photo-disabled')).toBeDisabled();
  });
});
