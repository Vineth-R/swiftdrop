import { expect, test } from '@playwright/test';

import { credentialsForRun, signInThroughForm } from './support/real-stack';

test('an administrator reads audit records from the real services', async ({
  page,
}) => {
  const admin = credentialsForRun('ADMIN');
  test.skip(!admin, 'set REAL_STACK_ADMIN_EMAIL and REAL_STACK_ADMIN_PASSWORD');
  if (!admin) return;

  await signInThroughForm(page, admin.email, admin.password);
  await page.goto('/admin/audit-logs');
  await expect(
    page.getByRole('heading', { level: 1, name: 'Audit logs' }),
  ).toBeVisible();
  const table = page.getByRole('table', { name: 'Administrative audit logs' });
  await expect(table).toBeVisible();
  await expect(table.getByRole('row').first()).toBeVisible();
});
