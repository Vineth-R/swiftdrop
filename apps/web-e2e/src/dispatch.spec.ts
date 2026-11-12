import { test, expect, type Page } from '@playwright/test';

import {
  fail,
  ok,
  REQUEST_ID,
  SERVER_WORDS,
  type Reply,
} from './support/mock-api';
import { signInAs } from './support/sign-in';

test.use({ colorScheme: 'light' });

const sampleParcel = {
  id: '00000000-0000-4000-8000-000000000001',
  trackingNumber: 'SDK7M2Q9XP4',
  status: 'CREATED',
  pickupAddress: '12 Temple Road',
  pickupCity: 'Colombo',
  pickupZoneId: '00000000-0000-4000-8000-000000000010',
  dropoffAddress: '45 Lake Drive',
  dropoffCity: 'Kandy',
  dropoffZoneId: '00000000-0000-4000-8000-000000000020',
  weightKg: 2.5,
  sizeCategory: 'SMALL',
  createdAt: '2026-11-02T10:00:00Z',
};

const sampleCourier = {
  id: '00000000-0000-4000-8000-0000000000c1',
  fullName: 'Kasun Silva',
  vehicleType: 'Motorcycle',
  isAvailable: true,
};

const sampleCourier2 = {
  id: '00000000-0000-4000-8000-0000000000c2',
  fullName: 'Amal Fernando',
  vehicleType: 'Van',
  isAvailable: true,
};

const sampleWorkload = [
  {
    courierId: '00000000-0000-4000-8000-0000000000c1',
    fullName: 'Kasun Silva',
    isAvailable: true,
    activeDeliveries: 4,
    byStatus: {
      CREATED: 1,
      PICKED_UP: 1,
      IN_TRANSIT: 1,
      OUT_FOR_DELIVERY: 1,
    },
  },
  {
    courierId: '00000000-0000-4000-8000-0000000000c2',
    fullName: 'Amal Fernando',
    isAvailable: false,
    activeDeliveries: 0,
    byStatus: {
      CREATED: 0,
      PICKED_UP: 0,
      IN_TRANSIT: 0,
      OUT_FOR_DELIVERY: 0,
    },
  },
];

const sampleAdminParcel = {
  id: '00000000-0000-4000-8000-000000000001',
  trackingNumber: 'SDK7M2Q9XP4',
  status: 'CREATED',
  pickupCity: 'Colombo',
  dropoffCity: 'Kandy',
  courierId: null,
  merchantId: null,
  customerId: '00000000-0000-4000-8000-0000000000u1',
  total: 450,
  createdAt: '2026-11-02T10:00:00Z',
};

function pageOf(data: unknown[], page = 1, totalItems = data.length): Reply {
  return {
    status: 200,
    body: {
      data,
      meta: {
        requestId: REQUEST_ID,
        pagination: {
          page,
          pageSize: 20,
          totalItems,
          totalPages: Math.ceil(totalItems / 20) || 1,
        },
      },
    },
  };
}

// Below 768px the same rows are cards; at the 1440px these tests use it is the table.
const unassignedTable = (page: Page) =>
  page.getByRole('table', { name: 'Unassigned parcels' });
const workloadTable = (page: Page) =>
  page.getByRole('table', { name: 'Courier workload' });
const parcelsTable = (page: Page) =>
  page.getByRole('table', { name: 'All parcels' });

