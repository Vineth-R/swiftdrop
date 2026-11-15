import { expect, test } from '@playwright/test';

import {
  fail,
  ok,
  REQUEST_ID,
  SERVER_WORDS,
  type Reply,
} from './support/mock-api';
import { signInAs, type Role } from './support/sign-in';

// Local runs can use installed Chrome; CI keeps bundled Chromium by default.
test.use({ channel: process.env['PLAYWRIGHT_CHANNEL'] });

const listEndpoint = 'GET /courier/deliveries';
const parcelId = '00000000-0000-4000-8000-000000000101';
const delivery = {
  id: parcelId,
  trackingNumber: 'SDTODAY001',
  status: 'OUT_FOR_DELIVERY',
  pickupAddress: '10 Test Road, Colombo',
  dropoffAddress: '45 Lake Drive',
  dropoffCity: 'Kandy',
  recipientName: 'Nimali Perera',
  recipientPhone: '+94771234567',
  weightKg: 2.5,
  assignedAt: '2026-11-05T09:00:00Z',
};

const detail = {
  ...delivery,
  description: 'Documents',
  sizeCategory: 'SMALL',
  senderName: 'Kamal Silva',
  senderPhone: '+94770000000',
  history: [
    {
      previousStatus: 'IN_TRANSIT',
      status: 'OUT_FOR_DELIVERY',
      note: 'At the destination',
      failureReason: null,
      actorRole: 'COURIER',
      changedAt: '2026-11-05T10:00:00Z',
    },
  ],
  proof: null,
  proofs: [],
};

const pickupProof = {
  id: '00000000-0000-4000-8000-000000000301',
  kind: 'PICKUP',
  recipientName: null,
  note: 'Collected from sender',
  hasPhoto: true,
  createdAt: '2026-11-05T09:30:00Z',
};

const deliveryProof = {
  id: '00000000-0000-4000-8000-000000000302',
  kind: 'DELIVERY',
  recipientName: 'Nimali Perera',
  note: 'Handed over',
  hasPhoto: true,
  createdAt: '2026-11-05T11:00:00Z',
};

function historyFor(status: string) {
  const created = {
    previousStatus: null,
    status: 'CREATED',
    note: null,
    failureReason: null,
    actorRole: 'CUSTOMER',
    changedAt: '2026-11-05T08:00:00Z',
  };
  const picked = {
    previousStatus: 'CREATED',
    status: 'PICKED_UP',
    note: 'Collected',
    failureReason: null,
    actorRole: 'COURIER',
    changedAt: '2026-11-05T09:30:00Z',
  };
  const transit = {
    previousStatus: 'PICKED_UP',
    status: 'IN_TRANSIT',
    note: null,
    failureReason: null,
    actorRole: 'COURIER',
    changedAt: '2026-11-05T09:30:01Z',
  };
  const out = detail.history[0];
  const delivered = {
    previousStatus: 'OUT_FOR_DELIVERY',
    status: 'DELIVERED',
    note: 'Handed over',
    failureReason: null,
    actorRole: 'COURIER',
    changedAt: '2026-11-05T11:00:00Z',
  };
  const failed = {
    previousStatus: 'OUT_FOR_DELIVERY',
    status: 'FAILED',
    note: null,
    failureReason: 'Recipient was unavailable',
    actorRole: 'COURIER',
    changedAt: '2026-11-05T11:00:00Z',
  };
  switch (status) {
    case 'CREATED':
      return [created];
    case 'PICKED_UP':
      return [created, picked];
    case 'IN_TRANSIT':
      return [created, picked, transit];
    case 'OUT_FOR_DELIVERY':
      return [created, picked, transit, out];
    case 'DELIVERED':
      return [created, picked, transit, out, delivered];
    case 'FAILED':
      return [created, picked, transit, out, failed];
    default:
      return [out];
  }
}

function detailWith(status: string, extra: Record<string, unknown> = {}) {
  return {
    ...detail,
    status,
    history: historyFor(status),
    ...extra,
  };
}

function pageOf(items: unknown[], page = 1, totalItems = items.length) {
  return {
    status: 200,
    body: {
      data: items,
      meta: {
        requestId: REQUEST_ID,
        pagination: {
          page,
          pageSize: 20,
          totalItems,
          totalPages: Math.ceil(totalItems / 20),
        },
      },
    },
  };
}

function colomboDate() {
  const parts = new Intl.DateTimeFormat('en-GB', {
    timeZone: 'Asia/Colombo',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  }).formatToParts(new Date());
  const part = (name: string) =>
    parts.find((item) => item.type === name)?.value;
  return `${part('year')}-${part('month')}-${part('day')}`;
}

