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
  proof: {
    recipientName: 'Nimali Perera',
    note: 'Left with recipient',
    hasPhoto: false,
    createdAt: '2026-11-05T10:30:00Z',
  },
};

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
  ).toContainText('OUT FOR DELIVERY');
  await expect(page.getByText('Left with recipient')).toBeVisible();
});

test('shows empty, loading and safe error states for Today', async ({
  page,
}) => {
  let response: Reply = pageOf([]);
  await signInAs(page, 'COURIER', { [listEndpoint]: () => response });
  await page.goto('/dashboard');
  await expect(
    page.getByRole('heading', { name: 'No assigned deliveries' }),
  ).toBeVisible();

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
  let response: Reply = { ...pageOf([delivery]), delayMs: 1000 };
  await signInAs(page, 'COURIER', { [listEndpoint]: () => response });
  await page.goto('/dashboard');
  await expect(
    page.getByRole('status').filter({ hasText: 'Loading assigned deliveries' }),
  ).toBeVisible();
  await expect(page.getByRole('button', { name: 'Loading' })).toBeVisible();
  await expect(page.getByText(delivery.trackingNumber)).toBeVisible();

  response = { ...pageOf([delivery]), delayMs: 1000 };
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
      await expect(page).toHaveURL(/\/unassigned$/);
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

test('saves a valid status and refreshes the detail timeline', async ({
  page,
}) => {
  let saved = false;
  const api = await signInAs(page, 'COURIER', {
    [`GET /courier/deliveries/${parcelId}`]: () => {
      return ok(
        !saved
          ? detail
          : {
              ...detail,
              status: 'DELIVERED',
              history: [
                ...detail.history,
                {
                  previousStatus: 'OUT_FOR_DELIVERY',
                  status: 'DELIVERED',
                  note: 'Handed over',
                  failureReason: null,
                  actorRole: 'COURIER',
                  changedAt: '2026-11-05T11:00:00Z',
                },
              ],
            },
      );
    },
    [`POST /courier/deliveries/${parcelId}/status`]: () => {
      saved = true;
      return ok({
        parcelId,
        trackingNumber: delivery.trackingNumber,
        previousStatus: 'OUT_FOR_DELIVERY',
        status: 'DELIVERED',
        updatedAt: '2026-11-05T11:00:00Z',
      });
    },
  });
  await page.goto(`/deliveries/${parcelId}`);
  await page.getByLabel('Next status').selectOption('DELIVERED');
  await page.getByLabel('Recipient name').fill('Nimali Perera');
  await page.getByLabel('Note', { exact: true }).fill('Handed over');
  await page.getByRole('button', { name: 'Save status' }).click();
  await expect(page.getByText('Handed over')).toBeVisible();
  await expect(
    page.getByText('Delivery status saved', { exact: true }),
  ).toBeVisible();
  await expect(page.getByRole('combobox', { name: 'Next status' })).toHaveCount(
    0,
  );
  expect(
    api.calls
      .filter(
        (call) => call.key === `POST /courier/deliveries/${parcelId}/status`,
      )
      .map((call) => call.body),
  ).toEqual([
    {
      status: 'DELIVERED',
      recipientName: 'Nimali Perera',
      note: 'Handed over',
    },
  ]);
});

test('validates recipient and failure fields before posting', async ({
  page,
}) => {
  const api = await signInAs(page, 'COURIER', {
    [`GET /courier/deliveries/${parcelId}`]: ok(detail),
    [`POST /courier/deliveries/${parcelId}/status`]: ok({
      parcelId,
      trackingNumber: delivery.trackingNumber,
      previousStatus: 'OUT_FOR_DELIVERY',
      status: 'DELIVERED',
      updatedAt: '2026-11-05T11:00:00Z',
    }),
  });
  await page.goto(`/deliveries/${parcelId}`);
  await page.getByLabel('Next status').selectOption('DELIVERED');
  await page.getByRole('button', { name: 'Save status' }).click();
  await expect(
    page.getByText('Enter the recipient name for a delivered parcel.'),
  ).toBeVisible();

  await page.getByLabel('Next status').selectOption('FAILED');
  await page.getByLabel('Failure reason').fill('bad');
  await page.getByRole('button', { name: 'Save status' }).click();
  await expect(
    page.getByText('Failure reason must be between 5 and 255 characters.'),
  ).toBeVisible();
  await page.getByLabel('Failure reason').fill('😀'.repeat(3));
  await page.getByRole('button', { name: 'Save status' }).click();
  await expect(page.getByLabel('Failure reason')).toHaveAttribute(
    'aria-invalid',
    'true',
  );
  await page.getByLabel('Failure reason').fill('😀'.repeat(5));
  await page.getByLabel('Note', { exact: true }).fill('😀'.repeat(501));
  await page.getByRole('button', { name: 'Save status' }).click();
  await expect(
    page.getByText('Note must be 500 characters or fewer.'),
  ).toBeVisible();
  expect(
    api.calls.filter((call) =>
      call.key.startsWith('POST /courier/deliveries/'),
    ),
  ).toHaveLength(0);
  await page.getByLabel('Next status').selectOption('DELIVERED');
  const recipientName = '\u{1F600}'.repeat(150);
  const note = '\u{1F600}'.repeat(500);
  await page.getByLabel('Recipient name').fill(recipientName);
  await page.getByLabel('Note', { exact: true }).fill(note);
  await page.getByRole('button', { name: 'Save status' }).click();
  await expect(
    page.getByText('Delivery status saved', { exact: true }),
  ).toBeVisible();
  expect(
    api.calls
      .filter((call) => call.key.startsWith('POST /courier/deliveries/'))
      .map((call) => call.body),
  ).toEqual([{ status: 'DELIVERED', recipientName, note }]);
});

test('handles 403, 409 and 422 update errors without exposing server text', async ({
  page,
}) => {
  let reply = fail(409, 'INVALID_TRANSITION');
  let refreshReply = ok(detail);
  await signInAs(page, 'COURIER', {
    [`GET /courier/deliveries/${parcelId}`]: () => refreshReply,
    [`POST /courier/deliveries/${parcelId}/status`]: () => reply,
  });
  await page.goto(`/deliveries/${parcelId}`);
  await page.getByLabel('Next status').selectOption('FAILED');
  await page.getByLabel('Failure reason').fill('Recipient was unavailable');
  await page.getByRole('button', { name: 'Save status' }).click();
  await expect(page.getByRole('status')).toContainText(
    'This delivery changed before you saved it',
  );
  await expect(page.getByLabel('Failure reason')).toHaveValue(
    'Recipient was unavailable',
  );
  refreshReply = fail(503, 'SERVICE_UNAVAILABLE');
  await page.getByRole('button', { name: 'Refresh delivery' }).click();
  await expect(
    page.getByRole('region', { name: 'Delivery refresh failed' }),
  ).toContainText('Something broke on our side');
  await expect(page.getByLabel('Failure reason')).toHaveValue(
    'Recipient was unavailable',
  );
  await expect(
    page.getByRole('button', { name: 'Save status' }),
  ).toBeDisabled();
  refreshReply = ok(detail);
  await page.getByRole('button', { name: 'Try again' }).click();
  await expect(page.getByRole('button', { name: 'Save status' })).toBeEnabled();
  await expect(page.getByLabel('Failure reason')).toHaveValue(
    'Recipient was unavailable',
  );
  reply = fail(422, 'FAILURE_REASON_REQUIRED');
  await page.getByRole('button', { name: 'Save status' }).click();
  await expect(page.getByText('Status update needs review')).toBeVisible();
  reply = fail(403, 'FORBIDDEN');
  await page.getByRole('button', { name: 'Save status' }).click();
  await expect(page.getByRole('main').getByRole('alert')).toContainText(
    'You no longer have access to this delivery',
  );
  await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
});

test('prevents duplicate submissions and works at phone and desktop widths', async ({
  page,
}) => {
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  const api = await signInAs(page, 'COURIER', {
    [`GET /courier/deliveries/${parcelId}`]: ok({
      ...detail,
      history: [{ ...detail.history[0], note: 'Unbroken'.repeat(70) }],
    }),
  });
  const bodies: unknown[] = [];
  await page.route(
    `**/api/v1/courier/deliveries/${parcelId}/status`,
    async (route) => {
      bodies.push(route.request().postDataJSON());
      await gate;
      await route.fulfill({
        status: 200,
        json: {
          data: {
            parcelId,
            trackingNumber: delivery.trackingNumber,
            previousStatus: 'OUT_FOR_DELIVERY',
            status: 'FAILED',
            updatedAt: '2026-11-05T11:00:00Z',
          },
        },
      });
    },
  );
  await page.setViewportSize({ width: 360, height: 900 });
  await page.emulateMedia({ colorScheme: 'dark' });
  await page.goto(`/deliveries/${parcelId}`);
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
  await page.getByLabel('Next status').selectOption('FAILED');
  await page.getByLabel('Failure reason').fill('Recipient was unavailable');
  try {
    await page.getByRole('button', { name: 'Save status' }).click();
    const save = page.getByRole('button', { name: 'Saving status' });
    await expect(save).toHaveAttribute('aria-disabled', 'true');
    // Exercise both programmatic click and form submission while the request is held.
    await save.evaluate((button) => {
      button.click();
      button.closest('form')?.requestSubmit();
    });
    expect(bodies).toHaveLength(1);
    const box = await save.boundingBox();
    expect(box?.height).toBeGreaterThanOrEqual(48);
    expect(
      await page.evaluate(
        'document.documentElement.scrollWidth <= window.innerWidth',
      ),
    ).toBe(true);
    expect(
      api.calls.filter((call) =>
        call.key.startsWith('POST /courier/deliveries/'),
      ),
    ).toHaveLength(0);
  } finally {
    release();
  }
  await expect(
    page.getByText('Delivery status saved', { exact: true }),
  ).toBeVisible();
  expect(bodies).toEqual([
    { status: 'FAILED', failureReason: 'Recipient was unavailable' },
  ]);
  await page.screenshot({
    path: test.info().outputPath('courier-360-dark.png'),
    fullPage: true,
  });
});

test('keeps the courier detail usable at desktop width', async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.emulateMedia({ colorScheme: 'light' });
  let currentDetail = detail;
  let response: Reply | null = null;
  await signInAs(page, 'COURIER', {
    [`GET /courier/deliveries/${parcelId}`]: () =>
      response ?? ok(currentDetail),
  });
  await page.goto(`/deliveries/${parcelId}`);
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
  await expect(page.getByLabel('Next status')).toBeVisible();
  await page.getByLabel('Next status').focus();
  await expect(page.getByLabel('Next status')).toBeFocused();
  const box = await page.getByLabel('Next status').boundingBox();
  expect(box?.height).toBeGreaterThanOrEqual(48);
  await expect(page.getByLabel('Next status')).toHaveCSS(
    'outline-style',
    /solid|auto/,
  );
  await page.keyboard.press('Tab');
  await expect(page.getByLabel('Note', { exact: true })).toBeFocused();
  await page.keyboard.press('Tab');
  await expect(page.getByRole('button', { name: 'Save status' })).toBeFocused();
  expect(
    await page.evaluate(
      'document.documentElement.scrollWidth <= window.innerWidth',
    ),
  ).toBe(true);
  await page.screenshot({
    path: test.info().outputPath('courier-1280-light.png'),
    fullPage: true,
  });
  for (const [status, next] of [
    ['CREATED', ['PICKED_UP', 'FAILED']],
    ['PICKED_UP', ['IN_TRANSIT', 'FAILED']],
    ['IN_TRANSIT', ['OUT_FOR_DELIVERY', 'FAILED']],
    ['OUT_FOR_DELIVERY', ['DELIVERED', 'FAILED']],
    ['DELIVERED', []],
    ['FAILED', []],
  ] as const) {
    currentDetail = { ...detail, status };
    await page.reload();
    await expect(
      page.getByRole('heading', { level: 1, name: delivery.trackingNumber }),
    ).toBeVisible();
    if (next.length) {
      await expect(page.getByLabel('Next status')).toBeVisible();
      expect(
        await page
          .getByLabel('Next status')
          .locator('option')
          .evaluateAll((options) =>
            options.map((option) => option.getAttribute('value')),
          ),
      ).toEqual(['', ...next]);
    } else {
      await expect(page.getByLabel('Next status')).toHaveCount(0);
      await expect(
        page.getByText('This delivery has reached a final status'),
      ).toBeVisible();
    }
  }
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
  await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
  response = null;
  await page.getByRole('button', { name: 'Try again' }).click();
  await expect(
    page.getByRole('heading', { level: 1, name: delivery.trackingNumber }),
  ).toBeVisible();
});

test('validates proof fields and previews a supported image', async ({
  page,
}) => {
  const api = await signInAs(page, 'COURIER', {
    [`GET /courier/deliveries/${parcelId}`]: ok(detail),
  });
  await page.goto(`/deliveries/${parcelId}`);
  await page.getByRole('button', { name: 'Upload proof' }).click();
  await expect(
    page.getByText('Add a photo, recipient name or delivery note.'),
  ).toBeVisible();

  const photo = page.getByLabel('Proof photo');
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
    name: 'doorstep.png',
    mimeType: 'image/png',
    buffer: Buffer.from('png preview'),
  });
  await expect(page.getByText('Selected: doorstep.png')).toBeVisible();
  await expect(page.getByAltText('Selected proof preview')).toBeVisible();
  expect(api.calls.filter((call) => call.key.endsWith('/proof'))).toHaveLength(
    0,
  );
});