test.describe('M6 Dispatcher screens', () => {
    test.beforeEach(async ({ page }) => {
    test.setTimeout(60000);
    await page.setViewportSize({ width: 1440, height: 900 });
    // The All parcels screen now opens a live socket. These tests run without
    // a backend, so refuse it quietly instead of reaching a real server.
    await page.routeWebSocket('**/ws', (ws) => ws.close({ code: 1011 }));
  });

  test.describe('Role Guard Protection (403)', () => {
    for (const role of ['CUSTOMER', 'COURIER'] as const) {
      test(`${role} cannot access /unassigned and sees 403`, async ({ page }) => {
        await signInAs(page, role);
        await page.goto('/unassigned');

        await expect(
          page.getByRole('heading', { name: 'You do not have access' }),
        ).toBeVisible();
        await expect(
          page.getByText('This page is for dispatchers and administrators'),
        ).toBeVisible();
        await expect(
          page.getByRole('link', { name: 'Back to my dashboard' }),
        ).toBeVisible();
      });

      test(`${role} cannot access /workload and sees 403`, async ({ page }) => {
        await signInAs(page, role);
        await page.goto('/workload');

        await expect(
          page.getByRole('heading', { name: 'You do not have access' }),
        ).toBeVisible();
        await expect(
          page.getByText('This page is for dispatchers and administrators'),
        ).toBeVisible();
        await expect(
          page.getByRole('link', { name: 'Back to my dashboard' }),
        ).toBeVisible();
      });
    }

    test('DISPATCHER can access /unassigned', async ({ page }) => {
      await signInAs(page, 'DISPATCHER', {
        'GET /dispatch/unassigned': pageOf([sampleParcel]),
        'GET /zones': ok([]),
      });

      await page.goto('/unassigned');
      await expect(
        page.getByRole('heading', { name: 'Unassigned parcels' }),
      ).toBeVisible();
    });

    test('DISPATCHER can access /workload', async ({ page }) => {
      await signInAs(page, 'DISPATCHER', {
        'GET /dispatch/workload': ok(sampleWorkload),
      });

      await page.goto('/workload');
      await expect(
        page.getByRole('heading', { name: 'Courier workload' }),
      ).toBeVisible();
    });
  });

  test.describe('Unassigned Parcels & Courier Assignment Flow', () => {
    test('renders list of unassigned parcels in the table', async ({ page }) => {
      await signInAs(page, 'DISPATCHER', {
        'GET /dispatch/unassigned': pageOf([sampleParcel]),
        'GET /zones': ok([]),
      });

      await page.goto('/unassigned');

      const row = unassignedTable(page)
        .getByRole('row')
        .filter({ hasText: 'SDK7M2Q9XP4' });
      await expect(row).toBeVisible({ timeout: 15000 });
      await expect(row).toContainText('Colombo');
      await expect(row).toContainText('Kandy');
      await expect(
        row.getByRole('button', { name: /assign/i }),
      ).toBeVisible();
    });

    test('shows loading skeleton while unassigned parcels load', async ({ page }) => {
      await signInAs(page, 'DISPATCHER', {
        'GET /dispatch/unassigned': pageOf([sampleParcel]),
        'GET /zones': ok([]),
      });

      let release!: () => void;
      const gate = new Promise<void>((resolve) => {
        release = resolve;
      });
      await page.route('**/api/v1/dispatch/unassigned**', async (route) => {
        await gate;
        await route.fallback();
      });

      await page.goto('/unassigned');
      await expect(
        page.getByText('Loading unassigned parcels'),
      ).toBeAttached();

      release();
      await expect(
        unassignedTable(page).getByText('SDK7M2Q9XP4'),
      ).toBeVisible();
      await expect(
        page.getByText('Loading unassigned parcels'),
      ).toHaveCount(0);
    });

    test('renders empty state when queue is clear', async ({ page }) => {
      await signInAs(page, 'DISPATCHER', {
        'GET /dispatch/unassigned': pageOf([]),
        'GET /zones': ok([]),
      });

      await page.goto('/unassigned');

      await expect(
        page.getByRole('heading', { name: 'No unassigned parcels' }),
      ).toBeVisible();
      await expect(
        page.getByText('All parcels in this queue have been assigned to couriers.'),
      ).toBeVisible();
    });

    test('renders error alert and retry button on 500 API failure', async ({ page }) => {
      let failRequest = true;
      await signInAs(page, 'DISPATCHER', {
        'GET /dispatch/unassigned': () =>
          failRequest ? fail(500, 'INTERNAL_ERROR') : pageOf([sampleParcel]),
        'GET /zones': ok([]),
      });

      await page.goto('/unassigned');

      await expect(
        page.getByText('Something broke on our side'),
      ).toBeVisible();
      // Must never expose raw server error text
      await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);

      const retryBtn = page.getByRole('button', { name: 'Try again' });
      await expect(retryBtn).toBeVisible();

      failRequest = false;
      await retryBtn.click();
      await expect(
        unassignedTable(page).getByText('SDK7M2Q9XP4'),
      ).toBeVisible();
    });

    test('renders rate-limited alert on 429 without exposing raw server words', async ({ page }) => {
      await signInAs(page, 'DISPATCHER', {
        'GET /dispatch/unassigned': fail(429, 'RATE_LIMITED', {
          headers: { 'Retry-After': '30' },
        }),
        'GET /zones': ok([]),
      });

      await page.goto('/unassigned');

      await expect(
        page.getByText('Too many attempts from this device'),
      ).toBeVisible();
      await expect(page.getByText(/Wait \d+ seconds and try again/)).toBeVisible();
      await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
    });

    test('opens assignment modal and submits courier assignment', async ({ page }) => {
      let assigned = false;
      const { calls } = await signInAs(page, 'DISPATCHER', {
        'GET /dispatch/unassigned': () =>
          assigned ? pageOf([]) : pageOf([sampleParcel]),
        'GET /zones': ok([]),
        'GET /couriers/': ok([sampleCourier, sampleCourier2]),
        'PUT /dispatch/parcels/00000000-0000-4000-8000-000000000001/assignment': {
          status: 200,
          body: {
            data: {
              parcelId: sampleParcel.id,
              trackingNumber: sampleParcel.trackingNumber,
              courierId: sampleCourier.id,
              assignedBy: 'user-dispatcher',
              assignedAt: '2026-11-02T10:15:00Z',
            },
            meta: { requestId: REQUEST_ID },
          },
        },
      });

      await page.goto('/unassigned');

      // Click assign button in table to open modal
      await unassignedTable(page).getByRole('button', { name: /assign/i }).click();

      const dialog = page.getByRole('dialog');
      await expect(
        dialog.getByRole('heading', { name: 'Assign courier' }),
      ).toBeVisible();
      await expect(
        dialog.getByText(`Assign courier to parcel ${sampleParcel.trackingNumber}.`),
      ).toBeVisible();

      // Verify courier options are listed
      const select = dialog.locator('#courier-select');
      await expect(select).toBeVisible();

      // Select courier inside modal
      await select.selectOption(sampleCourier.id);

      // Confirm assignment
      assigned = true;
      await dialog.getByRole('button', { name: 'Assign', exact: true }).click();

      // Modal closes and toast appears
      await expect(
        page.getByText(`Parcel ${sampleParcel.trackingNumber} assigned successfully.`),
      ).toBeVisible();

      // Verify PUT call was made with courierId
      const putCall = calls.find((c) =>
        c.key.startsWith('PUT /dispatch/parcels/'),
      );
      expect(putCall).toBeDefined();
      expect((putCall?.body as { courierId: string })?.courierId).toBe(
        sampleCourier.id,
      );
    });

    test('handles 409 PARCEL_CLOSED cleanly in modal and preserves user inputs', async ({ page }) => {
      await signInAs(page, 'DISPATCHER', {
        'GET /dispatch/unassigned': pageOf([sampleParcel]),
        'GET /zones': ok([]),
        'GET /couriers/': ok([sampleCourier]),
        'PUT /dispatch/parcels/00000000-0000-4000-8000-000000000001/assignment':
          fail(409, 'PARCEL_CLOSED'),
      });

      await page.goto('/unassigned');
      await unassignedTable(page).getByRole('button', { name: /assign/i }).click();

      const dialog = page.getByRole('dialog');
      await dialog.locator('#courier-select').selectOption(sampleCourier.id);
      await dialog.locator('#assign-reason').fill('Express delivery requested');
      await dialog.getByRole('button', { name: 'Assign', exact: true }).click();

      // Error alert visible in modal
      await expect(
        dialog.getByText('Parcel already closed'),
      ).toBeVisible();
      await expect(
        dialog.getByText(
          'This parcel is already delivered or closed and cannot be assigned or reassigned.',
        ),
      ).toBeVisible();
      await expect(dialog.getByText(SERVER_WORDS)).toHaveCount(0);

      // Verify user inputs are preserved
      await expect(dialog.locator('#courier-select')).toHaveValue(sampleCourier.id);
      await expect(dialog.locator('#assign-reason')).toHaveValue('Express delivery requested');
    });

    test('handles 422 COURIER_NOT_ELIGIBLE cleanly in modal and preserves user inputs', async ({ page }) => {
      await signInAs(page, 'DISPATCHER', {
        'GET /dispatch/unassigned': pageOf([sampleParcel]),
        'GET /zones': ok([]),
        'GET /couriers/': ok([sampleCourier]),
        'PUT /dispatch/parcels/00000000-0000-4000-8000-000000000001/assignment':
          fail(422, 'COURIER_NOT_ELIGIBLE'),
      });

      await page.goto('/unassigned');
      await unassignedTable(page).getByRole('button', { name: /assign/i }).click();

      const dialog = page.getByRole('dialog');
      await dialog.locator('#courier-select').selectOption(sampleCourier.id);
      await dialog.locator('#assign-reason').fill('Heavy parcel re-route');
      await dialog.getByRole('button', { name: 'Assign', exact: true }).click();

      // Error alert visible in modal
      await expect(
        dialog.getByText('Courier not eligible'),
      ).toBeVisible();
      await expect(
        dialog.getByText(
          'This courier is currently unavailable or ineligible for assignment. Please select another courier.',
        ),
      ).toBeVisible();
      await expect(dialog.getByText(SERVER_WORDS)).toHaveCount(0);

      // Verify user inputs are preserved
      await expect(dialog.locator('#courier-select')).toHaveValue(sampleCourier.id);
      await expect(dialog.locator('#assign-reason')).toHaveValue('Heavy parcel re-route');
    });
  });

  test.describe('Courier Workload Screen (/workload)', () => {
    test('renders courier rows, availability chips, and per-status delivery counts', async ({ page }) => {
      await signInAs(page, 'DISPATCHER', {
        'GET /dispatch/workload': ok(sampleWorkload),
      });

      await page.goto('/workload');

      const kasunRow = workloadTable(page)
        .getByRole('row')
        .filter({ hasText: 'Kasun Silva' });
      await expect(kasunRow).toBeVisible();
      await expect(kasunRow).toContainText('ACTIVE');
      await expect(kasunRow).toContainText('4');

      // Verify per-status delivery counts (CREATED, PICKED_UP, IN_TRANSIT, OUT_FOR_DELIVERY)
      const kasunCells = kasunRow.getByRole('cell');
      await expect(kasunCells.nth(3)).toHaveText('1'); // CREATED
      await expect(kasunCells.nth(4)).toHaveText('1'); // PICKED_UP
      await expect(kasunCells.nth(5)).toHaveText('1'); // IN_TRANSIT
      await expect(kasunCells.nth(6)).toHaveText('1'); // OUT_FOR_DELIVERY

      const amalRow = workloadTable(page)
        .getByRole('row')
        .filter({ hasText: 'Amal Fernando' });
      await expect(amalRow).toBeVisible();
      await expect(amalRow).toContainText('DEACTIVATED');
      await expect(amalRow.getByRole('cell').nth(2)).toHaveText('0');
    });

    test('filters couriers by availability', async ({ page }) => {
      await signInAs(page, 'DISPATCHER', {
        'GET /dispatch/workload': ok(sampleWorkload),
      });

      await page.goto('/workload');

      await page.getByLabel('Availability').selectOption('available');
      await expect(
        workloadTable(page).getByRole('row').filter({ hasText: 'Kasun Silva' }),
      ).toBeVisible();
      await expect(
        workloadTable(page).getByRole('row').filter({ hasText: 'Amal Fernando' }),
      ).toBeHidden();
    });
  });

  test.describe('All Parcels Search Screen (/parcels as DISPATCHER and ADMIN)', () => {
    for (const role of ['DISPATCHER', 'ADMIN'] as const) {
      test(`${role} can access /parcels and search parcels`, async ({ page }) => {
        await signInAs(page, role, {
          'GET /admin/parcels': pageOf([sampleAdminParcel]),
        });

        await page.goto('/parcels');

        await expect(
          page.getByRole('heading', { name: 'All parcels' }),
        ).toBeVisible();

        const row = parcelsTable(page)
          .getByRole('row')
          .filter({ hasText: sampleAdminParcel.trackingNumber });
        await expect(row).toBeVisible();
      });
    }

    test('filters parcels by tracking number and status', async ({ page }) => {
      const { calls } = await signInAs(page, 'DISPATCHER', {
        'GET /admin/parcels': pageOf([sampleAdminParcel]),
      });

      await page.goto('/parcels');

      await page.locator('#search-tracking-number').fill('SDK7M2Q9XP4');
      await page.locator('#filter-parcel-status').selectOption('CREATED');

      await expect
        .poll(() =>
          calls
            .filter((c) => c.key.startsWith('GET /admin/parcels'))
            .map((c) => {
              const u = new URL(c.url);
              return {
                trackingNumber: u.searchParams.get('trackingNumber'),
                status: u.searchParams.get('status'),
              };
            }),
        )
        .toContainEqual({
          trackingNumber: 'SDK7M2Q9XP4',
          status: 'CREATED',
        });
    });

    test('reassigns an active parcel with a reason', async ({ page }) => {
      const assignedParcel = {
        ...sampleAdminParcel,
        id: '00000000-0000-4000-8000-000000000002',
        trackingNumber: 'SDREASSIGN01',
        status: 'PICKED_UP',
        courierId: sampleCourier.id,
      };

      const { calls } = await signInAs(page, 'DISPATCHER', {
        'GET /admin/parcels': pageOf([assignedParcel]),
        'GET /couriers/': ok([sampleCourier, sampleCourier2]),
        'PUT /dispatch/parcels/00000000-0000-4000-8000-000000000002/assignment': {
          status: 200,
          body: {
            data: {
              parcelId: assignedParcel.id,
              trackingNumber: assignedParcel.trackingNumber,
              courierId: sampleCourier2.id,
              assignedBy: 'user-dispatcher',
              assignedAt: '2026-11-02T11:00:00Z',
            },
            meta: { requestId: REQUEST_ID },
          },
        },
      });

      await page.goto('/parcels');

      const row = parcelsTable(page)
        .getByRole('row')
        .filter({ hasText: 'SDREASSIGN01' });
      await expect(row).toBeVisible();

      // Click Reassign button
      await row.getByRole('button', { name: /reassign/i }).click();

      const dialog = page.getByRole('dialog');
      await expect(
        dialog.getByRole('heading', { name: 'Reassign courier' }),
      ).toBeVisible();

      // Select new courier and enter reason
      await dialog.locator('#courier-select').selectOption(sampleCourier2.id);
      await dialog
        .locator('#assign-reason')
        .fill('Reassigned due to vehicle breakdown');

      // Submit reassignment
      await dialog.getByRole('button', { name: 'Reassign', exact: true }).click();

      // Toast confirmation
      await expect(
        page.getByText('Parcel SDREASSIGN01 reassigned.'),
      ).toBeVisible();

      // Verify PUT payload included reason and new courier ID
      const putCall = calls.find((c) =>
        c.key.includes('/assignment'),
      );
      expect(putCall).toBeDefined();
      expect(putCall?.body).toEqual({
        courierId: sampleCourier2.id,
        reason: 'Reassigned due to vehicle breakdown',
      });
    });
  });
});
