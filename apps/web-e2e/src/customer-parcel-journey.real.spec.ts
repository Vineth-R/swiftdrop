import { readFileSync } from 'node:fs';

import { expect, test } from '@playwright/test';

import { credentialsForRun, signInThroughForm } from './support/real-stack';

const REAL_TIMEOUT = 20_000;

/**
 * Real stack: the customer uses the actual booking wizard, finds the parcel in
 * My parcels, opens its detail/history and downloads the real PDF receipt.
 *
 * The test deliberately does not create the parcel through the API. The wizard,
 * list, detail, history and receipt route are the behaviour under test.
 */
test('a customer creates, finds and opens a parcel through the real UI', async ({
  page,
}) => {
  test.setTimeout(120_000);
  const customer = credentialsForRun('CUSTOMER');
  test.skip(!customer, 'set REAL_STACK_CUSTOMER_*');
  if (!customer) return;

  const suffix = Date.now();
  const recipient = `Journey Recipient ${suffix}`;

  await signInThroughForm(page, customer.email, customer.password);
  await expect(page.locator('#login-email')).toHaveCount(0);

  await page.goto('/new');
  await expect(
    page.getByRole('heading', { level: 1, name: 'New delivery' }),
  ).toBeVisible({ timeout: REAL_TIMEOUT });

  await page
    .getByLabel('Pickup zone', { exact: false })
    .selectOption({ label: 'Colombo' });
  await page
    .getByLabel('Destination zone', { exact: false })
    .selectOption({ label: 'Kandy' });
  await page.getByLabel('Weight', { exact: false }).fill('2');
  await page.getByLabel('Size category').selectOption('SMALL');

  // The button stays disabled until the real quote returns.
  await expect(page.getByText(/^LKR [\d,]+\.\d{2}$/).first()).toBeVisible({
    timeout: REAL_TIMEOUT,
  });
  await page.getByRole('button', { name: 'Continue', exact: true }).click();

  for (const [label, value] of Object.entries({
    'Sender name': `Journey Sender ${suffix}`,
    'Sender phone': '+94771234567',
    'Sender address': '12 Temple Road',
    'Sender city': 'Colombo',
    'Recipient name': recipient,
    'Recipient phone': '+94712345678',
    'Recipient address': '45 Lake Drive',
    'Recipient city': 'Kandy',
  })) {
    await page.getByLabel(label, { exact: false }).fill(value);
  }
  await page
    .getByLabel('Description (optional)', { exact: false })
    .fill('Phase 4 customer journey');
  await page.getByRole('button', { name: 'Continue', exact: true }).click();

  await expect(
    page.getByRole('heading', { level: 2, name: 'Review delivery' }),
  ).toBeVisible();
  await page.getByRole('button', { name: 'Create delivery' }).click();

  await expect(
    page.getByRole('heading', { level: 1, name: 'New delivery' }),
  ).toBeVisible();
  await expect(page.getByText('Ready for collection')).toBeVisible({
    timeout: REAL_TIMEOUT,
  });
  const trackingNumber = await page
    .getByText(/^SD[KN][A-Z0-9]{8}$/)
    .first()
    .innerText();
  expect(trackingNumber).toMatch(/^SD[KN][A-Z0-9]{8}$/);

  await page
    .getByRole('main')
    .getByRole('link', { name: 'My parcels' })
    .click();
  await expect(
    page.getByRole('heading', { level: 1, name: 'My parcels' }),
  ).toBeVisible({ timeout: REAL_TIMEOUT });

  const card = page.locator('li').filter({ hasText: recipient }).first();
  await expect(card).toBeVisible({ timeout: REAL_TIMEOUT });
  await expect(card).toContainText(trackingNumber);
  await card
    .getByRole('link', { name: `View parcel ${trackingNumber}` })
    .click();

  await expect(
    page.getByRole('heading', { level: 1, name: 'Parcel details' }),
  ).toBeVisible({ timeout: REAL_TIMEOUT });
  await expect(page.getByText(trackingNumber, { exact: true })).toBeVisible();
  const history = page.getByRole('list', { name: 'Delivery history' });
  await expect(history).toBeVisible();
  await expect(history.getByRole('listitem').first()).toContainText('CREATED');

  const downloadPromise = page.waitForEvent('download');
  await page.getByRole('button', { name: 'Download PDF receipt' }).click();
  const download = await downloadPromise;
  expect(download.suggestedFilename()).toBe(`receipt_${trackingNumber}.pdf`);
  const path = await download.path();
  expect(path).toBeTruthy();
  if (!path) throw new Error('receipt download has no local path');
  const pdf = readFileSync(path);
  expect(pdf.subarray(0, 5).toString()).toBe('%PDF-');
});