test('shows assigned deliveries and opens their authorized detail', async ({
  page,
}) => {
  await page.setViewportSize({ width: 360, height: 900 });
  const api = await signInAs(page, 'COURIER', {
    [listEndpoint]: ({ query }) =>
      pageOf(
        [
          {
            ...delivery,
            trackingNumber:
              query.get('page') === '2'
                ? 'SDTODAY021'
                : delivery.trackingNumber,
          },
        ],
        Number(query.get('page')),
        21,
      ),
    [`GET /courier/deliveries/${parcelId}`]: ok(detail),
  });
  const dateBefore = colomboDate();
  await page.goto('/dashboard');
  await expect(
    page.getByRole('heading', { level: 1, name: 'Today' }),
  ).toBeVisible();
  await expect(page.getByText('Your deliveries for today.')).toBeVisible();
  await expect(page.getByText(delivery.trackingNumber)).toBeVisible();
  await expect(
    page.getByText('OUT FOR DELIVERY', { exact: true }),
  ).toBeVisible();
  await expect(page.getByText('10 Test Road, Colombo')).toBeVisible();
  await page.getByRole('button', { name: 'Go to next page' }).click();
  await expect(page.getByText('SDTODAY021')).toBeVisible();
  await page.getByRole('button', { name: 'Go to previous page' }).click();
  await expect(page.getByText(delivery.trackingNumber)).toBeVisible();
  expect(
    api.calls
      .filter((call) => call.key === listEndpoint)
      .map((call) => new URL(call.url).searchParams.get('page')),
  ).toContain('2');
  const dateAfter = colomboDate();
  const listDates = api.calls
    .filter((call) => call.key === listEndpoint)
    .map((call) => new URL(call.url).searchParams.get('date'));
  expect(listDates.length).toBeGreaterThanOrEqual(2);
  for (const value of listDates)
    expect([dateBefore, dateAfter]).toContain(value);
  for (const width of [360, 1280]) {
    await page.setViewportSize({ width, height: 900 });
    for (const control of [
      page.getByRole('link', { name: 'View delivery' }),
      page.getByRole('button', { name: 'Refresh' }),
      page.getByRole('button', { name: 'Go to next page' }),
    ]) {
      const box = await control.boundingBox();
      expect(box?.height).toBeGreaterThanOrEqual(48);
      expect(box?.width).toBeGreaterThanOrEqual(48);
    }
    expect(
      await page.evaluate(
        'document.documentElement.scrollWidth <= window.innerWidth',
      ),
    ).toBe(true);
    await page.screenshot({
      path: test.info().outputPath(`courier-today-${width}.png`),
      fullPage: true,
    });
  }
  await page.getByRole('link', { name: 'View delivery' }).click();
  await expect(page).toHaveURL(new RegExp(`/deliveries/${parcelId}$`));
  await expect(
    page.getByRole('heading', { level: 1, name: delivery.trackingNumber }),
  ).toBeVisible();
  await expect(page.getByText('Kamal Silva')).toBeVisible();
  await expect(page.getByText('Small', { exact: true })).toBeVisible();
  await expect(
    page.getByRole('list', { name: 'Status history' }),
  ).toContainText('Out for delivery');
  await expect(page.getByText('At the destination')).toBeVisible();
});

test('shows empty, loading and safe error states for Today', async ({
  page,
}) => {
  let response: Reply = pageOf([]);
  await signInAs(page, 'COURIER', { [listEndpoint]: () => response });
  await page.goto('/dashboard');
  await expect(
    page.getByRole('heading', { name: 'No assigned deliveries' }),
  ).toBeVisible({ timeout: 15_000 });

  response = fail(500, 'INTERNAL_ERROR');
  await page.getByRole('button', { name: 'Refresh' }).click();
  await expect(page.getByRole('main').getByRole('alert')).toContainText(
    'Something broke on our side',
  );
  await expect(page.getByText(`Reference ${REQUEST_ID}`)).toBeVisible();
  await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
  response = pageOf([delivery]);
  await page.getByRole('button', { name: 'Refresh' }).click();
  await expect(page.getByText(delivery.trackingNumber)).toBeVisible();
});

test('shows accessible initial-loading and manual-refresh states', async ({
  page,
}) => {
  let response: Reply = { ...pageOf([delivery]), delayMs: 3000 };
  await signInAs(page, 'COURIER', { [listEndpoint]: () => response });
  await page.goto('/dashboard');
  await expect(
    page.getByRole('status').filter({ hasText: 'Loading assigned deliveries' }),
  ).toBeVisible();
  await expect(page.getByRole('button', { name: 'Loading' })).toBeVisible();
  await expect(page.getByText(delivery.trackingNumber)).toBeVisible({
    timeout: 15_000,
  });

  response = { ...pageOf([delivery]), delayMs: 3000 };
  await page.getByRole('button', { name: 'Refresh' }).click();
  await expect(page.getByRole('button', { name: 'Refreshing' })).toBeVisible();
});

