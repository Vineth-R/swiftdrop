import { randomUUID } from 'node:crypto';

import { expect, test } from '@playwright/test';

import { apiLogin, credentialsFor } from './support/real-stack';

type Zone = { id: string; name: string };
type Contact = {
  name: string;
  phone: string;
  address: string;
  city: string;
  zoneId: string;
};

const contact = (name: string, zone: Zone): Contact => ({
  name,
  phone: '+94771234567',
  address: '12 Test Lane',
  city: zone.name,
  zoneId: zone.id,
});

/**
 * Real stack: a customer creates a parcel and the public tracking page shows
 * it. The parcel is created through the API the booking form uses (zones, fee
 * estimate, then POST /parcels), so the test does not depend on how the form
 * is laid out. It looks for a pair of zones that has a price.
 */
test('a parcel the customer creates shows on the public tracking page', async ({
  page,
  request,
}) => {
  const credentials = credentialsFor('CUSTOMER');
  test.skip(!credentials, 'set the REAL_STACK_CUSTOMER_* variables');
  if (!credentials) return;

  const token = await apiLogin(request, credentials);
  const auth = { Authorization: `Bearer ${token}` };

  const zonesResponse = await request.get('/api/v1/zones');
  expect(zonesResponse.ok()).toBeTruthy();
  const zones = ((await zonesResponse.json()) as { data: Zone[] }).data;
  expect(zones.length).toBeGreaterThan(1);

  let route: { from: Zone; to: Zone; fee: number } | null = null;
  for (const from of zones) {
    for (const to of zones) {
      if (route || from.id === to.id) continue;
      const estimate = await request.post('/api/v1/pricing/estimate', {
        headers: auth,
        data: {
          originZoneId: from.id,
          destinationZoneId: to.id,
          weightKg: 1,
          sizeCategory: 'SMALL',
        },
      });
      if (estimate.ok()) {
        const fee = ((await estimate.json()) as { data: { total: number } })
          .data.total;
        route = { from, to, fee };
      }
    }
  }
  expect(route, 'no priced pair of zones in the seed data').not.toBeNull();
  if (!route) return;

  const created = await request.post('/api/v1/parcels', {
    headers: { ...auth, 'Idempotency-Key': randomUUID() },
    data: {
      sender: contact('Test Sender', route.from),
      recipient: contact('Test Recipient', route.to),
      weightKg: 1,
      sizeCategory: 'SMALL',
      expectedFee: route.fee,
    },
  });
  expect(created.status(), await created.text()).toBe(201);
  const { trackingNumber, status } = (
    (await created.json()) as {
      data: { trackingNumber: string; status: string };
    }
  ).data;
  expect(status).toBe('CREATED');

  // The public page: no sign-in.
  await page.goto('/track');
  await page.getByRole('textbox').fill(trackingNumber);
  await page.getByRole('button', { name: 'Track', exact: true }).click();

  await expect(page).toHaveURL(new RegExp(`/track\\?number=${trackingNumber}`));
  await expect(page.getByText('CREATED', { exact: true })).toBeVisible();
  await expect(
    page.getByRole('list', { name: 'Status history' }),
  ).toBeVisible();
});
