import { test, expect, type Page } from '@playwright/test';

import { fail, mockApi } from './support/mock-api';
import { signInAs, type Role } from './support/sign-in';

const sidebar = (page: Page) => page.locator('aside nav[aria-label="Main"]');

test.use({ colorScheme: 'light' });

test.describe('navigation by role', () => {
  const sidebarLabels: Record<Exclude<Role, 'COURIER'>, string[]> = {
    CUSTOMER: ['Dashboard', 'New delivery', 'My parcels', 'Profile'],
    MERCHANT: ['Dashboard', 'Orders', 'API keys', 'Webhook', 'Profile'],
    DISPATCHER: ['Unassigned', 'Workload', 'All parcels', 'Reports'],
    ADMIN: [
      'Unassigned',
      'Workload',
      'All parcels',
      'Reports',
      'Users',
      'Merchants',
      'Zones',
      'Pricing',
    ],
  };

  for (const [role, labels] of Object.entries(sidebarLabels) as [
    Exclude<Role, 'COURIER'>,
    string[],
  ][]) {
    test(`${role} gets exactly its own sidebar`, async ({ page }) => {
      await page.setViewportSize({ width: 1440, height: 900 });
      await signInAs(page, role);
      await page.goto('/dashboard');

      const links = sidebar(page).getByRole('link');
      await expect(links).toHaveText(labels);
    });
  }

  test('only an admin sees the Administration group, marked ADMIN', async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
    await signInAs(page, 'DISPATCHER');
    await page.goto('/dashboard');
    await expect(page.getByText('Administration')).toHaveCount(0);

    await page.unroute('**/api/v1/**');
    await signInAs(page, 'ADMIN');
    await page.goto('/dashboard');
    await expect(
      sidebar(page).getByRole('group', { name: 'Administration' }),
    ).toBeVisible();
    await expect(
      sidebar(page).getByText('ADMIN', { exact: true }),
    ).toBeVisible();
  });

  test('the courier gets bottom tabs and no sidebar', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    await signInAs(page, 'COURIER');
    await page.goto('/dashboard');

    await expect(page.locator('aside')).toHaveCount(0);
    const tabs = page
      .getByRole('navigation', { name: 'Main' })
      .getByRole('link');
    await expect(tabs).toHaveText(['Today', 'History', 'Profile']);
    await expect(tabs.first()).toHaveAttribute('aria-current', 'page');
  });
});

test.describe('the sidebar', () => {
  test.beforeEach(async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
  });

  test('marks the current page, and a page that is not built yet shows 404 inside the shell', async ({
    page,
  }) => {
    await signInAs(page, 'CUSTOMER');
    await page.goto('/dashboard');
    await expect(
      sidebar(page).getByRole('link', { name: 'Dashboard' }),
    ).toHaveAttribute('aria-current', 'page');

    await sidebar(page).getByRole('link', { name: 'My parcels' }).click();
    await expect(page).toHaveURL(/\/parcels$/);
    await expect(
      page.getByRole('heading', { name: 'This page does not exist' }),
    ).toBeVisible();
    await expect(
      sidebar(page).getByRole('link', { name: 'My parcels' }),
    ).toHaveAttribute('aria-current', 'page');
    await expect(
      page.getByRole('link', { name: 'Go to my dashboard' }),
    ).toHaveAttribute('href', '/dashboard');
  });

  test('is 264px wide, folds to a 72px rail, and remembers the choice', async ({
    page,
  }) => {
    await signInAs(page, 'MERCHANT');
    await page.goto('/dashboard');
    const aside = page.locator('aside');
    await expect(aside).toHaveCSS('width', '264px');

    await page.getByRole('button', { name: 'Collapse navigation' }).click();
    await expect(aside).toHaveCSS('width', '72px');
    // In the rail the labels stay for screen readers.
    await expect(
      sidebar(page).getByRole('link', { name: 'Orders' }),
    ).toHaveCount(1);

    await page.reload();
    await expect(aside).toHaveCSS('width', '72px');
    await page.getByRole('button', { name: 'Expand navigation' }).click();
    await expect(aside).toHaveCSS('width', '264px');
  });

  test('is a 72px rail on a tablet', async ({ page }) => {
    await page.setViewportSize({ width: 800, height: 900 });
    await signInAs(page, 'CUSTOMER');
    await page.goto('/dashboard');

    await expect(page.locator('aside')).toHaveCSS('width', '72px');
  });

  test('the first focusable element is a skip link to the content', async ({
    page,
  }) => {
    await signInAs(page, 'CUSTOMER');
    await page.goto('/dashboard');
    // The account menu only exists once the shell has rendered and hydrated.
    await expect(
      page.getByRole('button', { name: 'Account menu' }),
    ).toBeEnabled();

    // First in the page order is first in the tab order, whatever has focus now.
    const first = await page.evaluate<string>(
      `(document.querySelector('a[href], button:not([disabled]), input, select, textarea, [tabindex]:not([tabindex="-1"])')?.textContent ?? '').trim()`,
    );
    expect(first).toBe('Skip to the content');

    const skip = page.getByRole('link', { name: 'Skip to the content' });
    await skip.focus();
    await expect(skip).toBeVisible();
    await page.keyboard.press('Enter');
    await expect(page.locator('#main')).toBeFocused();
  });
});