for (const role of ['CUSTOMER', 'MERCHANT', 'DISPATCHER', 'ADMIN'] as Role[]) {
  test(`${role} cannot open a courier delivery`, async ({ page }) => {
    const api = await signInAs(page, role, {
      [`GET /courier/deliveries/${parcelId}`]: ok(detail),
    });
    await page.goto(`/deliveries/${parcelId}`);
    await expect(
      page.getByRole('heading', { name: 'You do not have access' }),
    ).toBeVisible();
    expect(
      api.calls.some(
        (call) => call.key === `GET /courier/deliveries/${parcelId}`,
      ),
    ).toBe(false);
    await page.goto('/dashboard');
    if (role === 'DISPATCHER') {
      // A dispatcher has no dashboard: their home is the unassigned list.
      // The client-side redirect waits for the page to compile in dev.
      await expect(page).toHaveURL(/\/unassigned$/, { timeout: 15_000 });
    } else {
      await expect(
        page.getByRole('heading', { level: 1, name: 'Dashboard', exact: true }),
      ).toBeVisible();
    }
    expect(api.calls.some((call) => call.key === listEndpoint)).toBe(false);
  });
}

test('an assigned courier cannot view another courier’s delivery', async ({
  page,
}) => {
  await signInAs(page, 'COURIER', {
    [`GET /courier/deliveries/${parcelId}`]: fail(403, 'FORBIDDEN'),
  });
  await page.goto(`/deliveries/${parcelId}`);
  await expect(
    page.getByRole('heading', {
      name: 'You do not have access to this delivery',
    }),
  ).toBeVisible();
  await expect(page.getByText('Kamal Silva')).toHaveCount(0);
});

test('shows the redesigned delivery sections in order', async ({ page }) => {
  await signInAs(page, 'COURIER', {
    [`GET /courier/deliveries/${parcelId}`]: ok(detailWith('OUT_FOR_DELIVERY')),
  });
  await page.goto(`/deliveries/${parcelId}`);
  await expect(
    page.getByRole('heading', { level: 1, name: delivery.trackingNumber }),
  ).toBeVisible();
  await expect(page.getByText(/^Assigned \d/)).toBeVisible();

  const names = [
    'Delivery progress',
    'Sender and recipient',
    'Parcel details',
    'Confirm delivery',
    'Activity',
  ];
  const positions: number[] = [];
  for (const name of names) {
    const heading = page.getByRole('heading', { level: 2, name, exact: true });
    await expect(heading).toBeVisible();
    positions.push((await heading.boundingBox())?.y ?? 0);
  }
  expect(positions).toEqual([...positions].sort((left, right) => left - right));

  const progress = page.getByRole('list', { name: 'Delivery progress' });
  await expect(progress.locator('[aria-current="step"]')).toContainText(
    'Out for delivery',
  );
  await expect(progress).toContainText('Created');
  await expect(progress).toContainText('Picked up');
  await expect(progress).toContainText('Delivered');
});

for (const scenario of [
  { status: 'CREATED', action: 'Confirm pickup' },
  { status: 'PICKED_UP', action: 'Continue to in transit' },
  { status: 'IN_TRANSIT', action: 'Mark out for delivery' },
  { status: 'OUT_FOR_DELIVERY', action: 'Confirm delivery' },
] as const) {
  test(`offers only the ${scenario.action} action for ${scenario.status}`, async ({
    page,
  }) => {
    await signInAs(page, 'COURIER', {
      [`GET /courier/deliveries/${parcelId}`]: ok(detailWith(scenario.status)),
    });
    await page.goto(`/deliveries/${parcelId}`);
    await expect(page.getByLabel('Next status')).toHaveCount(0);
    await expect(
      page.getByRole('button', { name: scenario.action }),
    ).toBeVisible();
    await expect(
      page.getByRole('button', { name: 'Report delivery failure' }),
    ).toBeVisible();
    if (scenario.status === 'CREATED') {
      await expect(page.getByLabel('Pickup proof photo')).toBeVisible();
    }
    if (scenario.status === 'PICKED_UP') {
      await expect(page.getByLabel('Pickup proof photo')).toHaveCount(0);
      await expect(page.getByLabel('Delivery proof photo')).toHaveCount(0);
    }
    if (scenario.status === 'OUT_FOR_DELIVERY') {
      await expect(page.getByLabel('Recipient name')).toBeVisible();
      await expect(page.getByLabel('Delivery proof photo')).toBeVisible();
      await expect(page.getByLabel('Delivery note')).toBeVisible();
    }
  });
}

