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

const sampleWorkload = [
  {
    courierId: '00000000-0000-4000-8000-0000000000c1',
    fullName: 'Kasun Silva',
    isAvailable: true,
    activeDeliveries: 3,
    byStatus: {
      CREATED: 1,
      PICKED_UP: 2,
    },
  },
  {
    courierId: '00000000-0000-4000-8000-0000000000c2',
    fullName: 'Amal Fernando',
    isAvailable: false,
    activeDeliveries: 0,
    byStatus: {},
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
    await page.setViewportSize({ width: 1440, height: 900 });
  });

  test.describe('Role protection', () => {
    test('non-dispatcher roles cannot access /unassigned', async ({ page }) => {
      await signInAs(page, 'CUSTOMER');
      await page.goto('/unassigned');

      await expect(
        page.getByRole('heading', { name: 'Access restricted' }),
      ).toBeVisible();
    });

    test('non-dispatcher roles cannot access /workload', async ({ page }) => {
      await signInAs(page, 'COURIER');
      await page.goto('/workload');

      await expect(
        page.getByRole('heading', { name: 'Access restricted' }),
      ).toBeVisible();
    });

    test('DISPATCHER can access /unassigned and /workload', async ({ page }) => {
      await signInAs(page, 'DISPATCHER', {
        'GET /dispatch/unassigned': pageOf([sampleParcel]),
        'GET /zones': ok([]),
        'GET /dispatch/workload': ok(sampleWorkload),
      });

      await page.goto('/unassigned');
      await expect(
        page.getByRole('heading', { name: 'Unassigned parcels' }),
      ).toBeVisible();

      await page.goto('/workload');
      await expect(
        page.getByRole('heading', { name: 'Courier workload' }),
      ).toBeVisible();
    });
  });

  test.describe('Unassigned parcels screen', () => {
    test('renders list of unassigned parcels', async ({ page }) => {
      await signInAs(page, 'DISPATCHER', {
        'GET /dispatch/unassigned': pageOf([sampleParcel]),
        'GET /zones': ok([]),
      });

      await page.goto('/unassigned');

      const row = unassignedTable(page)
        .getByRole('row')
        .filter({ hasText: 'SDK7M2Q9XP4' });
      await expect(row).toBeVisible();
      await expect(row).toContainText('Colombo');
      await expect(row).toContainText('Kandy');
      await expect(
        row.getByRole('button', { name: /assign/i }),
      ).toBeVisible();
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

    test('renders error alert and retry button on API failure', async ({ page }) => {
      await signInAs(page, 'DISPATCHER', {
        'GET /dispatch/unassigned': fail(500, 'INTERNAL_ERROR'),
        'GET /zones': ok([]),
      });

      await page.goto('/unassigned');

      await expect(
        page.getByText('Something went wrong on our side.'),
      ).toBeVisible();
      // Must never expose raw server error text
      await expect(page.getByText(SERVER_WORDS)).toBeHidden();

      const retryBtn = page.getByRole('button', { name: /retry/i });
      await expect(retryBtn).toBeVisible();
    });

    test('opens assignment modal and submits courier assignment', async ({ page }) => {
      let assigned = false;
      await signInAs(page, 'DISPATCHER', {
        'GET /dispatch/unassigned': () =>
          assigned ? pageOf([]) : pageOf([sampleParcel]),
        'GET /zones': ok([]),
        'GET /couriers': ok([sampleCourier]),
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

      // Select courier inside modal
      await dialog.locator('#courier-select').selectOption(sampleCourier.id);

      // Confirm assignment
      assigned = true;
      await dialog.getByRole('button', { name: 'Assign', exact: true }).click();

      // Modal closes and toast appears
      await expect(
        page.getByText(`Parcel ${sampleParcel.trackingNumber} assigned successfully.`),
      ).toBeVisible();
    });

    test('handles 409 PARCEL_CLOSED cleanly in modal without server words', async ({ page }) => {
      await signInAs(page, 'DISPATCHER', {
        'GET /dispatch/unassigned': pageOf([sampleParcel]),
        'GET /zones': ok([]),
        'GET /couriers': ok([sampleCourier]),
        'PUT /dispatch/parcels/00000000-0000-4000-8000-000000000001/assignment':
          fail(409, 'PARCEL_CLOSED'),
      });

      await page.goto('/unassigned');
      await unassignedTable(page).getByRole('button', { name: /assign/i }).click();

      const dialog = page.getByRole('dialog');
      await dialog.locator('#courier-select').selectOption(sampleCourier.id);
      await dialog.getByRole('button', { name: 'Assign', exact: true }).click();

      // Error alert visible in modal
      await expect(
        dialog.getByText('This parcel is in a closed state and can no longer be assigned.'),
      ).toBeVisible();
      await expect(page.getByText(SERVER_WORDS)).toBeHidden();
    });
  });

  test.describe('Courier workload screen', () => {
    test('renders courier workload with availability and delivery counts', async ({ page }) => {
      await signInAs(page, 'DISPATCHER', {
        'GET /dispatch/workload': ok(sampleWorkload),
      });

      await page.goto('/workload');

      const kasunRow = workloadTable(page)
        .getByRole('row')
        .filter({ hasText: 'Kasun Silva' });
      await expect(kasunRow).toBeVisible();
      await expect(kasunRow).toContainText('ACTIVE');
      await expect(kasunRow).toContainText('3');

      const amalRow = workloadTable(page)
        .getByRole('row')
        .filter({ hasText: 'Amal Fernando' });
      await expect(amalRow).toBeVisible();
      await expect(amalRow).toContainText('DEACTIVATED');
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

  test.describe('Dispatcher parcels screen', () => {
    test('renders administrative parcel search list for dispatcher', async ({ page }) => {
      await signInAs(page, 'DISPATCHER', {
        'GET /admin/parcels': pageOf([sampleAdminParcel]),
      });

      await page.goto('/parcels');

      await expect(
        page.getByRole('heading', { name: 'All parcels' }),
      ).toBeVisible();

      const row = parcelsTable(page)
        .getByRole('row')
        .filter({ hasText: 'SDK7M2Q9XP4' });
      await expect(row).toBeVisible();
      await expect(
        row.getByRole('button', { name: /assign/i }),
      ).toBeVisible();
    });
  });
});
