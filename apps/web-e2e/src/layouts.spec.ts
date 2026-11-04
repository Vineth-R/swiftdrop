import { test, expect, type Page } from '@playwright/test';

import { mockApi, ok, signedOut, type Handler } from './support/mock-api';
import { signInAs, type Role } from './support/sign-in';

test.use({ colorScheme: 'light' });

/**
 * Every screen at the two widths the team sheet names: a small phone (360px)
 * and a laptop (1280px). Each must show its one h1, not scroll sideways and
 * keep every control inside the viewport.
 */
const sizes = [
  { name: '360px', width: 360, height: 800 },
  { name: '1280px', width: 1280, height: 800 },
];

type Screen = {
  name: string;
  path: string;
  as?: Role;
  extra?: Record<string, Handler>;
};

const screens: Screen[] = [
  { name: 'log in', path: '/login' },
  { name: 'register', path: '/register' },
  { name: 'forgot password', path: '/forgot-password' },
  { name: 'reset password', path: '/reset-password?token=abc' },
  { name: 'signed out', path: '/signed-out' },
  { name: 'customer dashboard', path: '/dashboard', as: 'CUSTOMER' },
  { name: 'customer profile', path: '/profile', as: 'CUSTOMER' },
  { name: 'customer security', path: '/profile/security', as: 'CUSTOMER' },
  { name: 'merchant dashboard', path: '/dashboard', as: 'MERCHANT' },
  { name: 'dispatcher dashboard', path: '/dashboard', as: 'DISPATCHER' },
  { name: 'admin dashboard', path: '/dashboard', as: 'ADMIN' },
  {
    name: 'admin notification logs',
    path: '/admin/notification-logs',
    as: 'ADMIN',
    extra: { 'GET /admin/notification-logs': ok([]) },
  },
  { name: 'courier dashboard', path: '/dashboard', as: 'COURIER' },
  { name: 'courier profile', path: '/profile', as: 'COURIER' },
  {
    name: 'a page that is not built yet (404 inside the shell)',
    path: '/no-such-page',
    as: 'CUSTOMER',
  },
  {
    name: 'a page the role may not open (403 inside the shell)',
    path: '/admin/users',
    as: 'CUSTOMER',
  },
  { name: 'public tracking page', path: '/track' },
  { name: 'style guide', path: '/design' },
];

async function openScreen(page: Page, screen: Screen) {
  if (screen.as) {
    await signInAs(page, screen.as, screen.extra);
    return;
  }
  await mockApi(page, signedOut);
}

async function expectFits(page: Page, label: string) {
  const overflow = await page.evaluate(
    'document.documentElement.scrollWidth - document.documentElement.clientWidth',
  );
  expect(overflow, `${label} scrolls sideways`).toBe(0);

  // A control that is pushed past an edge is clipped even when nothing scrolls.
  const outside = await page.evaluate(`(() => {
    const width = document.documentElement.clientWidth;
    return [...document.querySelectorAll('a, button, input, select, textarea')]
      .filter((element) => {
        const box = element.getBoundingClientRect();
        const style = getComputedStyle(element);
        const shown = style.display !== 'none' && style.visibility !== 'hidden';
        return shown && box.width > 2 && box.height > 2 && (box.left < -1 || box.right > width + 1);
      })
      .map((element) => (element.textContent || element.getAttribute('aria-label') || element.tagName).trim().slice(0, 40));
  })()`);
  expect(outside, `${label} has controls outside the viewport`).toEqual([]);
}

for (const size of sizes) {
  test.describe(`at ${size.name}`, () => {
    test.use({ viewport: { width: size.width, height: size.height } });

    for (const screen of screens) {
      test(`${screen.name} fits`, async ({ page }) => {
        await openScreen(page, screen);
        await page.goto(screen.path);
        // The style guide shows a system page, so it holds a second h1 as a sample.
        await expect(
          page.getByRole('heading', { level: 1 }).first(),
        ).toBeVisible();
        await expectFits(page, screen.name);
      });
    }
  });
}

test.describe('the layout check itself', () => {
  test.use({ viewport: { width: 360, height: 800 } });

  test('it reports a control pushed outside the viewport', async ({ page }) => {
    await mockApi(page, signedOut);
    await page.goto('/login');
    await expectFits(page, 'log in');

    await page.evaluate(`(() => {
      const button = document.createElement('button');
      button.textContent = 'Pushed out';
      button.style.position = 'fixed';
      button.style.left = '-200px';
      button.style.top = '10px';
      document.body.append(button);
    })()`);
    await expect(expectFits(page, 'log in')).rejects.toThrow(
      /outside the viewport/,
    );
  });

  test('it reports a page that scrolls sideways', async ({ page }) => {
    await mockApi(page, signedOut);
    await page.goto('/login');

    await page.evaluate(`(() => {
      const wide = document.createElement('div');
      wide.style.width = '2000px';
      wide.style.height = '1px';
      document.body.append(wide);
    })()`);
    await expect(expectFits(page, 'log in')).rejects.toThrow(
      /scrolls sideways/,
    );
  });
});
