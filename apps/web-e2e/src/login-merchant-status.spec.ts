import { expect, test, type Page } from '@playwright/test';

import { fail, mockApi, signedOut } from './support/mock-api';

test.use({ colorScheme: 'light' });

type Detail = { field: string; issue: string };

/** Signs in as a merchant owner whose login the API refuses with these details. */
async function refusedLogin(page: Page, details?: Detail[]) {
  await mockApi(page, {
    ...signedOut,
    'POST /auth/login': fail(403, 'MERCHANT_ACCOUNT_INACTIVE', { details }),
  });
  await page.goto('/login');
  await page.locator('#login-email').fill('owner@shopzone.lk');
  await page.locator('#login-password').fill('Str0ngPass!');
  await page.getByRole('button', { name: 'Log in' }).click();
}

test.describe('a merchant owner is told where the application stands', () => {
  test('a rejection shows the administrator’s reason and the support address', async ({
    page,
  }) => {
    await refusedLogin(page, [
      { field: 'status', issue: 'REJECTED' },
      {
        field: 'reason',
        issue: 'The registration number could not be verified.',
      },
      { field: 'supportEmail', issue: 'support@swiftdrop.example' },
    ]);

    const alert = page.getByRole('alert');
    await expect(alert.getByText('Application not approved')).toBeVisible();
    await expect(
      alert.getByText('Reason from the administrator'),
    ).toBeVisible();
    await expect(
      alert.getByText('The registration number could not be verified.'),
    ).toBeVisible();
    await expect(
      alert.getByRole('link', { name: 'support@swiftdrop.example' }),
    ).toHaveAttribute('href', 'mailto:support@swiftdrop.example');
    // The merchant stays on the login page: no session was created.
    await expect(page).toHaveURL(/\/login/);
  });

  test('a rejection with no reason says so, and without a support address points to support', async ({
    page,
  }) => {
    await refusedLogin(page, [{ field: 'status', issue: 'REJECTED' }]);

    const alert = page.getByRole('alert');
    await expect(alert.getByText('Application not approved')).toBeVisible();
    await expect(alert.getByText('No reason was given.')).toBeVisible();
    await expect(
      alert.getByText('To ask about this decision, contact SwiftDrop support.'),
    ).toBeVisible();
    await expect(alert.getByRole('link')).toHaveCount(0);
  });

  test('a suspension shows its reason', async ({ page }) => {
    await refusedLogin(page, [
      { field: 'status', issue: 'SUSPENDED' },
      { field: 'reason', issue: 'Repeated delivery complaints.' },
      { field: 'supportEmail', issue: 'support@swiftdrop.example' },
    ]);

    const alert = page.getByRole('alert');
    await expect(alert.getByText('Merchant account suspended')).toBeVisible();
    await expect(
      alert.getByText('Repeated delivery complaints.'),
    ).toBeVisible();
  });

  test('a pending application says it is under review and has no reason block', async ({
    page,
  }) => {
    await refusedLogin(page, [{ field: 'status', issue: 'PENDING' }]);

    await expect(page.getByText('Application under review')).toBeVisible();
    await expect(page.getByText('Reason from the administrator')).toHaveCount(
      0,
    );
  });

  test('without details the plain refusal stays', async ({ page }) => {
    await refusedLogin(page);

    await expect(page.getByText('Merchant account not active')).toBeVisible();
    await expect(page.getByText('Reason from the administrator')).toHaveCount(
      0,
    );
  });

  test('the reason is shown as plain text, never as markup', async ({
    page,
  }) => {
    await refusedLogin(page, [
      { field: 'status', issue: 'REJECTED' },
      { field: 'reason', issue: '<img src=x onerror="window.__reason=1">' },
      // A support address that is not a plain address never becomes a link.
      { field: 'supportEmail', issue: 'javascript:alert(1)' },
    ]);

    await expect(
      page.getByText('<img src=x onerror="window.__reason=1">'),
    ).toBeVisible();
    expect(await page.evaluate<boolean>("'__reason' in window")).toBe(false);
    await expect(page.getByRole('alert').getByRole('link')).toHaveCount(0);
  });

  test('a long reason wraps at 375px and the page does not scroll sideways', async ({
    page,
  }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    await refusedLogin(page, [
      { field: 'status', issue: 'REJECTED' },
      {
        field: 'reason',
        issue: `${'Unverifiable-registration-number-'.repeat(8)}\nSecond line of the administrator’s note.`,
      },
      { field: 'supportEmail', issue: 'support@swiftdrop.example' },
    ]);

    await expect(page.getByText('Application not approved')).toBeVisible();
    const overflow = await page.evaluate<number>(
      'document.documentElement.scrollWidth - window.innerWidth',
    );
    expect(overflow).toBeLessThanOrEqual(0);
  });
});
