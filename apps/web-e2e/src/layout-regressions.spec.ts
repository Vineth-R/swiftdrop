import { expect, test, type Page } from '@playwright/test';

import { type Reply } from './support/mock-api';
import { signInAs } from './support/sign-in';

test.use({ colorScheme: 'light' });

/**
 * Four layout defects the bug bash found with the real stack. Each test fails
 * without its fix:
 *  - a wide table made the whole page scroll sideways between 768px and about
 *    1100px (the hidden header text of a table was positioned against the page
 *    instead of the table's own scrolling box);
 *  - the user search box shrank to about 26px wide on a phone;
 *  - the unknown-address page kept the site's default title;
 *  - the business name in a merchant card was a 23px target.
 */

function list(items: unknown[]): Reply {
  return {
    status: 200,
    body: {
      data: items,
      meta: {
        requestId: 'req-1',
        pagination: {
          page: 1,
          pageSize: 20,
          totalItems: items.length,
          totalPages: 1,
        },
      },
    },
  };
}

/** A merchant whose contact address is long, as real ones often are. */
const merchant = (n: number, businessName: string) => ({
  id: `3f1c7a52-0000-4000-8000-0000000000a${n}`,
  ownerUserId: `3f1c7a52-0000-4000-8000-0000000000b${n}`,
  businessName,
  registrationNumber: `PV 12345${n}`,
  businessAddress: '88 Galle Road, Colombo 03',
  contactEmail: `owner.of.${businessName.toLowerCase().replace(/\W+/g, '.')}@example.com`,
  contactPhone: '+94112223344',
  pickupAddress: '88 Galle Road, Colombo 03',
  pickupZoneId: '7a1e0000-0000-4000-8000-000000000001',
  status: 'APPROVED' as const,
  createdAt: '2026-11-04T08:00:00Z',
});

const merchants = [
  merchant(1, 'ShopZone Lanka'),
  merchant(2, 'Ceylon Spice Traders'),
  merchant(3, 'Island Fresh Produce'),
];

/** How far the page itself can be scrolled sideways. It must be 0. */
const sidewaysOverflow = (page: Page) =>
  page.evaluate<number>(
    'document.documentElement.scrollWidth - document.documentElement.clientWidth',
  );

test.describe('a wide table', () => {
  for (const viewport of [
    { width: 768, height: 1024 },
    { width: 1024, height: 768 },
  ]) {
    test(`scrolls inside its own box at ${viewport.width}px and never moves the page`, async ({
      page,
    }) => {
      await page.setViewportSize(viewport);
      await signInAs(page, 'ADMIN', {
        'GET /admin/merchants': list(merchants),
      });
      await page.goto('/admin/merchants');
      await expect(
        page.getByRole('row').filter({ hasText: 'ShopZone Lanka' }),
      ).toBeVisible();

      // The table really is wider than its box here, so the check below means something.
      const box = page.locator('table').locator('xpath=..');
      expect(
        await box.evaluate((el) => el.scrollWidth - el.clientWidth),
      ).toBeGreaterThan(0);

      expect(await sidewaysOverflow(page)).toBe(0);
    });
  }
});

test.describe('the users filter row', () => {
  for (const width of [360, 768]) {
    test(`keeps a usable search box at ${width}px`, async ({ page }) => {
      await page.setViewportSize({ width, height: 900 });
      await signInAs(page, 'ADMIN', {
        'GET /admin/users/': list([
          {
            id: 'u1',
            email: 'kasun@example.com',
            fullName: 'Kasun Silva',
            phone: '',
            role: 'COURIER',
            isActive: true,
            createdAt: '2026-10-01T08:00:00Z',
          },
        ]),
      });
      await page.goto('/admin/users');
      await expect(page.locator('#user-search')).toBeVisible();

      const search = await page.locator('#user-search').boundingBox();
      const role = await page.locator('#user-role').boundingBox();
      expect(search).not.toBeNull();
      expect(role).not.toBeNull();
      // Wide enough to type a name or an email address.
      expect(search!.width).toBeGreaterThanOrEqual(200);
      // And it does not sit on top of the Role filter.
      const apart =
        search!.x + search!.width <= role!.x + 1 ||
        role!.x + role!.width <= search!.x + 1 ||
        search!.y + search!.height <= role!.y + 1 ||
        role!.y + role!.height <= search!.y + 1;
      expect(apart).toBe(true);
      expect(await sidewaysOverflow(page)).toBe(0);
    });
  }
});

test('an unknown address says so in the tab title', async ({ page }) => {
  await signInAs(page, 'CUSTOMER');
  await page.goto('/this-page-does-not-exist');
  await expect(
    page.getByRole('heading', { name: 'This page does not exist' }),
  ).toBeVisible();
  await expect(page).toHaveTitle('Page not found — SwiftDrop');
});

test.describe('on a phone', () => {
  test.use({
    viewport: { width: 375, height: 812 },
    hasTouch: true,
    isMobile: true,
  });

  test('the business name in a merchant card is a 44px target', async ({
    page,
  }) => {
    await signInAs(page, 'ADMIN', {
      'GET /admin/merchants': list(merchants),
    });
    await page.goto('/admin/merchants');
    const name = page.getByRole('button', { name: 'ShopZone Lanka' });
    await expect(name).toBeVisible();

    const box = await name.boundingBox();
    expect(box).not.toBeNull();
    expect(box!.height).toBeGreaterThanOrEqual(44 - 1);
  });
});
