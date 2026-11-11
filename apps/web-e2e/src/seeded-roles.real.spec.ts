import { expect, test } from '@playwright/test';

import {
  credentialsFor,
  credentialsForRun,
  isCI,
  type SeededAccount,
} from './support/real-stack';

/**
 * Real stack: every seeded demo user can sign in and reaches their own home.
 * Run with the stack up and the seed applied:
 *
 *   REAL_STACK=1 pnpm exec nx e2e @org/web-e2e --project=real-stack
 *
 * See support/real-stack.ts for the REAL_STACK_*_EMAIL / _PASSWORD variables.
 */
const accounts: Array<{
  account: SeededAccount;
  label: string;
  home: RegExp;
}> = [
  { account: 'ADMIN', label: 'administrator', home: /\/dashboard$/ },
  { account: 'DISPATCHER', label: 'dispatcher', home: /\/unassigned$/ },
  { account: 'COURIER_1', label: 'first courier', home: /\/dashboard$/ },
  { account: 'COURIER_2', label: 'second courier', home: /\/dashboard$/ },
  { account: 'CUSTOMER', label: 'customer', home: /\/dashboard$/ },
];

test.describe('seeded users on the real stack', () => {
  // Logins share a rate limit, so one retry covers a brief 429.
  test.describe.configure({ retries: 1 });

  test('every demo account is available to the run', () => {
    test.skip(!isCI, 'only enforced in CI');
    const missing = accounts
      .map(({ account }) => account)
      .filter((account) => !credentialsFor(account));
    expect(missing).toEqual([]);
  });

  for (const { account, label, home } of accounts) {
    test(`the ${label} signs in and reaches their home page`, async ({
      page,
    }) => {
      const credentials = credentialsForRun(account);
      test.skip(
        !credentials,
        `set REAL_STACK_${account}_EMAIL and REAL_STACK_${account}_PASSWORD`,
      );
      if (!credentials) return;

      await page.goto('/login');
      await page.locator('#login-email').fill(credentials.email);
      await page.locator('#login-password').fill(credentials.password);
      await page.getByRole('button', { name: 'Log in' }).click();

      await expect(page).toHaveURL(home);
      await expect(page.getByRole('main')).toBeVisible();
      // Signed in: the login form is gone.
      await expect(page.locator('#login-email')).toHaveCount(0);
    });
  }

  test('a wrong password does not sign anyone in', async ({ page }) => {
    const credentials = credentialsForRun('CUSTOMER');
    test.skip(!credentials, 'set the REAL_STACK_CUSTOMER_* variables');
    if (!credentials) return;

    await page.goto('/login');
    await page.locator('#login-email').fill(credentials.email);
    await page.locator('#login-password').fill(`${credentials.password}-wrong`);
    await page.getByRole('button', { name: 'Log in' }).click();

    await expect(page).toHaveURL(/\/login/);
    await expect(page.locator('#login-email')).toBeVisible();
  });
});
