import { expect, test, type Page, type WebSocketRoute } from '@playwright/test';

import { mockApi, ok } from './support/mock-api';

/**
 * Live updates on /track, with the real hook and a fake realtime server
 * (API v1.1 section 9). No backend is needed.
 */

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

const update = (changes: Record<string, unknown> = {}) =>
  JSON.stringify({
    type: 'parcel.status.updated',
    data: {
      trackingNumber: NUMBER,
      previousStatus: 'IN_TRANSIT',
      status: 'OUT_FOR_DELIVERY',
      updatedAt: '2026-10-31T09:15:00Z',
      ...changes,
    },
  });

/** A realtime server that confirms each subscription and hands back the socket. */
async function liveServer(page: Page) {
  const subscribed: string[] = [];
  let connections = 0;
  let socket: WebSocketRoute | undefined;
  await page.routeWebSocket('**/ws', (ws) => {
    connections += 1;
    socket = ws;
    ws.onMessage((message) => {
      const data = JSON.parse(String(message)) as {
        type: string;
        trackingNumber?: string;
      };
      if (data.type === 'subscribe' && data.trackingNumber) {
        subscribed.push(data.trackingNumber);
        ws.send(
          JSON.stringify({
            type: 'subscribed',
            trackingNumber: data.trackingNumber,
          }),
        );
      }
    });
  });
  return {
    subscribed,
    connections: () => connections,
    send: (message: string) => socket?.send(message),
    close: (code: number) => socket?.close({ code }),
  };
}

const live = (page: Page) =>
  page.getByText('Live. Changes appear here as they happen.');

