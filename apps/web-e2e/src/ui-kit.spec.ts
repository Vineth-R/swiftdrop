import { test, expect } from '@playwright/test';

test.use({ colorScheme: 'light' });

test.beforeEach(async ({ page }) => {
  await page.goto('/design');
});

test.describe('Button', () => {
  test('a disabled button uses the disabled attribute', async ({ page }) => {
    const disabled = page.getByRole('button', { name: 'Create' }).nth(1);
    await expect(disabled).toBeDisabled();
  });

  test('a loading button is busy, stays focusable and cannot submit', async ({
    page,
  }) => {
    const loading = page.locator('button[aria-busy="true"]').first();
    await expect(loading).toHaveAttribute('aria-disabled', 'true');
    await expect(loading).toHaveAttribute('type', 'button');
    // toBeEnabled() would fail here: Playwright treats aria-disabled as disabled.
    await expect(loading).not.toHaveAttribute('disabled');
    await loading.focus();
    await expect(loading).toBeFocused();
  });

  test('buttons are 8px radius and 40px high at medium size', async ({
    page,
  }) => {
    const medium = page.getByRole('button', { name: 'Medium' });
    await expect(medium).toHaveCSS('height', '40px');
    await expect(medium).toHaveCSS('border-radius', '8px');
  });
});

test.describe('TextField', () => {
  test('an error replaces the helper and is wired to the input', async ({
    page,
  }) => {
    const phone = page.locator('#demo-phone');
    await expect(phone).toHaveAttribute('aria-invalid', 'true');
    await expect(phone).toHaveAttribute('aria-describedby', 'demo-phone-error');
    await expect(page.locator('#demo-phone-error')).toContainText(
      'Enter a full number',
    );
    await expect(page.getByLabel('Recipient phone')).toHaveAttribute(
      'aria-required',
      'true',
    );
  });

  test('a helper is wired to its input', async ({ page }) => {
    await expect(page.locator('#demo-name')).toHaveAttribute(
      'aria-describedby',
      'demo-name-helper',
    );
  });

  test('read-only and disabled fields are not editable', async ({ page }) => {
    await expect(page.locator('#demo-readonly')).not.toBeEditable();
    await expect(page.locator('#demo-disabled')).toBeDisabled();
  });
});

test.describe('PasswordField', () => {
  test('the checklist follows what is typed', async ({ page }) => {
    const input = page.locator('#demo-new-password');
    await input.fill('abc');

    const list = page.getByRole('list').filter({ hasText: 'One digit' });
    await expect(list).toContainText('8 to 72 characters');
    await expect(list.getByText('One lower case letter')).toBeVisible();
    await expect(page.getByText('3 / 72')).toBeVisible();
    await expect(page.getByText('Weak — two more to go')).toBeVisible();

    await input.fill('Colombo26');
    await expect(page.getByText('Meets the rule')).toBeVisible();
    await expect(page.getByText('9 / 72')).toBeVisible();
  });

  test('the checklist is announced politely', async ({ page }) => {
    await expect(
      page.getByRole('list').filter({ hasText: 'One digit' }),
    ).toHaveAttribute('aria-live', 'polite');
  });

  test('Show and Hide reveal the password and keep a clear name', async ({
    page,
  }) => {
    const input = page.locator('#demo-password');
    await expect(input).toHaveAttribute('type', 'password');
    await expect(input).toHaveAttribute('autocomplete', 'current-password');

    await page.getByRole('button', { name: 'Show password' }).last().click();
    await expect(input).toHaveAttribute('type', 'text');
    await expect(
      page.getByRole('button', { name: 'Hide password' }),
    ).toHaveAttribute('aria-pressed', 'true');
  });

  test('the new password field tells password managers it is new', async ({
    page,
  }) => {
    await expect(page.locator('#demo-new-password')).toHaveAttribute(
      'autocomplete',
      'new-password',
    );
  });
});

test.describe('Alert and ErrorSummary', () => {
  test('danger alerts interrupt, other tones wait', async ({ page }) => {
    await expect(
      page.getByRole('alert').filter({ hasText: 'Delivery failed' }),
    ).toBeVisible();
    await expect(
      page.getByRole('status').filter({ hasText: 'Proof of delivery saved' }),
    ).toBeVisible();
  });

  test('the error summary counts its issues and links to the fields', async ({
    page,
  }) => {
    const summary = page
      .getByRole('alert')
      .filter({ hasText: 'fields need attention' });
    await expect(summary).toContainText('2 fields need attention');
    await expect(
      summary.getByRole('link', { name: 'Recipient phone' }),
    ).toHaveAttribute('href', '#demo-phone');
    await expect(summary).toContainText('Reference 7f2c9a14');
  });
});

test.describe('Skeleton', () => {
  const group = (page: import('@playwright/test').Page) =>
    page.getByRole('status').filter({ hasText: 'Loading parcels' });

  test('a group says once what is loading and is marked busy', async ({
    page,
  }) => {
    await expect(group(page)).toHaveAttribute('aria-busy', 'true');
    await expect(group(page)).toContainText('Loading parcels');
  });

  test('the grey blocks are hidden from screen readers', async ({ page }) => {
    const blocks = group(page).locator('[aria-hidden="true"]');
    expect(await blocks.count()).toBeGreaterThan(3);
    await expect(blocks.first()).toBeAttached();
  });

  test('a block is still, and square-cornered like a chip', async ({
    page,
  }) => {
    const block = group(page).locator('[aria-hidden="true"]').first();
    await expect(block).toHaveCSS('animation-name', 'none');
    await expect(block).toHaveCSS('border-radius', '6px');
  });

  test('a text skeleton ends in a shorter line', async ({ page }) => {
    const lines = group(page).locator('div[aria-hidden="true"] > div');
    await expect(lines).toHaveCount(3);
    const widths = await Promise.all(
      [0, 1, 2].map(
        async (index) => (await lines.nth(index).boundingBox())?.width ?? 0,
      ),
    );
    expect(widths[2]).toBeLessThan(widths[0]);
    expect(widths[0]).toBe(widths[1]);
  });
});

test.describe('EmptyState', () => {
  test('its title is a third level heading and its action a button', async ({
    page,
  }) => {
    await expect(
      page.getByRole('heading', { level: 3, name: 'No parcels yet' }),
    ).toBeVisible();
    await expect(
      page.getByRole('button', { name: 'New delivery' }),
    ).toBeVisible();
  });

  test('the icon is decorative and a state without an action has none', async ({
    page,
  }) => {
    const withIcon = page
      .getByRole('heading', { name: 'No parcels yet' })
      .locator('xpath=ancestor::div[contains(@class, "rounded-card")][1]');
    await expect(withIcon.locator('[aria-hidden="true"] svg')).toHaveCount(1);

    const withoutAction = page
      .getByRole('heading', { name: /No results for/ })
      .locator('xpath=ancestor::div[contains(@class, "rounded-card")][1]');
    await expect(withoutAction.getByRole('button')).toHaveCount(0);
    await expect(withoutAction.locator('svg')).toHaveCount(0);
  });

  test('it is a hairline box, with no shadow', async ({ page }) => {
    const box = page
      .getByRole('heading', { name: 'No parcels yet' })
      .locator('xpath=ancestor::div[contains(@class, "rounded-card")][1]');
    await expect(box).toHaveCSS('border-top-width', '1px');
    await expect(box).toHaveCSS('box-shadow', 'none');
    await expect(box).toHaveCSS('border-radius', '12px');
  });
});