test.describe('the drawer under 768px', () => {
  test.beforeEach(async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 812 });
  });

  test('opens from the top bar, traps focus, closes on Escape and returns focus', async ({
    page,
  }) => {
    await signInAs(page, 'DISPATCHER');
    await page.goto('/dashboard');
    const opener = page.getByRole('button', { name: 'Open navigation' });
    await expect(page.locator('aside')).toBeHidden();

    await opener.click();
    const drawer = page.getByRole('dialog', { name: 'Navigation' });
    await expect(drawer).toBeVisible();
    await expect(drawer).toHaveCSS('width', '240px');
    await expect(
      drawer.getByRole('link', { name: 'Unassigned' }),
    ).toBeVisible();

    await page.keyboard.press('Escape');
    await expect(drawer).toBeHidden();
    await expect(opener).toBeFocused();
  });

  test('closes itself when a link is chosen', async ({ page }) => {
    await signInAs(page, 'DISPATCHER');
    await page.goto('/dashboard');

    await page.getByRole('button', { name: 'Open navigation' }).click();
    await page
      .getByRole('dialog', { name: 'Navigation' })
      .getByRole('link', { name: 'Workload' })
      .click();
    await expect(page).toHaveURL(/\/workload$/);
    await expect(page.getByRole('dialog', { name: 'Navigation' })).toBeHidden();
  });

  test('rows are at least 48px tall for touch', async ({ page }) => {
    await signInAs(page, 'CUSTOMER');
    await page.goto('/dashboard');
    await page.getByRole('button', { name: 'Open navigation' }).click();

    const row = page
      .getByRole('dialog', { name: 'Navigation' })
      .getByRole('link', { name: 'My parcels' });
    await expect(row).toHaveCSS('height', '48px');
  });
});

test.describe('the account menu', () => {
  test.beforeEach(async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
  });

  test('says who is signed in and their role first', async ({ page }) => {
    await signInAs(page, 'ADMIN');
    await page.goto('/dashboard');
    await page.getByRole('button', { name: 'Account menu' }).click();

    const menu = page.getByRole('menu', { name: 'Account' });
    await expect(menu).toContainText('Anusha Nawarathna');
    await expect(menu).toContainText('admin@example.com');
    await expect(menu).toContainText('ADMIN');
    await expect(menu.getByRole('menuitem')).toHaveText([
      'Profile',
      'Change password',
      'Log out',
    ]);
  });

  test('works from the keyboard: arrows, Home, End and Escape', async ({
    page,
  }) => {
    await signInAs(page, 'CUSTOMER');
    await page.goto('/dashboard');
    const trigger = page.getByRole('button', { name: 'Account menu' });
    await trigger.focus();
    await page.keyboard.press('Enter');

    const items = page.getByRole('menuitem');
    await expect(items.nth(0)).toBeFocused();
    await page.keyboard.press('ArrowDown');
    await expect(items.nth(1)).toBeFocused();
    await page.keyboard.press('End');
    await expect(items.nth(2)).toBeFocused();
    await page.keyboard.press('ArrowDown');
    await expect(items.nth(0)).toBeFocused();
    await page.keyboard.press('Escape');
    await expect(page.getByRole('menu')).toHaveCount(0);
    await expect(trigger).toBeFocused();
  });

  test('closes when the page is clicked elsewhere', async ({ page }) => {
    await signInAs(page, 'CUSTOMER');
    await page.goto('/dashboard');
    await page.getByRole('button', { name: 'Account menu' }).click();
    await expect(page.getByRole('menu')).toBeVisible();

    await page.getByRole('heading', { name: 'Dashboard' }).click();
    await expect(page.getByRole('menu')).toHaveCount(0);
  });

  test('logging out ends the session and shows the signed-out page', async ({
    page,
  }) => {
    const api = await signInAs(page, 'CUSTOMER');
    await page.goto('/dashboard');
    await page.getByRole('button', { name: 'Account menu' }).click();
    await page.getByRole('menuitem', { name: 'Log out' }).click();

    await expect(page).toHaveURL(/\/signed-out$/);
    expect(api.calls.map((call) => call.key)).toContain('POST /auth/logout');
  });

  test('the theme switch is in the top bar from 768px, and in the menu below it', async ({
    page,
  }) => {
    await signInAs(page, 'CUSTOMER');
    await page.goto('/dashboard');
    await page.getByRole('button', { name: 'Dark' }).click();
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');

    await page.setViewportSize({ width: 375, height: 812 });
    await page.getByRole('button', { name: 'Account menu' }).click();
    await expect(
      page.getByRole('menu').getByRole('group', { name: 'Theme' }),
    ).toBeVisible();
  });
});

