import { test, expect } from '@playwright/test';

import { mockApi } from './support/mock-api';
import { signInAs } from './support/sign-in';
import { computed } from './support/style';

test.use({ colorScheme: 'light' });

test.describe('PublicLayout at /track', () => {
  test('it opens with no session and makes no API call', async ({ page }) => {
    const requests: string[] = [];
    page.on('request', (request) => {
      if (request.url().includes('/api/')) requests.push(request.url());
    });

    await page.goto('/track');
    await expect(
      page.getByRole('heading', { level: 1, name: 'Track a parcel' }),
    ).toBeVisible();
    expect(requests).toEqual([]);
  });

  test('it is the public frame, not the signed-in shell', async ({ page }) => {
    await page.goto('/track');
    await expect(
      page.getByRole('link', { name: 'SwiftDrop home' }),
    ).toHaveAttribute('href', '/');
    await expect(page.getByRole('link', { name: 'Log in' })).toHaveAttribute(
      'href',
      '/login',
    );
    await expect(page.getByRole('group', { name: 'Theme' })).toBeVisible();
    await expect(
      page.getByRole('navigation', { name: /main|primary/i }),
    ).toHaveCount(0);
  });

  test('a signed-in visitor can open it and is not redirected', async ({
    page,
  }) => {
    await signInAs(page, 'CUSTOMER');
    await page.goto('/track');
    await expect(page).toHaveURL(/\/track$/);
    await expect(
      page.getByRole('heading', { level: 1, name: 'Track a parcel' }),
    ).toBeVisible();
  });

  test('the skip link is the first tab stop and lands on the content', async ({
    page,
  }) => {
    await page.goto('/track');
    await page.keyboard.press('Tab');
    const skip = page.getByRole('link', { name: 'Skip to the content' });
    await expect(skip).toBeFocused();
    await expect(skip).toBeVisible();
    await page.keyboard.press('Enter');
    await expect(page).toHaveURL(/#main$/);
  });

  test('the header is a hairline bar with no shadow, and Log in goes to the login page', async ({
    page,
  }) => {
    await mockApi(page, {
      'GET /users/me': {
        status: 401,
        body: {
          error: { code: 'UNAUTHENTICATED', message: 'x', details: [] },
          meta: { requestId: 'r' },
        },
      },
    });
    await page.goto('/track');
    const header = page.locator('header').first();
    await expect(header).toHaveCSS('border-bottom-width', '1px');
    await expect(header).toHaveCSS('box-shadow', 'none');
    await page.getByRole('link', { name: 'Log in' }).click();
    await expect(page).toHaveURL(/\/login/);
  });

  test('on a laptop the content is a reading column, on a phone it fills the width', async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto('/track');
    const wide = await page.locator('main#main').boundingBox();
    expect(wide!.width).toBeLessThanOrEqual(640);

    await page.setViewportSize({ width: 360, height: 800 });
    const narrow = await page.locator('main#main').boundingBox();
    expect(narrow!.width).toBe(360);
    const overflow = await page.evaluate(
      'document.documentElement.scrollWidth - document.documentElement.clientWidth',
    );
    expect(overflow).toBe(0);
  });

  test('the theme switch works here too', async ({ page }) => {
    await page.goto('/track');
    const before = await computed(page.locator('body'), 'backgroundColor');
    await page.getByRole('button', { name: 'Dark' }).click();
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
    await expect(page.locator('body')).not.toHaveCSS(
      'background-color',
      before,
    );
  });
});
