import { expect, test, type Page } from '@playwright/test';

import { SERVER_WORDS, fail, mockApi, ok, signedOut } from './support/mock-api';
import { signInAs } from './support/sign-in';

test.use({ colorScheme: 'light' });

const logs = '/admin/notification-logs?status=FAILED';
const dialog = (page: Page) =>
  page.getByRole('dialog', { name: 'Your session ended' });

/** An administrator whose notification-log call finds the session gone. */
async function expiredOnLogs(page: Page) {
  return signInAs(page, 'ADMIN', {
    'GET /admin/notification-logs': fail(401, 'UNAUTHENTICATED'),
  });
}

test.describe('the session-expired dialog', () => {
  test('a session that ends while a page is open shows a dialog, not the login page', async ({
    page,
  }) => {
    await expiredOnLogs(page);
    await page.goto(logs);

    await expect(dialog(page)).toBeVisible();
    await expect(
      dialog(page).getByText(
        'You were signed out. Log in again to carry on where you were.',
      ),
    ).toBeVisible();
    // The person is still on the page they were using.
    await expect(page).toHaveURL(/\/admin\/notification-logs/);
    await expect(
      page.getByRole('heading', { level: 1, name: 'Notification logs' }),
    ).toBeVisible();
    await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
  });

  test('Log in brings the person back to the same page, query string included', async ({
    page,
  }) => {
    await expiredOnLogs(page);
    await page.goto(logs);

    await dialog(page).getByRole('button', { name: 'Log in' }).click();

    await expect(page).toHaveURL(
      /\/login\?next=%2Fadmin%2Fnotification-logs%3Fstatus%3DFAILED$/,
    );
  });

  test('it must be answered: Escape, a click outside and a close button do nothing', async ({
    page,
  }) => {
    await expiredOnLogs(page);
    await page.goto(logs);
    const box = dialog(page);
    await expect(box).toBeVisible();

    // Focus starts on the one action, and the page behind cannot be reached.
    await expect(box.getByRole('button', { name: 'Log in' })).toBeFocused();
    await expect(box.getByRole('button', { name: 'Close' })).toHaveCount(0);
    await page.keyboard.press('Escape');
    await expect(box).toBeVisible();
    await page.mouse.click(5, 5);
    await expect(box).toBeVisible();
  });

  test('several requests failing together show one dialog', async ({
    page,
  }) => {
    await expiredOnLogs(page);
    await page.goto(logs);

    await expect(page.getByRole('dialog')).toHaveCount(1);
  });

  test('someone who was never signed in goes straight to the login page, with no dialog', async ({
    page,
  }) => {
    await mockApi(page, signedOut);
    await page.goto('/dashboard');

    await expect(page).toHaveURL(/\/login\?next=%2Fdashboard$/);
    await expect(page.getByRole('dialog')).toHaveCount(0);
  });

  test('a session that is renewed quietly never shows it', async ({ page }) => {
    let calls = 0;
    await signInAs(page, 'ADMIN', {
      'GET /admin/notification-logs': () => {
        calls += 1;
        return calls === 1
          ? fail(401, 'UNAUTHENTICATED')
          : (ok([]) as ReturnType<typeof fail>);
      },
      'POST /auth/refresh': ok({}),
    });
    await page.goto(logs);

    await expect(
      page.getByRole('heading', { level: 1, name: 'Notification logs' }),
    ).toBeVisible();
    await expect(page.getByRole('dialog')).toHaveCount(0);
  });

  test('it reads in the dark theme and fits a 375px screen with a 44px button', async ({
    page,
  }) => {
    await page.emulateMedia({ colorScheme: 'dark' });
    await page.setViewportSize({ width: 375, height: 812 });
    await expiredOnLogs(page);
    await page.goto(logs);

    const button = dialog(page).getByRole('button', { name: 'Log in' });
    await expect(button).toBeVisible();
    const box = await button.boundingBox();
    expect(box?.height ?? 0).toBeGreaterThanOrEqual(44);
    const overflow = await page.evaluate<number>(
      'document.documentElement.scrollWidth - window.innerWidth',
    );
    expect(overflow).toBeLessThanOrEqual(0);
  });
});
