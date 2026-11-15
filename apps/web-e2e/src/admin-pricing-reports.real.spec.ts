import {
  expect,
  test,
  type APIRequestContext,
  type Page,
} from '@playwright/test';

import {
  apiLogin,
  credentialsForRun,
  signInThroughForm,
  type Credentials,
} from './support/real-stack';

const REAL_TIMEOUT = 20_000;

type Zone = { id: string; code: string; name: string };

test.describe('real-stack pricing administration and reports', () => {
  test.setTimeout(240_000);

  test('an admin creates and updates pricing, then reports include real parcel data', async ({
    page,
    request,
  }) => {
    const admin = credentialsForRun('ADMIN');
    const customer = credentialsForRun('CUSTOMER');
    test.skip(
      !admin || !customer,
      'set REAL_STACK_ADMIN_* and REAL_STACK_CUSTOMER_*',
    );
    if (!admin || !customer) return;

    const suffix = Date.now().toString().slice(-6);
    const zoneCode = `P4${suffix}`;
    const zoneName = `Phase Four Zone ${suffix}`;
    const updatedZoneName = `${zoneName} Updated`;
    const customerToken = await apiLogin(
      request,
      customer.email,
      customer.password,
    );
    const adminToken = await apiLogin(request, admin.email, admin.password);
    const existingBands = await loadBands(request, adminToken);
    const bandMin =
      Math.max(25, ...existingBands.map((band) => band.maxWeightKg)) + 1;
    const bandMax = bandMin + 1;

    await signIn(page, admin);
    await page.goto('/admin/zones');
    await page.getByRole('button', { name: 'Add zone', exact: true }).click();
    let dialog = page.getByRole('dialog', { name: 'Add a zone' });
    await dialog.getByLabel('Code').fill(zoneCode);
    await dialog.getByLabel('Name').fill(zoneName);
    await dialog.getByLabel('Districts').fill('Colombo');
    await dialog.getByRole('button', { name: 'Add zone' }).click();
    await expect(page.getByText(`${zoneName} added.`)).toBeVisible({
      timeout: REAL_TIMEOUT,
    });

    let zoneRow = page.getByRole('row').filter({ hasText: zoneName });
    await zoneRow.getByRole('button', { name: `Edit ${zoneName}` }).click();
    dialog = page.getByRole('dialog', { name: `Edit ${zoneName}` });
    await dialog.getByLabel('Name').fill(updatedZoneName);
    await dialog.getByRole('button', { name: 'Save zone' }).click();
    await expect(page.getByText(`${updatedZoneName} saved.`)).toBeVisible({
      timeout: REAL_TIMEOUT,
    });
    zoneRow = page.getByRole('row').filter({ hasText: updatedZoneName });
    await expect(zoneRow).toBeVisible();

    const zones = await loadZones(request);
    const colombo = zones.find((zone) => zone.code === 'CMB');
    const createdZone = zones.find((zone) => zone.code === zoneCode);
    if (!colombo || !createdZone)
      throw new Error('pricing zones were not loaded');
    expect(createdZone.name).toBe(updatedZoneName);

    await page.goto('/admin/pricing');
    await page.getByRole('button', { name: 'Add rule', exact: true }).click();
    dialog = page.getByRole('dialog', { name: 'Add a rule' });
    await dialog.getByLabel('From').selectOption({ label: colombo.name });
    await dialog.getByLabel('To').selectOption({ label: createdZone.name });
    await dialog.getByLabel('Base fee').fill('550');
    await dialog.getByRole('button', { name: 'Add rule' }).click();
    await expect(
      page.getByText(`Rule added for ${colombo.name} to ${createdZone.name}.`),
    ).toBeVisible({ timeout: REAL_TIMEOUT });
    await page.locator('#rule-destination-filter').selectOption(createdZone.id);
    await expect(
      page.getByRole('table', { name: 'Pricing rules' }),
    ).toContainText(createdZone.name, { timeout: REAL_TIMEOUT });

    let ruleRow = page
      .getByRole('row')
      .filter({ hasText: `${colombo.name}` })
      .filter({ hasText: createdZone.name });
    await expect(ruleRow).toContainText('LKR 550.00');
    await ruleRow
      .getByRole('button', {
        name: `Edit ${colombo.name} to ${createdZone.name}`,
      })
      .click();
    dialog = page.getByRole('dialog', {
      name: `Edit ${colombo.name} to ${createdZone.name}`,
    });
    await dialog.getByLabel('Base fee').fill('600');
    await dialog.getByRole('button', { name: 'Save rule' }).click();
    ruleRow = page
      .getByRole('row')
      .filter({ hasText: `${colombo.name}` })
      .filter({ hasText: createdZone.name });
    await expect(ruleRow).toContainText('LKR 600.00', {
      timeout: REAL_TIMEOUT,
    });

    await page.getByRole('button', { name: 'Add band' }).click();
    dialog = page.getByRole('dialog', { name: 'Add a weight band' });
    await dialog.getByLabel('Over').fill(String(bandMin));
    await dialog.getByLabel('Up to').fill(String(bandMax));
    await dialog.getByLabel('Charge').fill('1000');
    await dialog.getByRole('button', { name: 'Add band' }).click();
    await expect(page.getByText(/band is added\./)).toBeVisible({
      timeout: REAL_TIMEOUT,
    });

    await page
      .getByRole('button', {
        name: new RegExp(`Edit the ${bandMin}\\.00 kg to ${bandMax}\\.00 kg`),
      })
      .click();
    dialog = page.getByRole('dialog', { name: 'Edit weight band' });
    await expect(dialog.getByLabel('Charge')).toHaveValue('1000.00');
    await dialog.getByLabel('Charge').fill('1200');
    await expect(dialog.getByLabel('Charge')).toHaveValue('1200');
    await expect(
      dialog.getByRole('button', { name: 'Save band' }),
    ).toBeEnabled();
    await dialog.getByRole('button', { name: 'Save band' }).click();
    await expect(page.getByText(/band is saved\./)).toBeVisible({
      timeout: REAL_TIMEOUT,
    });
    await expect(
      page.getByRole('row').filter({ hasText: `${bandMax.toFixed(2)} kg` }),
    ).toContainText('LKR 1,200.00', { timeout: REAL_TIMEOUT });

    await page
      .getByRole('row')
      .filter({ hasText: 'Small' })
      .getByRole('button', { name: 'Edit the small surcharge' })
      .click();
    dialog = page.getByRole('dialog', { name: 'Edit the small surcharge' });
    await dialog.getByLabel('Surcharge').fill('50');
    await dialog.getByRole('button', { name: 'Save surcharge' }).click();
    await expect(page.getByText('Small surcharge saved.')).toBeVisible({
      timeout: REAL_TIMEOUT,
    });

    const unauthorized = await request.post('/api/v1/admin/zones', {
      headers: { Authorization: `Bearer ${customerToken}` },
      data: { code: 'NOPE', name: 'Not allowed' },
    });
    expect(unauthorized.status()).toBe(403);

    const estimate = await request.post('/api/v1/pricing/estimate', {
      headers: { Authorization: `Bearer ${customerToken}` },
      data: {
        originZoneId: colombo.id,
        destinationZoneId: createdZone.id,
        weightKg: bandMax,
        sizeCategory: 'SMALL',
      },
    });
    expect(estimate.ok(), await estimate.text()).toBeTruthy();
    const fee = ((await estimate.json()) as { data: { total: number } }).data
      .total;
    expect(fee).toBe(1850);

    const created = await request.post('/api/v1/parcels', {
      headers: {
        Authorization: `Bearer ${customerToken}`,
        'Idempotency-Key': crypto.randomUUID(),
      },
      data: {
        sender: contact('Pricing Sender', colombo),
        recipient: contact('Pricing Recipient', createdZone),
        weightKg: bandMax,
        sizeCategory: 'SMALL',
        expectedFee: fee,
      },
    });
    expect(created.status(), await created.text()).toBe(201);

    const today = new Date().toISOString().slice(0, 10);
    const dailyResponse = await request.get(
      `/api/v1/reports/daily?from=${today}&to=${today}`,
      { headers: { Authorization: `Bearer ${adminToken}` } },
    );
    expect(dailyResponse.ok()).toBeTruthy();
    const daily = (
      (await dailyResponse.json()) as {
        data: Array<{ date: string; created: number }>;
      }
    ).data;
    expect(
      daily.find((row) => row.date === today)?.created ?? 0,
    ).toBeGreaterThan(0);

    await page.goto('/reports');
    await expect(
      page.getByRole('heading', { level: 1, name: 'Reports' }),
    ).toBeVisible({ timeout: REAL_TIMEOUT });
    await expect(
      page.getByRole('heading', { level: 2, name: 'Outcomes' }),
    ).toBeVisible({ timeout: REAL_TIMEOUT });
    await expect(
      page.getByRole('heading', { level: 2, name: 'Daily parcels' }),
    ).toBeVisible({ timeout: REAL_TIMEOUT });
    await expect(page.getByText('No parcels in this range')).toHaveCount(0);
    const dateLabel = new Intl.DateTimeFormat('en-GB', {
      dateStyle: 'medium',
      timeZone: 'UTC',
    }).format(new Date(`${today}T00:00:00Z`));
    await expect(
      page
        .getByRole('table', { name: 'Daily parcels' })
        .getByText(dateLabel)
        .first(),
    ).toBeVisible({ timeout: REAL_TIMEOUT });

    await page.goto('/admin/pricing');
    await page
      .getByRole('row')
      .filter({ hasText: 'Small' })
      .getByRole('button', { name: 'Edit the small surcharge' })
      .click();
    dialog = page.getByRole('dialog', { name: 'Edit the small surcharge' });
    await dialog.getByLabel('Surcharge').fill('0');
    await dialog.getByRole('button', { name: 'Save surcharge' }).click();
    await expect(page.getByText('Small surcharge saved.')).toBeVisible({
      timeout: REAL_TIMEOUT,
    });
  });
});

function contact(name: string, zone: Zone) {
  return {
    name,
    phone: '+94771234567',
    address: '12 Temple Road',
    city: zone.name,
    zoneId: zone.id,
  };
}

async function signIn(page: Page, credentials: Credentials) {
  await signInThroughForm(page, credentials.email, credentials.password);
  await expect(page.locator('#login-email')).toHaveCount(0);
}

async function loadZones(request: APIRequestContext): Promise<Zone[]> {
  const response = await request.get('/api/v1/zones');
  expect(response.ok()).toBeTruthy();
  return ((await response.json()) as { data: Zone[] }).data;
}

async function loadBands(
  request: APIRequestContext,
  token: string,
): Promise<Array<{ maxWeightKg: number }>> {
  const response = await request.get(
    '/api/v1/admin/pricing/bands?pageSize=100',
    {
      headers: { Authorization: `Bearer ${token}` },
    },
  );
  expect(response.ok()).toBeTruthy();
  return ((await response.json()) as { data: Array<{ maxWeightKg: number }> })
    .data;
}