test.describe('live updates on /track', () => {
  test('a status change appears without a refresh', async ({ page }) => {
    await mockApi(page, { [KEY]: ok(tracking) });
    const server = await liveServer(page);
    await page.goto(`/track?number=${NUMBER}`);

    await expect(live(page)).toBeVisible();
    expect(server.subscribed).toContain(NUMBER);

    server.send(update());
    await expect(
      page.getByText('OUT FOR DELIVERY', { exact: true }),
    ).toBeVisible();
    const history = page.getByRole('list', { name: 'Status history' });
    await expect(history.locator('[aria-current="step"]')).toContainText(
      'Out for delivery',
    );
    await expect(history.getByText('Not yet reached')).toHaveCount(0);
  });

  test('loads the parcel again once subscribed, in case a change was missed', async ({
    page,
  }) => {
    let calls = 0;
    await mockApi(page, {
      [KEY]: () => {
        calls += 1;
        return ok(
          calls === 1 ? tracking : { ...tracking, status: 'DELIVERED' },
        );
      },
    });
    await liveServer(page);
    await page.goto(`/track?number=${NUMBER}`);
    await expect(page.getByText('DELIVERED', { exact: true })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Refreshing' })).toHaveCount(
      0,
    );
  });

  test('an update for another parcel, a repeat or an older one changes nothing', async ({
    page,
  }) => {
    await mockApi(page, { [KEY]: ok(tracking) });
    const server = await liveServer(page);
    await page.goto(`/track?number=${NUMBER}`);
    await expect(live(page)).toBeVisible();

    // The other parcel gets a status and a time that would win if it leaked:
    // DELIVERED, later than the real update below. Had it been applied, the
    // real update would count as older and OUT FOR DELIVERY would never show.
    server.send(
      update({
        trackingNumber: 'SDN1A2B3C4D',
        status: 'DELIVERED',
        updatedAt: '2026-10-31T23:00:00Z',
      }),
    );
    server.send(
      update({ status: 'PICKED_UP', updatedAt: '2026-10-30T00:00:00Z' }),
    );
    server.send(
      update({ status: 'IN_TRANSIT', updatedAt: '2026-10-31T01:10:00Z' }),
    );
    // A real update last: once it shows, the three before it were handled.
    server.send(update());
    await expect(
      page.getByText('OUT FOR DELIVERY', { exact: true }),
    ).toBeVisible();
    const history = page.getByRole('list', { name: 'Status history' });
    await expect(history.getByRole('listitem')).toHaveCount(5);
    await expect(history.getByText('Delivered', { exact: true })).toHaveCount(
      1,
    );
    await expect(history.locator('[aria-current="step"]')).toContainText(
      'Out for delivery',
    );
  });

  test('a newer update with the same status moves the last scan but adds no history step', async ({
    page,
  }) => {
    await mockApi(page, { [KEY]: ok(tracking) });
    const server = await liveServer(page);
    await page.goto(`/track?number=${NUMBER}`);
    await expect(live(page)).toBeVisible();

    server.send(
      update({
        previousStatus: 'PICKED_UP',
        status: 'IN_TRANSIT',
        updatedAt: '2026-10-31T02:00:00Z',
      }),
    );
    // A real change last: once it shows, the same-status update was handled.
    server.send(update());
    await expect(
      page.getByText('OUT FOR DELIVERY', { exact: true }),
    ).toBeVisible();
    // Three reached steps, one new step and the final "Delivered" step. A
    // second "In transit" would make six.
    await expect(
      page.getByRole('list', { name: 'Status history' }).getByRole('listitem'),
    ).toHaveCount(5);
  });

  test('a snapshot that arrives after a newer live update does not overwrite it', async ({
    page,
  }) => {
    let loads = 0;
    await mockApi(page, {
      [KEY]: () => {
        loads += 1;
        // The load after subscribing is slow and returns the old state.
        return loads === 1 ? ok(tracking) : { ...ok(tracking), delayMs: 1500 };
      },
    });
    const secondLoad = new Promise<void>((resolve) => {
      let responses = 0;
      page.on('response', (response) => {
        if (response.url().includes(`/tracking/${NUMBER}`)) {
          responses += 1;
          if (responses === 2) resolve();
        }
      });
    });
    const server = await liveServer(page);
    await page.goto(`/track?number=${NUMBER}`);
    await expect(live(page)).toBeVisible();

    server.send(update());
    await expect(
      page.getByText('OUT FOR DELIVERY', { exact: true }),
    ).toBeVisible();

    await secondLoad; // the old snapshot has now come back
    // Two frames give React time to apply it, if it were going to. The e2e
    // project has no DOM types, so the page code is passed as a string.
    await page.evaluate(
      'new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)))',
    );
    await expect(
      page.getByText('OUT FOR DELIVERY', { exact: true }),
    ).toBeVisible();
    await expect(page.getByText('IN TRANSIT', { exact: true })).toHaveCount(0);
  });

  test('private text in a socket message never appears', async ({ page }) => {
    await mockApi(page, { [KEY]: ok(tracking) });
    const server = await liveServer(page);
    await page.goto(`/track?number=${NUMBER}`);
    await expect(live(page)).toBeVisible();

    server.send(
      JSON.stringify({
        type: 'parcel.status.updated',
        data: {
          trackingNumber: NUMBER,
          previousStatus: 'IN_TRANSIT',
          status: 'OUT_FOR_DELIVERY',
          updatedAt: '2026-10-31T09:15:00Z',
          recipientName: 'Amali Fernando',
          recipientPhone: '+94771234567',
          note: 'Left at 45 Lake Drive',
        },
      }),
    );
    await expect(
      page.getByText('OUT FOR DELIVERY', { exact: true }),
    ).toBeVisible();
    await expect(page.locator('body')).not.toContainText(
      /Amali|\+94|Lake Drive/,
    );
  });

  test('when the connection drops, it says so and Refresh still works', async ({
    page,
  }) => {
    await mockApi(page, { [KEY]: ok(tracking) });
    const server = await liveServer(page);
    await page.goto(`/track?number=${NUMBER}`);
    await expect(live(page)).toBeVisible();

    server.close(1011);
    await expect(
      page.getByText('Connecting to live updates. Use Refresh to check now.'),
    ).toBeVisible();
    await page.getByRole('button', { name: 'Refresh' }).click();
    await expect(page.getByRole('button', { name: 'Refresh' })).toBeEnabled();
  });

  test('when the server refuses, live updates are off for good', async ({
    page,
  }) => {
    await mockApi(page, { [KEY]: ok(tracking) });
    const server = await liveServer(page);
    await page.goto(`/track?number=${NUMBER}`);
    await expect(live(page)).toBeVisible();

    server.close(4403);
    await expect(
      page.getByText('Live updates are off. Use Refresh to check for changes.'),
    ).toBeVisible();
  });

  test('nothing connects until a parcel is found', async ({ page }) => {
    await mockApi(page, { [KEY]: ok(tracking) });
    const server = await liveServer(page);
    await page.goto('/track');
    await page.getByLabel('Tracking number').fill('nonsense');
    await page.getByRole('button', { name: 'Track', exact: true }).click();
    await expect(page.getByText('Check the number.')).toBeVisible();
    expect(server.connections()).toBe(0);

    await page.getByLabel('Tracking number').fill(NUMBER);
    await page.getByRole('button', { name: 'Track', exact: true }).click();
    await expect(live(page)).toBeVisible();
    expect(server.connections()).toBe(1);
    expect(server.subscribed).toEqual([NUMBER]);
  });
});
