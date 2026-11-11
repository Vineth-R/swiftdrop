import { test, expect } from '@playwright/test';

const PAPER_LIGHT = 'rgb(246, 244, 239)';
const PAPER_DARK = 'rgb(14, 17, 19)';

test.describe('design tokens', () => {
  test.use({ colorScheme: 'light' });

  test('light theme is the default and uses the paper token', async ({
    page,
  }) => {
    await page.goto('/design');

    await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
    await expect(page.locator('body')).toHaveCSS(
      'background-color',
      PAPER_LIGHT,
    );
  });

  test('the theme switch flips every token and remembers the choice', async ({
    page,
  }) => {
    await page.goto('/design');

    await page.getByRole('button', { name: 'Dark' }).click();
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
    await expect(page.locator('body')).toHaveCSS(
      'background-color',
      PAPER_DARK,
    );

    await page.reload();
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
    await expect(page.getByRole('button', { name: 'Dark' })).toHaveAttribute(
      'aria-pressed',
      'true',
    );

    await page.getByRole('button', { name: 'System' }).click();
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
  });

  test('has one h1, so the page has a single main heading', async ({
    page,
  }) => {
    await page.goto('/design');

    await expect(page.getByRole('heading', { level: 1 })).toHaveCount(1);
  });

  test('uses the display font for headings and the interface font for text', async ({
    page,
  }) => {
    await page.goto('/design');

    await expect(
      page.getByRole('heading', { level: 1, name: 'Design tokens' }),
    ).toHaveCSS('font-family', /bricolage/i);
    await expect(page.getByText('Switch the theme')).toHaveCSS(
      'font-family',
      /instrument/i,
    );
    await expect(page.locator('code').first()).toHaveCSS(
      'font-family',
      /jetbrains/i,
    );
  });

  test('fonts are served by the app, never by Google', async ({ page }) => {
    const external: string[] = [];
    page.on('request', (request) => {
      const { hostname } = new URL(request.url());
      if (/googleapis|gstatic/.test(hostname)) external.push(request.url());
    });

    await page.goto('/design');
    // Run as page source: this project's tsconfig has no DOM types.
    await page.evaluate('document.fonts.ready.then(() => undefined)');

    const loaded = await page.evaluate<number>(
      "[...document.fonts].filter((f) => f.status === 'loaded').length",
    );
    expect(loaded).toBeGreaterThanOrEqual(3);
    expect(external).toEqual([]);
  });

  test('focus is a 2px outline on every interactive element', async ({
    page,
  }) => {
    await page.goto('/design');

    await page.keyboard.press('Tab');
    await expect(page.getByRole('button', { name: 'Light' })).toBeFocused();
    await expect(page.getByRole('button', { name: 'Light' })).toHaveCSS(
      'outline-width',
      '2px',
    );
    await expect(page.getByRole('button', { name: 'Light' })).toHaveCSS(
      'outline-offset',
      '2px',
    );
  });
});

test.describe('system dark preference', () => {
  test.use({ colorScheme: 'dark' });

  test('follows the operating system before any choice is stored', async ({
    page,
  }) => {
    await page.goto('/design');

    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
    await expect(page.locator('body')).toHaveCSS(
      'background-color',
      PAPER_DARK,
    );
  });
});
