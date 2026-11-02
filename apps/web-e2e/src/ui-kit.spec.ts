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
    await expect(loading).toBeEnabled();
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