test('uploads authenticated multipart proof and refreshes its metadata', async ({
  page,
}) => {
  let uploaded = false;
  const uploadRequest: { contentType: string | undefined; body: string } = {
    contentType: undefined,
    body: '',
  };
  await signInAs(page, 'COURIER', {
    [`GET /courier/deliveries/${parcelId}`]: () =>
      ok(
        uploaded
          ? {
              ...detail,
              proof: {
                recipientName: 'Nimali Perera',
                note: 'Handed over safely',
                hasPhoto: true,
                createdAt: '2026-11-10T09:00:00Z',
              },
            }
          : { ...detail, proof: null },
      ),
  });
  await page.route(
    `**/api/v1/courier/deliveries/${parcelId}/proof`,
    async (route) => {
      const request = route.request();
      uploadRequest.contentType = request.headers()['content-type'];
      uploadRequest.body = request.postDataBuffer()?.toString('utf8') ?? '';
      uploaded = true;
      await route.fulfill({
        status: 201,
        contentType: 'application/json',
        body: JSON.stringify({
          data: {
            id: '00000000-0000-4000-8000-000000000201',
            parcelId,
            recipientName: 'Nimali Perera',
            note: 'Handed over safely',
            hasPhoto: true,
            createdAt: '2026-11-10T09:00:00Z',
          },
          meta: { requestId: REQUEST_ID },
        }),
      });
    },
  );
  await page.goto(`/deliveries/${parcelId}`);
  await page.getByLabel('Proof photo').setInputFiles({
    name: 'handover.jpg',
    mimeType: 'image/jpeg',
    buffer: Buffer.from('jpeg content'),
  });
  await page.getByLabel('Proof received by').fill('Nimali Perera');
  await page.getByLabel('Delivery note').fill('Handed over safely');
  await page.getByRole('button', { name: 'Upload proof' }).click();

  await expect(
    page.getByText('Delivery proof uploaded', { exact: true }),
  ).toBeVisible();
  await expect(page.getByText('Handed over safely')).toBeVisible();
  const contentType = uploadRequest.contentType ?? '';
  const body = uploadRequest.body;
  expect(contentType).toMatch(/^multipart\/form-data; boundary=/);
  expect(contentType).not.toContain('application/json');
  expect(body).toContain('name="photo"; filename="handover.jpg"');
  expect(body).toContain('name="recipientName"');
  expect(body).toContain('Nimali Perera');
  expect(body).toContain('name="note"');
  expect(body).toContain('Handed over safely');
});

