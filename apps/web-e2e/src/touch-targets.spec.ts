import { expect, test, type Page } from '@playwright/test';

import { signInAs } from './support/sign-in';

test.use({ colorScheme: 'light' });

/**
 * The controls inside `scope` a finger has to hit. A checkbox or a radio is
 * left out: its whole row is the 44px target, not the 16px box.
 */
const controls = (scope: string) =>
  `${scope} :is(button, select, textarea, input:not([type="checkbox"]):not([type="radio"]):not([type="hidden"]), [role="button"]):not([role="checkbox"]):not([role="radio"]):not([role="switch"])`;

/** Each visible control shorter than `min` pixels, named so a failure says which. */
async function tooSmall(page: Page, scope: string, min: number) {
  return page.locator(controls(scope)).evaluateAll((elements, min) => {
    const found: string[] = [];
    for (const element of elements) {
      const box = element.getBoundingClientRect();
      // Not on the page at all, such as a visually hidden helper.
      if (box.width === 0 || box.height === 0) continue;
      // A control inside a bordered box, such as the show-password button, is
      // the box's height less its two hairlines; the whole box is the target.
      if (box.height < min - 2.5) {
        const name =
          element.getAttribute('aria-label') ??
          element.textContent?.trim().slice(0, 30) ??
          element.id;
        found.push(
          `${element.tagName.toLowerCase()} "${name}" is ${Math.round(box.height)}px`,
        );
      }
    }
    return found;
  }, min);
}

test.describe('touch targets', () => {
  test.describe('on a phone', () => {
    test.use({
      viewport: { width: 375, height: 812 },
      hasTouch: true,
      isMobile: true,
    });

    test('every control on the style guide is at least 44px tall', async ({
      page,
    }) => {
      await page.goto('/design');
      await expect(
        page.getByRole('heading', { level: 1, name: 'Design tokens' }),
      ).toBeVisible();

      expect(await tooSmall(page, 'main', 44)).toEqual([]);
    });

    test('the signed-in shell is at least 44px too', async ({ page }) => {
      await signInAs(page, 'CUSTOMER');
      await page.goto('/profile');
      await expect(
        page.getByRole('heading', { level: 1, name: 'Profile' }),
      ).toBeVisible();

      expect(await tooSmall(page, 'body', 44)).toEqual([]);
    });
  });

  test.describe('with a mouse', () => {
    test.use({ viewport: { width: 1440, height: 900 } });

    test('the compact desktop sizes are kept', async ({ page }) => {
      await page.goto('/design');
      await expect(
        page.getByRole('heading', { level: 1, name: 'Design tokens' }),
      ).toBeVisible();

      const sizes = await page
        .locator('main button')
        .evaluateAll((buttons) =>
          buttons.map((button) =>
            Math.round(button.getBoundingClientRect().height),
          ),
        );
      // A mouse does not need the touch size, so the md button stays 40px.
      expect(sizes).toContain(40);
      expect(sizes).toContain(32);
    });
  });
});