test('shows a delivered parcel without further actions', async ({ page }) => {
  const delivered = detailWith('DELIVERED', {
    proof: deliveryProof,
    proofs: [pickupProof, deliveryProof],
  });
  await signInAs(page, 'COURIER', {
    [`GET /courier/deliveries/${parcelId}`]: ok(delivered),
  });
  await page.goto(`/deliveries/${parcelId}`);
  await expect(
    page.getByRole('heading', { level: 2, name: 'Delivered', exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText('No further status action is available.'),
  ).toBeVisible();
  await expect(
    page.getByRole('button', { name: 'Report delivery failure' }),
  ).toHaveCount(0);
  await expect(page.getByLabel('Next status')).toHaveCount(0);
  await expect(page.getByText('Nimali Perera').first()).toBeVisible();
});

test('shows a failed parcel with its reason and no further actions', async ({
  page,
}) => {
  await signInAs(page, 'COURIER', {
    [`GET /courier/deliveries/${parcelId}`]: ok(detailWith('FAILED')),
  });
  await page.goto(`/deliveries/${parcelId}`);
  await expect(
    page.getByRole('heading', {
      level: 2,
      name: 'Delivery failed',
      exact: true,
    }),
  ).toBeVisible();
  await expect(
    page.getByText('Recipient was unavailable').first(),
  ).toBeVisible();
  await expect(
    page.getByRole('button', { name: 'Report delivery failure' }),
  ).toHaveCount(0);
  await expect(page.getByLabel('Next status')).toHaveCount(0);
});

test('pickup rejects a missing, invalid or oversized photo before posting', async ({
  page,
}) => {
  const api = await signInAs(page, 'COURIER', {
    [`GET /courier/deliveries/${parcelId}`]: ok(detailWith('CREATED')),
  });
  await page.goto(`/deliveries/${parcelId}`);
  const photo = page.getByLabel('Pickup proof photo');
  await expect(photo).toHaveAttribute('capture', 'environment');

  await page.getByRole('button', { name: 'Confirm pickup' }).click();
  await expect(
    page.getByText('Add a JPEG or PNG photo of the parcel at pickup.'),
  ).toBeVisible();

  await photo.setInputFiles({
    name: 'renamed.jpg',
    mimeType: 'text/plain',
    buffer: Buffer.from('not an image'),
  });
  await expect(page.getByText('Choose a JPEG or PNG image.')).toBeVisible();
  await expect(photo).toHaveValue('');

  await photo.setInputFiles({
    name: 'too-large.png',
    mimeType: 'image/png',
    buffer: Buffer.alloc(5 * 1024 * 1024 + 1),
  });
  await expect(page.getByText('Photo must be 5 MiB or smaller.')).toBeVisible();
  await expect(photo).toHaveValue('');

  await photo.setInputFiles({
    name: 'pickup.png',
    mimeType: 'image/png',
    buffer: Buffer.from('png preview'),
  });
  await expect(page.getByAltText('Pickup proof photo preview')).toBeVisible();
  expect(
    api.calls.filter((call) =>
      call.key.startsWith('POST /courier/deliveries/'),
    ),
  ).toHaveLength(0);
});

test('confirms pickup with a photo and records both transitions', async ({
  page,
}) => {
  let pickedUp = false;
  const captured = { contentType: '', body: '' };
  await signInAs(page, 'COURIER', {
    [`GET /courier/deliveries/${parcelId}`]: () =>
      ok(
        pickedUp
          ? detailWith('IN_TRANSIT', {
              proof: pickupProof,
              proofs: [pickupProof],
            })
          : detailWith('CREATED'),
      ),
  });
  await page.route(
    `**/api/v1/courier/deliveries/${parcelId}/pickup`,
    async (route) => {
      if (route.request().method() !== 'POST') return route.fallback();
      captured.contentType = route.request().headers()['content-type'] ?? '';
      captured.body = route.request().postDataBuffer()?.toString('utf8') ?? '';
      pickedUp = true;
      await route.fulfill({
        status: 200,
        json: {
          data: {
            parcelId,
            trackingNumber: delivery.trackingNumber,
            previousStatus: 'CREATED',
            status: 'IN_TRANSIT',
            updatedAt: '2026-11-05T09:31:00Z',
            alreadyApplied: false,
            history: [],
            proof: pickupProof,
          },
          meta: { requestId: REQUEST_ID },
        },
      });
    },
  );
  await page.goto(`/deliveries/${parcelId}`);
  await page.getByLabel('Pickup proof photo').setInputFiles({
    name: 'pickup.png',
    mimeType: 'image/png',
    buffer: Buffer.from('png content'),
  });
  await page.getByRole('button', { name: 'Confirm pickup' }).click();

  await expect(
    page.getByText('Pickup confirmed. The parcel is now in transit.'),
  ).toBeVisible();
  await expect(
    page
      .getByRole('list', { name: 'Delivery progress' })
      .locator('[aria-current="step"]'),
  ).toContainText('In transit');
  const history = page.getByRole('list', { name: 'Status history' });
  await expect(history).toContainText('Picked up');
  await expect(history).toContainText('In transit');
  await expect(page.getByLabel('Next status')).toHaveCount(0);
  expect(captured.contentType).toMatch(/^multipart\/form-data; boundary=/);
  expect(captured.body).toContain('name="photo"; filename="pickup.png"');
});

test('prevents duplicate pickup submissions at phone width', async ({
  page,
}) => {
  let pickedUp = false;
  let attempts = 0;
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  await signInAs(page, 'COURIER', {
    [`GET /courier/deliveries/${parcelId}`]: () =>
      ok(
        pickedUp
          ? detailWith('IN_TRANSIT', {
              proof: pickupProof,
              proofs: [pickupProof],
            })
          : detailWith('CREATED'),
      ),
  });
  await page.route(
    `**/api/v1/courier/deliveries/${parcelId}/pickup`,
    async (route) => {
      attempts += 1;
      pickedUp = true;
      await gate;
      await route.fulfill({
        status: 200,
        json: {
          data: {
            parcelId,
            trackingNumber: delivery.trackingNumber,
            previousStatus: 'CREATED',
            status: 'IN_TRANSIT',
            updatedAt: '2026-11-05T09:31:00Z',
            alreadyApplied: false,
            history: [],
            proof: pickupProof,
          },
          meta: { requestId: REQUEST_ID },
        },
      });
    },
  );
  await page.setViewportSize({ width: 360, height: 900 });
  await page.emulateMedia({ colorScheme: 'dark' });
  await page.goto(`/deliveries/${parcelId}`);
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
  await page.getByLabel('Pickup proof photo').setInputFiles({
    name: 'pickup.png',
    mimeType: 'image/png',
    buffer: Buffer.from('png content'),
  });
  try {
    await page.getByRole('button', { name: 'Confirm pickup' }).click();
    const saving = page.getByRole('button', { name: 'Confirming pickup' });
    await expect(saving).toHaveAttribute('aria-disabled', 'true');
    await saving.evaluate((button) => {
      button.click();
      button.closest('form')?.requestSubmit();
    });
    expect(attempts).toBe(1);
    const box = await saving.boundingBox();
    expect(box?.height).toBeGreaterThanOrEqual(48);
    expect(
      await page.evaluate(
        'document.documentElement.scrollWidth <= window.innerWidth',
      ),
    ).toBe(true);
  } finally {
    release();
  }
  await expect(
    page.getByText('Pickup confirmed. The parcel is now in transit.'),
  ).toBeVisible();
  expect(attempts).toBe(1);
  await page.screenshot({
    path: test.info().outputPath('courier-360-dark.png'),
    fullPage: true,
  });
});

test('delivery needs a recipient and photo, then a confirmation step', async ({
  page,
}) => {
  let delivered = false;
  const captured = { contentType: '', body: '' };
  await signInAs(page, 'COURIER', {
    [`GET /courier/deliveries/${parcelId}`]: () =>
      ok(
        delivered
          ? detailWith('DELIVERED', {
              proof: deliveryProof,
              proofs: [pickupProof, deliveryProof],
            })
          : detailWith('OUT_FOR_DELIVERY'),
      ),
  });
  await page.route(
    `**/api/v1/courier/deliveries/${parcelId}/delivery`,
    async (route) => {
      if (route.request().method() !== 'POST') return route.fallback();
      captured.contentType = route.request().headers()['content-type'] ?? '';
      captured.body = route.request().postDataBuffer()?.toString('utf8') ?? '';
      delivered = true;
      await route.fulfill({
        status: 200,
        json: {
          data: {
            parcelId,
            trackingNumber: delivery.trackingNumber,
            previousStatus: 'OUT_FOR_DELIVERY',
            status: 'DELIVERED',
            updatedAt: '2026-11-05T11:00:00Z',
            alreadyApplied: false,
            history: [],
            proof: deliveryProof,
          },
          meta: { requestId: REQUEST_ID },
        },
      });
    },
  );
  await page.goto(`/deliveries/${parcelId}`);
  await page.getByRole('button', { name: 'Confirm delivery' }).click();
  await expect(
    page.getByText('Enter the name of the person who received the parcel.'),
  ).toBeVisible();

  await page.getByLabel('Recipient name').fill('Nimali Perera');
  await page.getByRole('button', { name: 'Confirm delivery' }).click();
  await expect(
    page.getByText('Add a JPEG or PNG photo of the handover.'),
  ).toBeVisible();

  await page.getByLabel('Delivery proof photo').setInputFiles({
    name: 'handover.png',
    mimeType: 'image/png',
    buffer: Buffer.from('png content'),
  });
  await page.getByLabel('Delivery note').fill('Handed over');
  await page.getByRole('button', { name: 'Confirm delivery' }).click();
  const dialog = page.getByRole('dialog', { name: 'Confirm delivery?' });
  await expect(dialog).toBeVisible();
  await dialog.getByRole('button', { name: 'Cancel' }).click();
  await expect(dialog).toHaveCount(0);
  expect(captured.body).toBe('');

  await page.getByRole('button', { name: 'Confirm delivery' }).click();
  await dialog.getByRole('button', { name: 'Confirm delivery' }).click();
  await expect(
    page.getByText('Delivery confirmed. The parcel is now delivered.'),
  ).toBeVisible();
  await expect(
    page.getByRole('heading', { level: 2, name: 'Delivered', exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText('No further status action is available.'),
  ).toBeVisible();
  await expect(
    page.getByRole('list', { name: 'Status history' }),
  ).toContainText('Delivered');
  await expect(
    page.getByRole('heading', { level: 3, name: 'Delivery proof' }),
  ).toBeVisible();
  expect(captured.contentType).toMatch(/^multipart\/form-data; boundary=/);
  expect(captured.body).toContain('name="recipientName"');
  expect(captured.body).toContain('Nimali Perera');
  expect(captured.body).toContain('name="photo"; filename="handover.png"');
  expect(captured.body).toContain('name="note"');
  expect(captured.body).toContain('Handed over');
});

test('records a failed delivery with a reason', async ({ page }) => {
  let failed = false;
  const api = await signInAs(page, 'COURIER', {
    [`GET /courier/deliveries/${parcelId}`]: () =>
      ok(failed ? detailWith('FAILED') : detailWith('OUT_FOR_DELIVERY')),
    [`POST /courier/deliveries/${parcelId}/status`]: () => {
      failed = true;
      return ok({
        parcelId,
        trackingNumber: delivery.trackingNumber,
        previousStatus: 'OUT_FOR_DELIVERY',
        status: 'FAILED',
        updatedAt: '2026-11-05T11:00:00Z',
      });
    },
  });
  await page.goto(`/deliveries/${parcelId}`);
  await page.getByRole('button', { name: 'Report delivery failure' }).click();
  const dialog = page.getByRole('dialog', {
    name: 'Report a failed delivery',
  });
  await expect(dialog).toBeVisible();
  await dialog.getByRole('button', { name: 'Report failure' }).click();
  await expect(
    page.getByText('Failure reason must be between 5 and 255 characters.'),
  ).toBeVisible();

  await page
    .getByLabel('Failure reason')
    .fill('Recipient was unavailable at the address');
  await dialog.getByRole('button', { name: 'Report failure' }).click();

  await expect(page.getByText('Delivery failure recorded.')).toBeVisible();
  await expect(
    page.getByRole('heading', {
      level: 2,
      name: 'Delivery failed',
      exact: true,
    }),
  ).toBeVisible();
  await expect(
    page.getByText('Recipient was unavailable').first(),
  ).toBeVisible();
  await expect(
    page.getByRole('button', { name: 'Report delivery failure' }),
  ).toHaveCount(0);
  expect(
    api.calls
      .filter(
        (call) => call.key === `POST /courier/deliveries/${parcelId}/status`,
      )
      .map((call) => call.body),
  ).toEqual([
    {
      status: 'FAILED',
      failureReason: 'Recipient was unavailable at the address',
    },
  ]);
});

test('handles 403, 409 and 422 errors without exposing server text', async ({
  page,
}) => {
  let detailReply: Reply = ok(detailWith('OUT_FOR_DELIVERY'));
  let updateReply: Reply = fail(409, 'INVALID_TRANSITION');
  await signInAs(page, 'COURIER', {
    [`GET /courier/deliveries/${parcelId}`]: () => detailReply,
    [`POST /courier/deliveries/${parcelId}/delivery`]: () => updateReply,
  });
  await page.goto(`/deliveries/${parcelId}`);
  await page.getByLabel('Recipient name').fill('Nimali Perera');
  await page.getByLabel('Delivery proof photo').setInputFiles({
    name: 'handover.png',
    mimeType: 'image/png',
    buffer: Buffer.from('png content'),
  });
  await page.getByRole('button', { name: 'Confirm delivery' }).click();
  await page
    .getByRole('dialog', { name: 'Confirm delivery?' })
    .getByRole('button', { name: 'Confirm delivery' })
    .click();
  await expect(
    page.getByText('This delivery changed before you saved it'),
  ).toBeVisible();
  await expect(page.getByLabel('Recipient name')).toHaveValue('Nimali Perera');

  detailReply = fail(503, 'SERVICE_UNAVAILABLE');
  await page.getByRole('button', { name: 'Refresh delivery' }).click();
  await expect(
    page.getByRole('region', { name: 'Delivery refresh failed' }),
  ).toContainText('Something broke on our side');
  await expect(page.getByText(`Reference ${REQUEST_ID}`)).toBeVisible();
  await expect(
    page.getByRole('button', { name: 'Confirm delivery' }),
  ).toBeDisabled();

  detailReply = ok(detailWith('OUT_FOR_DELIVERY'));
  await page.getByRole('button', { name: 'Try again' }).click();
  await expect(
    page.getByRole('button', { name: 'Confirm delivery' }),
  ).toBeEnabled();

  updateReply = fail(422, 'VALIDATION_ERROR');
  await page.getByRole('button', { name: 'Confirm delivery' }).click();
  await page
    .getByRole('dialog', { name: 'Confirm delivery?' })
    .getByRole('button', { name: 'Confirm delivery' })
    .click();
  await expect(page.getByText('Check the delivery details')).toBeVisible();

  updateReply = fail(403, 'FORBIDDEN');
  await page.getByRole('button', { name: 'Confirm delivery' }).click();
  await page
    .getByRole('dialog', { name: 'Confirm delivery?' })
    .getByRole('button', { name: 'Confirm delivery' })
    .click();
  await expect(
    page.getByText('You no longer have access to this delivery'),
  ).toBeVisible();
  await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
});

test('handles a pickup rate limit and re-enables the action', async ({
  page,
}) => {
  let attempts = 0;
  let pickedUp = false;
  await signInAs(page, 'COURIER', {
    [`GET /courier/deliveries/${parcelId}`]: () =>
      ok(
        pickedUp
          ? detailWith('IN_TRANSIT', {
              proof: pickupProof,
              proofs: [pickupProof],
            })
          : detailWith('CREATED'),
      ),
    [`POST /courier/deliveries/${parcelId}/pickup`]: () => {
      attempts += 1;
      if (attempts === 1)
        return fail(429, 'RATE_LIMITED', {
          headers: { 'Retry-After': '1' },
        });
      pickedUp = true;
      return ok({
        parcelId,
        trackingNumber: delivery.trackingNumber,
        previousStatus: 'CREATED',
        status: 'IN_TRANSIT',
        updatedAt: '2026-11-05T09:31:00Z',
        alreadyApplied: false,
        history: [],
        proof: pickupProof,
      });
    },
  });
  await page.goto(`/deliveries/${parcelId}`);
  const confirm = page.getByRole('button', { name: 'Confirm pickup' });
  await page.getByLabel('Pickup proof photo').setInputFiles({
    name: 'pickup.png',
    mimeType: 'image/png',
    buffer: Buffer.from('png content'),
  });
  await confirm.click();
  await expect(
    page.getByText('Too many attempts from this device'),
  ).toBeVisible();
  await expect(page.getByText(/\d+ seconds/)).toBeVisible();
  await expect(confirm).toBeDisabled();
  await expect(page.getByText('You can try again now.')).toBeVisible({
    timeout: 5000,
  });
  await expect(confirm).toBeEnabled();

  await confirm.click();
  await expect(
    page.getByText('Pickup confirmed. The parcel is now in transit.'),
  ).toBeVisible();
  expect(attempts).toBe(2);
});

test('opens the pickup and delivery proof photos from Activity', async ({
  page,
  context,
}) => {
  const delivered = detailWith('DELIVERED', {
    proof: deliveryProof,
    proofs: [pickupProof, deliveryProof],
  });
  await context.route('https://storage.example/**', (route) =>
    route.fulfill({ status: 200, contentType: 'image/jpeg', body: 'photo' }),
  );
  const api = await signInAs(page, 'COURIER', {
    [`GET /courier/deliveries/${parcelId}`]: ok(delivered),
    [`GET /parcels/${parcelId}/proofs/${pickupProof.id}/photo`]: ok({
      url: 'https://storage.example/pickup.jpg',
      expiresAt: '2026-11-05T11:00:00Z',
      mimeType: 'image/jpeg',
      stream: false,
    }),
    [`GET /parcels/${parcelId}/proofs/${deliveryProof.id}/photo`]: ok({
      url: 'https://storage.example/delivery.jpg',
      expiresAt: '2026-11-05T11:00:00Z',
      mimeType: 'image/jpeg',
      stream: false,
    }),
  });
  await page.goto(`/deliveries/${parcelId}`);
  await expect(
    page.getByRole('heading', { level: 3, name: 'Pickup proof' }),
  ).toBeVisible();
  await expect(
    page.getByRole('heading', { level: 3, name: 'Delivery proof' }),
  ).toBeVisible();
  await expect(page.getByText('Collected from sender').first()).toBeVisible();

  const buttons = page.getByRole('button', { name: 'View proof photo' });
  await expect(buttons).toHaveCount(2);
  const pickupPopup = page.waitForEvent('popup');
  await buttons.nth(0).click();
  await (await pickupPopup).waitForURL('https://storage.example/pickup.jpg');
  await (await pickupPopup).close();
  const deliveryPopup = page.waitForEvent('popup');
  await buttons.nth(1).click();
  await (
    await deliveryPopup
  ).waitForURL('https://storage.example/delivery.jpg');
  await (await deliveryPopup).close();

  expect(
    api.calls
      .filter((call) => call.key.includes('/proofs/'))
      .map((call) => call.key)
      .sort(),
  ).toEqual(
    [
      `GET /parcels/${parcelId}/proofs/${pickupProof.id}/photo`,
      `GET /parcels/${parcelId}/proofs/${deliveryProof.id}/photo`,
    ].sort(),
  );
});

test('shows loading, not-found and server-fault states', async ({ page }) => {
  let response: Reply = {
    ...ok(detailWith('OUT_FOR_DELIVERY')),
    delayMs: 3000,
  };
  await signInAs(page, 'COURIER', {
    [`GET /courier/deliveries/${parcelId}`]: () => response,
  });
  await page.goto(`/deliveries/${parcelId}`);
  await expect(
    page.getByRole('status').filter({ hasText: 'Loading delivery details' }),
  ).toBeVisible();
  await expect(
    page.getByRole('heading', { level: 1, name: delivery.trackingNumber }),
  ).toBeVisible({ timeout: 15_000 });

  response = fail(404, 'PARCEL_NOT_FOUND');
  await page.reload();
  await expect(
    page.getByRole('heading', { name: 'Delivery not found', exact: true }),
  ).toBeVisible();

  response = fail(503, 'SERVICE_UNAVAILABLE');
  await page.reload();
  await expect(page.getByRole('main').getByRole('alert')).toContainText(
    'Something broke on our side',
  );
  await expect(page.getByText(`Reference ${REQUEST_ID}`)).toBeVisible();
  await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);

  response = ok(detailWith('OUT_FOR_DELIVERY'));
  await page.getByRole('button', { name: 'Try again' }).click();
  await expect(
    page.getByRole('heading', { level: 1, name: delivery.trackingNumber }),
  ).toBeVisible();
});

test('keeps the redesigned detail usable at phone and desktop widths', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.emulateMedia({ colorScheme: 'light' });
  await signInAs(page, 'COURIER', {
    [`GET /courier/deliveries/${parcelId}`]: ok(detailWith('OUT_FOR_DELIVERY')),
  });
  await page.goto(`/deliveries/${parcelId}`);
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
  const confirm = page.getByRole('button', { name: 'Confirm delivery' });
  await expect(confirm).toBeVisible();
  await confirm.focus();
  await expect(confirm).toBeFocused();
  await expect(confirm).toHaveCSS('outline-style', /solid|auto/);
  expect(
    await page.evaluate(
      'document.documentElement.scrollWidth <= window.innerWidth',
    ),
  ).toBe(true);
  await page.screenshot({
    path: test.info().outputPath('courier-1280-light.png'),
    fullPage: true,
  });

  await page.setViewportSize({ width: 360, height: 900 });
  await page.emulateMedia({ colorScheme: 'dark' });
  await page.reload();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
  const box = await page
    .getByRole('button', { name: 'Confirm delivery' })
    .boundingBox();
  expect(box?.height).toBeGreaterThanOrEqual(48);
  expect(
    await page.evaluate(
      'document.documentElement.scrollWidth <= window.innerWidth',
    ),
  ).toBe(true);
  for (const heading of [
    'Delivery progress',
    'Sender and recipient',
    'Parcel details',
    'Confirm delivery',
    'Activity',
  ]) {
    await expect(
      page.getByRole('heading', { level: 2, name: heading, exact: true }),
    ).toBeVisible();
  }
});