test('retains proof input after failure, prevents duplicates and retries', async ({
  page,
}) => {
  let attempts = 0;
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  await signInAs(page, 'COURIER', {
    [`GET /courier/deliveries/${parcelId}`]: ok(detail),
  });
  await page.route(
    `**/api/v1/courier/deliveries/${parcelId}/proof`,
    async (route) => {
      attempts += 1;
      if (attempts === 1) {
        await route.fulfill({
          status: 503,
          contentType: 'application/json',
          body: JSON.stringify({
            error: { code: 'SERVICE_UNAVAILABLE', message: SERVER_WORDS },
            meta: { requestId: REQUEST_ID },
          }),
        });
        return;
      }
      await gate;
      await route.fulfill({
        status: 201,
        contentType: 'application/json',
        body: JSON.stringify({
          data: {
            id: '00000000-0000-4000-8000-000000000202',
            parcelId,
            recipientName: null,
            note: 'Retry this proof',
            hasPhoto: false,
            createdAt: '2026-11-10T09:05:00Z',
          },
          meta: { requestId: REQUEST_ID },
        }),
      });
    },
  );
  await page.goto(`/deliveries/${parcelId}`);
  await page.getByLabel('Delivery note').fill('Retry this proof');
  await page.getByRole('button', { name: 'Upload proof' }).click();
  await expect(
    page.getByText('Proof uploads are temporarily unavailable'),
  ).toBeVisible();
  await expect(page.getByLabel('Delivery note')).toHaveValue(
    'Retry this proof',
  );

  try {
    await page.getByRole('button', { name: 'Upload proof' }).click();
    const uploading = page.getByRole('button', { name: 'Uploading proof' });
    await expect(uploading).toHaveAttribute('aria-disabled', 'true');
    await uploading.evaluate((button) => {
      button.click();
      button.closest('form')?.requestSubmit();
    });
    expect(attempts).toBe(2);
  } finally {
    release();
  }
  await expect(
    page.getByText('Delivery proof uploaded', { exact: true }),
  ).toBeVisible();
  expect(attempts).toBe(2);
});

test('opens a short-lived proof photo when the delivery has one', async ({
  page,
  context,
}) => {
  const withPhoto = {
    ...detail,
    status: 'DELIVERED',
    proof: { ...detail.proof, hasPhoto: true },
  };
  await context.route('https://storage.example/**', (route) =>
    route.fulfill({ status: 200, contentType: 'image/jpeg', body: 'photo' }),
  );
  await signInAs(page, 'COURIER', {
    [`GET /courier/deliveries/${parcelId}`]: ok(withPhoto),
    [`GET /parcels/${parcelId}/proof/photo`]: ok({
      url: 'https://storage.example/proof.jpg',
      expiresAt: '2026-11-05T11:00:00Z',
      mimeType: 'image/jpeg',
      stream: false,
    }),
  });
  await page.goto(`/deliveries/${parcelId}`);
  const popupPromise = page.waitForEvent('popup');
  await page.getByRole('button', { name: 'View proof photo' }).click();
  const popup = await popupPromise;
  await popup.waitForURL('https://storage.example/proof.jpg');
});