test.describe('roles and missing pages', () => {
  test.beforeEach(async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
  });

  test('a role without access sees 403 inside the shell, not an empty page', async ({
    page,
  }) => {
    await signInAs(page, 'DISPATCHER');
    await page.goto('/admin/users');

    await expect(
      page.getByRole('heading', { name: 'You do not have access' }),
    ).toBeVisible();
    await expect(
      page.getByText('You are signed in as a dispatcher'),
    ).toBeVisible();
    await expect(
      sidebar(page).getByRole('link', { name: 'Workload' }),
    ).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Users' })).toHaveCount(0);
  });

  test('an admin opens the same page', async ({ page }) => {
    await signInAs(page, 'ADMIN');
    await page.goto('/admin/users');

    await expect(
      page.getByRole('heading', { name: 'Users', level: 1 }),
    ).toBeVisible();
    await expect(page.getByText('You do not have access')).toHaveCount(0);
  });

  test('an unknown signed-in page shows 404 inside the shell', async ({
    page,
  }) => {
    await signInAs(page, 'CUSTOMER');
    await page.goto('/nowhere-at-all');

    await expect(
      page.getByRole('heading', { name: 'This page does not exist' }),
    ).toBeVisible();
    await expect(
      sidebar(page).getByRole('link', { name: 'Dashboard' }),
    ).toBeVisible();
  });

  test('an unknown page for a signed-out visitor goes to login', async ({
    page,
  }) => {
    await mockApi(page, {
      'GET /users/me': fail(401, 'UNAUTHENTICATED'),
      'POST /auth/refresh': fail(401, 'REFRESH_TOKEN_INVALID'),
    });
    await page.goto('/nowhere-at-all');

    await expect(page).toHaveURL(/\/login\?next=%2Fnowhere-at-all$/);
  });
});

test.describe('dark theme', () => {
  test.use({ colorScheme: 'dark' });

  test('the shell follows the dark tokens', async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
    await signInAs(page, 'ADMIN');
    await page.goto('/dashboard');

    await expect(page.locator('aside')).toHaveCSS(
      'background-color',
      'rgb(23, 27, 30)',
    );
    await expect(page.locator('body')).toHaveCSS(
      'background-color',
      'rgb(14, 17, 19)',
    );
  });

  test('the ADMIN badges stay readable', async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
    await signInAs(page, 'ADMIN');
    await page.goto('/dashboard');

    // A light fill with dark text. The badges once used the fixed ink and
    // paper colours, which are both near-black here, so the label vanished.
    const sidebarBadge = page
      .locator('aside')
      .getByText('ADMIN', { exact: true });
    await expect(sidebarBadge).toHaveCSS(
      'background-color',
      'rgb(236, 234, 228)',
    );
    await expect(sidebarBadge).toHaveCSS('color', 'rgb(20, 23, 26)');

    await page.getByRole('button', { name: 'Account menu' }).click();
    // The role badge starts with a lettered square, so its text is "AADMIN".
    const roleBadge = page
      .getByRole('menu', { name: 'Account' })
      .locator('span')
      .filter({ hasText: /^AADMIN$/ });
    await expect(roleBadge).toHaveCSS('background-color', 'rgb(236, 234, 228)');
    await expect(roleBadge).toHaveCSS('color', 'rgb(20, 23, 26)');
  });
});
