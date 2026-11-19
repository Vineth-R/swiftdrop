import { expect, test } from '@playwright/test';

import { fail, REQUEST_ID } from './support/mock-api';
import { signInAs } from './support/sign-in';

test.use({ colorScheme: 'light' });

const entry = {
  id: 'audit-1',
  actorUserId: '3f1c7a52-0000-4000-8000-0000000000c3',
  action: 'USER_UPDATED',
  entityType: 'user',
  entityId: '3f1c7a52-0000-4000-8000-0000000000d4',
  details: { role: 'COURIER', password: '[redacted]' },
  createdAt: '2026-11-03T08:00:00Z',
};

const list = {
  status: 200,
  body: {
    data: [entry],
    meta: {
      requestId: REQUEST_ID,
      pagination: { page: 1, pageSize: 20, totalItems: 1, totalPages: 1 },
    },
  },
};

test.beforeEach(async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
});

test('ADMIN sees audit records and redacted details', async ({ page }) => {
  await signInAs(page, 'ADMIN', {
    'GET /admin/audit-logs/identity': list,
  });
  await page.goto('/admin/audit-logs');

  await expect(
    page.getByRole('heading', { level: 1, name: 'Audit logs' }),
  ).toBeVisible();
  const table = page.getByRole('table', { name: 'Administrative audit logs' });
  await expect(table.getByText('USER_UPDATED')).toBeVisible();
  await table.getByText('Show details').click();
  await expect(table.getByText('[redacted]')).toBeVisible();
  await expect(page.getByText('never-return-this')).toHaveCount(0);
});

test('a refused audit read says administrator access is required', async ({
  page,
}) => {
  await signInAs(page, 'ADMIN', {
    'GET /admin/audit-logs/identity': fail(403, 'FORBIDDEN'),
  });
  await page.goto('/admin/audit-logs');
  await expect(
    page.getByText('Administrator access is required.'),
  ).toBeVisible();
});
