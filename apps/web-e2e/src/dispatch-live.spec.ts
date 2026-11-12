import { expect, test, type Page, type WebSocketRoute } from '@playwright/test';

import { REQUEST_ID, type Handler, type Reply } from './support/mock-api';
import { signInAs } from './support/sign-in';

/**
 * Live updates on the dispatcher's All parcels page (/parcels), with the real
 * hook and a fake realtime server (API v1.1 section 9, scope "all"). No
 * backend is needed.
 */

test.use({ colorScheme: 'light' });

const parcel = (changes: Record<string, unknown> = {}) => ({
  id: '00000000-0000-4000-8000-000000000001',
  trackingNumber: 'SDK7M2Q9XP4',
  status: 'CREATED',
  pickupCity: 'Colombo',
  dropoffCity: 'Kandy',
  courierId: null,
  merchantId: null,
  customerId: '00000000-0000-4000-8000-0000000000c1',
  total: 450,
  createdAt: '2026-11-02T10:00:00Z',
  ...changes,
});

const FIRST = parcel();
const SECOND = parcel({
  id: '00000000-0000-4000-8000-000000000002',
  trackingNumber: 'SDK2B3C4D5E',
  status: 'PICKED_UP',
});

function pageOf(data: unknown[]): Reply {
  return {
    status: 200,
    body: {
      data,
      meta: {
        requestId: REQUEST_ID,
        pagination: {
          page: 1,
          pageSize: 20,
          totalItems: data.length,
          totalPages: 1,
        },
      },
    },
  };
}

const update = (changes: Record<string, unknown> = {}) =>
  JSON.stringify({
    type: 'parcel.status.updated',
    data: {
      trackingNumber: FIRST.trackingNumber,
      previousStatus: 'CREATED',
      status: 'OUT_FOR_DELIVERY',
      updatedAt: '2026-11-12T09:15:00Z',
      ...changes,
    },
  });

/**
 * A realtime server. It confirms the subscription only when the test says so,
 * after the table has loaded, so the order of events never depends on timing.
 */
async function liveServer(page: Page) {
  const subscriptions: unknown[] = [];
  let connections = 0;
  let socket: WebSocketRoute | undefined;
  let release: () => void = () => undefined;
  const released = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.routeWebSocket('**/ws', (ws) => {
    connections += 1;
    socket = ws;
    ws.onMessage(async (message) => {
      const data = JSON.parse(String(message)) as { type: string };
      if (data.type !== 'subscribe') return;
      subscriptions.push(data);
      await released;
      ws.send(JSON.stringify({ type: 'subscribed', scope: 'all' }));
    });
  });
  return {
    subscriptions,
    connections: () => connections,
    confirm: release,
    send: (message: string) => socket?.send(message),
    close: (code: number) => socket?.close({ code }),
  };
}

const table = (page: Page) => page.getByRole('table', { name: 'All parcels' });
const row = (page: Page, trackingNumber: string) =>
  table(page).getByRole('row').filter({ hasText: trackingNumber });
const live = (page: Page) =>
  page.getByText('Live. Parcel statuses change here as they happen.');

/** Opens /parcels as a dispatcher, waits for the table, then lets the socket go live. */
async function openLive(
  page: Page,
  handlers: Record<string, Handler> = {},
  beforeConfirm: () => void = () => undefined,
) {
  const server = await liveServer(page);
  const session = await signInAs(page, 'DISPATCHER', {
    'GET /admin/parcels': pageOf([FIRST, SECOND]),
    ...handlers,
  });
  await page.goto('/parcels');
  await expect(row(page, FIRST.trackingNumber)).toBeVisible();
  beforeConfirm();
  server.confirm();
  await expect(live(page)).toBeVisible();
  return { server, ...session };
}

