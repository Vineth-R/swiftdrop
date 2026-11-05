import { expect, test } from '@playwright/test';

import { fail, mockApi, ok, SERVER_WORDS } from './support/mock-api';

const NUMBER = 'SDK7M2Q9XP4';
const KEY = `GET /tracking/${NUMBER}`;

const tracking = {
  trackingNumber: NUMBER,
  status: 'IN_TRANSIT',
  pickupCity: 'Colombo',
  dropoffCity: 'Kandy',
  lastUpdatedAt: '2026-10-31T01:10:00Z',
  timeline: [
    { status: 'CREATED', at: '2026-10-30T03:44:00Z' },
    { status: 'PICKED_UP', at: '2026-10-30T08:32:00Z' },
    { status: 'IN_TRANSIT', at: '2026-10-31T01:10:00Z' },
  ],
};

const field = (page: import('@playwright/test').Page) =>
  page.getByRole('textbox');
const track = (page: import('@playwright/test').Page) =>
  page.getByRole('button', { name: 'Track', exact: true });

test.describe('public tracking at /track', () => {
  test('a first visit shows an empty form and calls nothing', async ({
    page,
  }) => {
    const { calls } = await mockApi(page, {});
    await page.goto('/track');
    await expect(page.getByLabel('Tracking number')).toHaveValue('');
    await expect(track(page)).toBeVisible();
    expect(calls).toEqual([]);
  });

  test('a found parcel shows its status, cities, times and history', async ({
    page,
  }) => {
    await mockApi(page, { [KEY]: ok(tracking) });
    await page.goto('/track');
    await field(page).fill(NUMBER);
    await track(page).click();

    await expect(page).toHaveURL(`/track?number=${NUMBER}`);
    await expect(page.getByText('IN TRANSIT', { exact: true })).toBeVisible();
    await expect(page.getByText('Colombo', { exact: true })).toBeVisible();
    await expect(page.getByText('Kandy', { exact: true })).toBeVisible();
    await expect(page.getByLabel('Track another parcel')).toHaveValue(NUMBER);

    const history = page.getByRole('list', { name: 'Status history' });
    await expect(history.getByRole('listitem')).toHaveCount(5);
    await expect(history.locator('[aria-current="step"]')).toContainText(
      'In transit',
    );
    await expect(history.getByText('Not yet reached')).toHaveCount(1);
    await expect(history.getByText('Final step')).toHaveCount(1);
    await expect(
      page.getByRole('heading', { level: 1, name: `Parcel ${NUMBER}` }),
    ).toBeVisible();
  });

  test('a delivered parcel has nothing still to come', async ({ page }) => {
    await mockApi(page, { [KEY]: ok({ ...tracking, status: 'DELIVERED' }) });
    await page.goto(`/track?number=${NUMBER}`);
    await expect(page.getByText('DELIVERED', { exact: true })).toBeVisible();
    await expect(page.getByText('Not yet reached')).toHaveCount(0);
    await expect(page.getByText('Final step')).toHaveCount(0);
  });

  test('a failed parcel has nothing still to come', async ({ page }) => {
    await mockApi(page, {
      [KEY]: ok({
        ...tracking,
        status: 'FAILED',
        timeline: [
          ...tracking.timeline,
          { status: 'FAILED', at: '2026-10-31T09:00:00Z' },
        ],
      }),
    });
    await page.goto(`/track?number=${NUMBER}`);
    await expect(page.getByText('Delivery failed')).toBeVisible();
    await expect(page.getByText('Not yet reached')).toHaveCount(0);
  });

  test('a shared link opens the parcel, and survives a refresh', async ({
    page,
  }) => {
    await mockApi(page, { [KEY]: ok(tracking) });
    await page.goto(`/track?number=${NUMBER}`);
    await expect(page.getByText('IN TRANSIT', { exact: true })).toBeVisible();
    await page.reload();
    await expect(page.getByText('IN TRANSIT', { exact: true })).toBeVisible();
    await expect(field(page)).toHaveValue(NUMBER);
  });

  test('lowercase and spaces are cleaned up, in the form and in a link', async ({
    page,
  }) => {
    const { calls } = await mockApi(page, { [KEY]: ok(tracking) });
    await page.goto('/track');
    await field(page).fill('  sdk7m2q9xp4 ');
    await track(page).click();
    await expect(page).toHaveURL(`/track?number=${NUMBER}`);
    await expect(field(page)).toHaveValue(NUMBER);
    await expect(page.getByText('IN TRANSIT', { exact: true })).toBeVisible();

    await page.goto('/track?number=sdk7m2q9xp4');
    await expect(page.getByText('IN TRANSIT', { exact: true })).toBeVisible();
    expect(new Set(calls.map((call) => call.key))).toEqual(new Set([KEY]));
  });

  test('loading shows a skeleton and blocks a second search', async ({
    page,
  }) => {
    await mockApi(page, { [KEY]: { ...ok(tracking), delayMs: 1500 } });
    await page.goto(`/track?number=${NUMBER}`);
    await expect(page.getByText('Loading parcel details')).toBeAttached();
    await expect(page.getByRole('button', { name: 'Tracking' })).toBeDisabled();
    await expect(page.getByText('IN TRANSIT', { exact: true })).toBeVisible();
  });

  test('an unknown number says so', async ({ page }) => {
    await mockApi(page, { [KEY]: fail(404, 'TRACKING_NOT_FOUND') });
    await page.goto(`/track?number=${NUMBER}`);
    await expect(
      page.getByRole('heading', { name: 'No parcel with that number' }),
    ).toBeVisible();
    await expect(page.locator('body')).not.toContainText(SERVER_WORDS);
    await expect(page.getByText('Reference')).toHaveCount(0);

    await page.getByRole('link', { name: 'Clear search' }).click();
    await expect(page).toHaveURL(/\/track$/);
    await expect(field(page)).toHaveValue('');
  });

  for (const [name, typed, message] of [
    ['an empty number', '', 'Enter a tracking number.'],
    ['a wrong prefix', 'ABC7M2Q9XP4', 'Check the number.'],
    ['a short number', 'SDK123', 'Check the number.'],
    ['a symbol', 'SDK7M2Q9X-4', 'Check the number.'],
  ]) {
    test(`${name} is refused beside the field, with no call`, async ({
      page,
    }) => {
      const { calls } = await mockApi(page, {});
      await page.goto('/track');
      await field(page).fill(typed);
      await track(page).click();
      await expect(page.getByText(message)).toBeVisible();
      await expect(field(page)).toBeFocused();
      await expect(field(page)).toHaveValue(typed);
      expect(calls).toEqual([]);
    });
  }

  test('a bad number in a link is refused with no call', async ({ page }) => {
    const { calls } = await mockApi(page, {});
    await page.goto('/track?number=nonsense');
    await expect(page.getByText('Check the number.')).toBeVisible();
    expect(calls).toEqual([]);
  });

  test('a 422 from the service lands beside the field', async ({ page }) => {
    await mockApi(page, { [KEY]: fail(422, 'VALIDATION_ERROR') });
    await page.goto(`/track?number=${NUMBER}`);
    await expect(page.getByText('Check the number.')).toBeVisible();
    await expect(page.locator('body')).not.toContainText(SERVER_WORDS);
  });

  test('too many requests shows the wait and holds the button', async ({
    page,
  }) => {
    await mockApi(page, {
      [KEY]: fail(429, 'RATE_LIMITED', { headers: { 'Retry-After': '30' } }),
    });
    await page.goto(`/track?number=${NUMBER}`);
    await expect(
      page.getByText('Too many attempts from this device'),
    ).toBeVisible();
    await expect(track(page)).toBeDisabled();
    await expect(field(page)).toHaveValue(NUMBER);
  });

  test('a fault on our side shows a reference, and the same number can be tried again', async ({
    page,
  }) => {
    let isDown = true;
    await mockApi(page, {
      [KEY]: () => (isDown ? fail(500, 'INTERNAL_ERROR') : ok(tracking)),
    });
    await page.goto(`/track?number=${NUMBER}`);
    await expect(page.getByText('Something broke on our side')).toBeVisible();
    await expect(page.getByText('Reference')).toBeVisible();
    await expect(page.locator('body')).not.toContainText(SERVER_WORDS);

    isDown = false;
    await track(page).click();
    await expect(page.getByText('IN TRANSIT', { exact: true })).toBeVisible();
    await expect(page.getByText('Something broke on our side')).toHaveCount(0);
  });

  test('no connection says so', async ({ page }) => {
    await page.route(`**/api/v1/tracking/**`, (route) => route.abort());
    await page.goto(`/track?number=${NUMBER}`);
    await expect(page.getByText('We could not reach SwiftDrop')).toBeVisible();
  });

  test('the back button returns to the earlier parcel', async ({ page }) => {
    const other = 'SDN1A2B3C4D';
    await mockApi(page, {
      [KEY]: ok(tracking),
      [`GET /tracking/${other}`]: fail(404, 'TRACKING_NOT_FOUND'),
    });
    await page.goto(`/track?number=${NUMBER}`);
    await expect(page.getByText('IN TRANSIT', { exact: true })).toBeVisible();
    await field(page).fill(other);
    await track(page).click();
    await expect(page.getByText('No parcel with that number')).toBeVisible();
    await page.goBack();
    await expect(field(page)).toHaveValue(NUMBER);
    await expect(page.getByText('IN TRANSIT', { exact: true })).toBeVisible();
  });

  test('Refresh looks up the parcel again and keeps it on screen', async ({
    page,
  }) => {
    let status = 'IN_TRANSIT';
    const { calls } = await mockApi(page, {
      [KEY]: () => ({ ...ok({ ...tracking, status }), delayMs: 300 }),
    });
    await page.goto(`/track?number=${NUMBER}`);
    await expect(page.getByText('IN TRANSIT', { exact: true })).toBeVisible();

    status = 'OUT_FOR_DELIVERY';
    await page.getByRole('button', { name: 'Refresh' }).click();
    await expect(
      page.getByRole('button', { name: 'Refreshing' }),
    ).toBeVisible();
    await expect(page.getByText('IN TRANSIT', { exact: true })).toBeVisible();
    await expect(
      page.getByText('OUT FOR DELIVERY', { exact: true }),
    ).toBeVisible();
    expect(calls.length).toBeGreaterThanOrEqual(2);
  });

  test('Copy number copies it and says so', async ({ page, context }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write']);
    await mockApi(page, { [KEY]: ok(tracking) });
    await page.goto(`/track?number=${NUMBER}`);
    await page.getByRole('button', { name: 'Copy number' }).click();
    await expect(page.getByText('Tracking number copied.')).toBeVisible();
    expect(await page.evaluate('navigator.clipboard.readText()')).toBe(NUMBER);
  });

  test('it never shows a name, phone, address or fee, even if the API sends one', async ({
    page,
  }) => {
    await mockApi(page, {
      [KEY]: ok({
        ...tracking,
        senderName: 'Nimal Perera',
        recipientName: 'Amali Fernando',
        recipientPhone: '+94771234567',
        dropoffAddress: '45 Lake Drive',
        priceAmount: 987.65,
        timeline: tracking.timeline.map((event) => ({
          ...event,
          note: 'Handed to Nimal Perera',
          city: 'Kegalle',
        })),
      }),
    });
    await page.goto(`/track?number=${NUMBER}`);
    await expect(page.getByText('IN TRANSIT', { exact: true })).toBeVisible();
    await expect(page.locator('body')).not.toContainText(
      /Nimal|Amali|\+94|Lake Drive|987\.65|Kegalle/,
    );
  });

  test('the form fills the width on a phone', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 800 });
    await page.goto('/track');
    const box = await field(page).boundingBox();
    expect(box?.width).toBeGreaterThan(320);
  });

  test('it works with the keyboard alone', async ({ page }) => {
    await mockApi(page, { [KEY]: ok(tracking) });
    await page.goto('/track');
    await field(page).focus();
    await page.keyboard.type(NUMBER);
    await page.keyboard.press('Enter');
    await expect(page.getByText('IN TRANSIT', { exact: true })).toBeVisible();
  });

  for (const width of [375, 1280]) {
    for (const theme of ['Light', 'Dark']) {
      test(`nothing scrolls sideways at ${width}px in ${theme.toLowerCase()} mode`, async ({
        page,
      }) => {
        await page.setViewportSize({ width, height: 800 });
        await mockApi(page, { [KEY]: ok(tracking) });
        await page.goto(`/track?number=${NUMBER}`);
        await page.getByRole('button', { name: theme }).click();
        await expect(
          page.getByText('IN TRANSIT', { exact: true }),
        ).toBeVisible();
        const overflow = await page.evaluate(
          'document.documentElement.scrollWidth - document.documentElement.clientWidth',
        );
        expect(overflow).toBe(0);
      });
    }
  }
});
