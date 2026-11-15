import { randomUUID } from 'node:crypto';

import { expect, test } from '@playwright/test';

import {
  apiLogin,
  createParcel,
  credentialsForRun,
  signInThroughForm,
} from './support/real-stack';

const REAL_TIMEOUT = 20_000;

/**
 * Real stack: the dispatcher assigns and reassigns through the actual modal,
 * and the form rejects an empty courier selection. Parcel creation is fixture
 * setup; the assignment interactions under test stay in the browser.
 */
test('a dispatcher assigns and reassigns a parcel through the real UI', async ({
  page,
  request,
}) => {
  test.setTimeout(120_000);
  const customer = credentialsForRun('CUSTOMER');
  const dispatcher = credentialsForRun('DISPATCHER');
  test.skip(
    !customer || !dispatcher,
    'set REAL_STACK_CUSTOMER_* and REAL_STACK_DISPATCHER_*',
  );
  if (!customer || !dispatcher) return;

  const customerToken = await apiLogin(
    request,
    customer.email,
    customer.password,
  );
  const first = await createParcel(request, customerToken, randomUUID());
  const second = await createParcel(request, customerToken, randomUUID());

  await signIn(page, dispatcher);
  await page.goto('/parcels');
  await expect(
    page.getByText('Live. Parcel statuses change here as they happen.', {
      exact: true,
    }),
  ).toBeVisible({ timeout: REAL_TIMEOUT });

  const table = page.getByRole('table', { name: 'All parcels' });
  const rowFor = (trackingNumber: string) =>
    table.getByRole('row').filter({ hasText: trackingNumber });

  await search(page, first.trackingNumber);
  const firstRow = rowFor(first.trackingNumber);
  await expect(firstRow).toBeVisible({ timeout: REAL_TIMEOUT });
  await firstRow
    .getByRole('button', {
      name: `Assign courier to parcel ${first.trackingNumber}`,
    })
    .click();

  const assignDialog = page.getByRole('dialog');
  await expect(
    assignDialog.getByRole('heading', { name: 'Assign courier' }),
  ).toBeVisible();
  await assignDialog.getByLabel('Courier').selectOption({ index: 1 });
  await assignDialog
    .getByRole('button', { name: 'Assign', exact: true })
    .click();
  await expect(
    page.getByText(`Parcel ${first.trackingNumber} assigned successfully.`),
  ).toBeVisible({ timeout: REAL_TIMEOUT });
  await expect(
    firstRow.getByRole('button', {
      name: `Reassign parcel ${first.trackingNumber}`,
    }),
  ).toBeVisible({ timeout: REAL_TIMEOUT });

  await firstRow
    .getByRole('button', {
      name: `Reassign parcel ${first.trackingNumber}`,
    })
    .click();
  const reassignDialog = page.getByRole('dialog');
  await expect(
    reassignDialog.getByRole('heading', { name: 'Reassign courier' }),
  ).toBeVisible();
  await reassignDialog.getByLabel('Courier').selectOption({ index: 2 });
  await reassignDialog
    .getByRole('button', { name: 'Reassign', exact: true })
    .click();
  await expect(
    page.getByText(`Parcel ${first.trackingNumber} reassigned.`),
  ).toBeVisible({ timeout: REAL_TIMEOUT });

  await search(page, second.trackingNumber);
  const secondRow = rowFor(second.trackingNumber);
  await expect(secondRow).toBeVisible({ timeout: REAL_TIMEOUT });
  await secondRow
    .getByRole('button', {
      name: `Assign courier to parcel ${second.trackingNumber}`,
    })
    .click();
  const failureDialog = page.getByRole('dialog');
  await failureDialog
    .getByRole('button', { name: 'Assign', exact: true })
    .click();
  await expect(
    failureDialog.getByText('Please select a courier'),
  ).toBeVisible();
});

async function signIn(
  page: import('@playwright/test').Page,
  credentials: { email: string; password: string },
) {
  await signInThroughForm(page, credentials.email, credentials.password);
  await expect(page.locator('#login-email')).toHaveCount(0);
}

async function search(
  page: import('@playwright/test').Page,
  trackingNumber: string,
) {
  const searchField = page.locator('#search-tracking-number');
  await searchField.fill(trackingNumber);
  await expect(
    page.getByRole('table', { name: 'All parcels' }).getByRole('row').filter({
      hasText: trackingNumber,
    }),
  ).toBeVisible({ timeout: REAL_TIMEOUT });
}