test.describe('live updates on /parcels', () => {
  test.beforeEach(async ({ page }) => {
    test.setTimeout(60000);
    await page.setViewportSize({ width: 1440, height: 900 });
  });

  test('follows every parcel, not one', async ({ page }) => {
    const { server } = await openLive(page);
    expect(server.subscriptions).toEqual([{ type: 'subscribe', scope: 'all' }]);
  });

  test('a status change appears in its row without a reload', async ({
    page,
  }) => {
    const { server } = await openLive(page);
    await expect(row(page, FIRST.trackingNumber)).toContainText('CREATED');

    server.send(update());
    await expect(row(page, FIRST.trackingNumber)).toContainText(
      'OUT FOR DELIVERY',
    );
    // The other row is untouched.
    await expect(row(page, SECOND.trackingNumber)).toContainText('PICKED UP');
  });

  test('an update for a parcel that is not on the page adds no row', async ({
    page,
  }) => {
    const { server } = await openLive(page);

    server.send(update({ trackingNumber: 'SDN9Z8Y7X6W', status: 'DELIVERED' }));
    // A real update last: once it shows, the one before it was handled.
    server.send(update());
    await expect(row(page, FIRST.trackingNumber)).toContainText(
      'OUT FOR DELIVERY',
    );
    await expect(table(page)).not.toContainText('SDN9Z8Y7X6W');
    await expect(row(page, SECOND.trackingNumber)).toContainText('PICKED UP');
  });

  test('an older update does not undo a newer one', async ({ page }) => {
    const { server } = await openLive(page);

    server.send(update());
    await expect(row(page, FIRST.trackingNumber)).toContainText(
      'OUT FOR DELIVERY',
    );
    server.send(
      update({ status: 'PICKED_UP', updatedAt: '2026-11-12T07:00:00Z' }),
    );
    // A later change for the other parcel: once it shows, the older one was handled.
    server.send(
      update({
        trackingNumber: SECOND.trackingNumber,
        status: 'IN_TRANSIT',
        updatedAt: '2026-11-12T09:30:00Z',
      }),
    );
    await expect(row(page, SECOND.trackingNumber)).toContainText('IN TRANSIT');
    await expect(row(page, FIRST.trackingNumber)).toContainText(
      'OUT FOR DELIVERY',
    );
  });

  test('loads the page again once subscribed, in case a change was missed', async ({
    page,
  }) => {
    // The first answers are the old state. Only a load that starts after the
    // socket is confirmed gets the new one, so the page must ask again then.
    // (In dev, React may load once more on its own at start, so the number of
    // calls is not counted, only what the table ends up showing.)
    let subscribed = false;
    await openLive(
      page,
      {
        'GET /admin/parcels': () =>
          pageOf([
            subscribed ? parcel({ status: 'DELIVERED' }) : FIRST,
            SECOND,
          ]),
      },
      () => {
        subscribed = true;
      },
    );
    await expect(row(page, FIRST.trackingNumber)).toContainText('DELIVERED');
    // Quiet: the table is never swapped for a loading skeleton.
    await expect(table(page)).toBeVisible();
  });

  test('a page that loads after a newer live update does not overwrite it', async ({
    page,
  }) => {
    let loads = 0;
    let responses = 0;
    let answered: () => void = () => undefined;
    const secondAnswer = new Promise<void>((resolve) => {
      answered = resolve;
    });
    page.on('response', (response) => {
      if (response.url().includes('/admin/parcels')) {
        responses += 1;
        if (responses === 2) answered();
      }
    });
    const { server } = await openLive(page, {
      // The load after subscribing is slow and returns the old state.
      'GET /admin/parcels': () => {
        loads += 1;
        const reply = pageOf([FIRST, SECOND]);
        return loads === 1 ? reply : { ...reply, delayMs: 1500 };
      },
    });

    server.send(update());
    await expect(row(page, FIRST.trackingNumber)).toContainText(
      'OUT FOR DELIVERY',
    );

    await secondAnswer; // the old state has now come back
    // Two frames give React time to apply it, if it were going to. The e2e
    // project has no DOM types, so the page code is passed as a string.
    await page.evaluate(
      'new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)))',
    );
    await expect(row(page, FIRST.trackingNumber)).toContainText(
      'OUT FOR DELIVERY',
    );
  });

  test('a change that arrives while a reload fails stays on screen', async ({
    page,
  }) => {
    const server = await liveServer(page);
    let failing = false;
    await signInAs(page, 'DISPATCHER', {
      'GET /admin/parcels': () => {
        // The quiet reload after subscribing fails, after a delay. Every load
        // before that answers with the table, so React's extra dev render
        // cannot fail the first paint.
        return failing
          ? {
              status: 500,
              delayMs: 800,
              body: {
                error: { code: 'INTERNAL', message: 'reload failed' },
                meta: { requestId: REQUEST_ID },
              },
            }
          : pageOf([FIRST, SECOND]);
      },
    });
    await page.goto('/parcels');
    await expect(row(page, FIRST.trackingNumber)).toBeVisible();

    const syncRequest = page.waitForRequest((request) =>
      request.url().includes('/admin/parcels'),
    );
    const syncResponse = page.waitForResponse(
      (response) =>
        response.url().includes('/admin/parcels') && response.status() === 500,
    );
    failing = true;
    server.confirm();
    await syncRequest;
    server.send(update());
    await expect(row(page, FIRST.trackingNumber)).toContainText(
      'OUT FOR DELIVERY',
    );

    await syncResponse; // the failed reload has now come back
    await page.evaluate(
      'new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)))',
    );
    await expect(row(page, FIRST.trackingNumber)).toContainText(
      'OUT FOR DELIVERY',
    );
  });

  test('private text in a socket message never appears', async ({ page }) => {
    const { server } = await openLive(page);

    server.send(
      JSON.stringify({
        type: 'parcel.status.updated',
        data: {
          trackingNumber: FIRST.trackingNumber,
          previousStatus: 'CREATED',
          status: 'OUT_FOR_DELIVERY',
          updatedAt: '2026-11-12T09:15:00Z',
          recipientName: 'Amali Fernando',
          recipientPhone: '+94771234567',
          note: 'Left at 45 Lake Drive',
        },
      }),
    );
    await expect(row(page, FIRST.trackingNumber)).toContainText(
      'OUT FOR DELIVERY',
    );
    await expect(page.locator('body')).not.toContainText(
      /Amali|\+94|Lake Drive/,
    );
  });

  test('when the connection drops, it says so and the table stays', async ({
    page,
  }) => {
    const { server } = await openLive(page);

    server.close(1011);
    await expect(
      page.getByText(
        'Connecting to live updates. Reload the page to check now.',
      ),
    ).toBeVisible();
    await expect(row(page, FIRST.trackingNumber)).toBeVisible();
  });

  test('when the server refuses, live updates are off for good', async ({
    page,
  }) => {
    const { server } = await openLive(page);

    server.close(4403);
    await expect(
      page.getByText(
        'Live updates are off. Reload the page to check for changes.',
      ),
    ).toBeVisible();
    await expect(row(page, FIRST.trackingNumber)).toBeVisible();
  });

  test('a customer gets their own list, and nothing connects', async ({
    page,
  }) => {
    const server = await liveServer(page);
    await signInAs(page, 'CUSTOMER');
    await page.goto('/parcels');

    // /parcels shows customers their own parcels, not the dispatcher table.
    await expect(
      page.getByRole('heading', { name: 'My parcels' }),
    ).toBeVisible();
    await expect(table(page)).toHaveCount(0);
    expect(server.connections()).toBe(0);
  });
});
