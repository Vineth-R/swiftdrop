import {
  expect,
  test,
  type APIRequestContext,
  type Page,
} from '@playwright/test';

import {
  credentialsForRun,
  signInThroughForm,
  type Credentials,
} from './support/real-stack';

const REAL_TIMEOUT = 20_000;
const PASSWORD = 'Phase4Admin!2026';

function uniqueEmail(prefix: string) {
  return `${prefix}.${Date.now()}.${Math.floor(Math.random() * 10000)}@example.test`;
}

test.describe('real-stack admin mutations', () => {
  test.setTimeout(180_000);

  test('an admin creates, edits, deactivates and reactivates a user', async ({
    page,
    request,
  }) => {
    const admin = credentialsForRun('ADMIN');
    test.skip(!admin, 'set REAL_STACK_ADMIN_*');
    if (!admin) return;

    const suffix = Date.now();
    const name = `Phase Four Dispatcher ${suffix}`;
    const email = uniqueEmail('phase4.admin-user');
    await signIn(page, admin);
    await page.goto('/admin/users/new');

    await page.getByLabel('Full name').fill(name);
    await page.getByLabel('Email').fill(email);
    await page.getByLabel('Phone').fill('71 234 5678');
    await page.getByRole('radio', { name: /^Dispatcher/ }).check();
    await page.getByLabel('Initial password').fill(PASSWORD);
    await page.getByRole('button', { name: 'Create account' }).click();

    await expect(page).toHaveURL(/\/admin\/users$/);
    await expect(page.getByText(`${name}'s account was created.`)).toBeVisible({
      timeout: REAL_TIMEOUT,
    });
    await searchUsers(page, email);
    let row = userRow(page, email);
    await expect(row).toBeVisible({ timeout: REAL_TIMEOUT });

    await row.getByRole('link', { name: `Edit ${name}` }).click();
    await expect(
      page.getByRole('heading', { level: 1, name: 'Edit user' }),
    ).toBeVisible();
    await page.getByLabel('Full name').fill(`${name} Edited`);
    await page.getByRole('button', { name: 'Save changes' }).click();
    await expect(page).toHaveURL(/\/admin\/users$/);
    await expect(page.getByText(`${name} Edited was updated.`)).toBeVisible({
      timeout: REAL_TIMEOUT,
    });

    await searchUsers(page, email);
    row = userRow(page, email);
    await expect(row).toContainText(`${name} Edited`);
    await row
      .getByRole('button', { name: `Deactivate ${name} Edited` })
      .click();
    const dialog = page.getByRole('dialog', {
      name: `Deactivate ${name} Edited?`,
    });
    await dialog.getByRole('button', { name: 'Deactivate' }).click();
    await expect(row.getByText('DEACTIVATED')).toBeVisible({
      timeout: REAL_TIMEOUT,
    });

    const denied = await request.post('/api/v1/auth/login', {
      data: { email, password: PASSWORD },
    });
    expect(denied.ok()).toBeFalsy();

    await page.reload();
    await searchUsers(page, email);
    row = userRow(page, email);
    await row.getByRole('button', { name: `Activate ${name} Edited` }).click();
    await expect(row.getByText('ACTIVE')).toBeVisible({
      timeout: REAL_TIMEOUT,
    });
  });

  test('an admin approves, suspends and rejects merchants through the UI', async ({
    page,
    request,
  }) => {
    const admin = credentialsForRun('ADMIN');
    test.skip(!admin, 'set REAL_STACK_ADMIN_*');
    if (!admin) return;

    const zones = await loadZones(request);
    const colombo = zones.find((zone) => zone.code === 'CMB');
    if (!colombo) throw new Error('CMB zone is required');

    const suffix = Date.now();
    const approvedBusiness = `Phase Four Approved ${suffix}`;
    const approvedEmail = uniqueEmail('phase4.approved');
    const rejectedBusiness = `Phase Four Rejected ${suffix}`;
    const rejectedEmail = uniqueEmail('phase4.rejected');
    await registerMerchant(
      request,
      approvedEmail,
      approvedBusiness,
      colombo.id,
    );
    await registerMerchant(
      request,
      rejectedEmail,
      rejectedBusiness,
      colombo.id,
    );

    await signIn(page, admin);
    await page.goto('/admin/merchants');
    await searchMerchants(page, approvedBusiness);
    let row = page.getByRole('row').filter({ hasText: approvedBusiness });
    await expect(row).toBeVisible({ timeout: REAL_TIMEOUT });

    await row.getByRole('button', { name: 'Approve', exact: true }).click();
    await page
      .getByRole('dialog', { name: 'Approve application?' })
      .getByRole('button', { name: 'Approve' })
      .click();
    await expect(
      page.getByText('Merchant approved and activated.'),
    ).toBeVisible({
      timeout: REAL_TIMEOUT,
    });
    await expect(row.getByText('APPROVED', { exact: true })).toBeVisible({
      timeout: REAL_TIMEOUT,
    });

    await row.getByRole('button', { name: 'Suspend', exact: true }).click();
    const suspendDialog = page.getByRole('dialog', {
      name: 'Suspend merchant?',
    });
    await suspendDialog
      .getByLabel('Reason')
      .fill('Phase 4 suspension evidence.');
    await suspendDialog.getByRole('button', { name: 'Suspend' }).click();
    await expect(
      page.getByText('Merchant suspended and sessions invalidated.'),
    ).toBeVisible({ timeout: REAL_TIMEOUT });
    await expect(row.getByText('SUSPENDED', { exact: true })).toBeVisible({
      timeout: REAL_TIMEOUT,
    });

    const suspendedLogin = await request.post('/api/v1/auth/login', {
      data: { email: approvedEmail, password: 'Str0ngPass!' },
    });
    expect(suspendedLogin.ok()).toBeFalsy();

    await searchMerchants(page, rejectedBusiness);
    row = page.getByRole('row').filter({ hasText: rejectedBusiness });
    await row.getByRole('button', { name: 'Reject', exact: true }).click();
    const rejectDialog = page.getByRole('dialog', {
      name: 'Reject application?',
    });
    await rejectDialog
      .getByLabel('Reason (optional)')
      .fill('Phase 4 rejection evidence.');
    await rejectDialog.getByRole('button', { name: 'Reject' }).click();
    await expect(page.getByText('Merchant rejected.')).toBeVisible({
      timeout: REAL_TIMEOUT,
    });
    await expect(row.getByText('REJECTED', { exact: true })).toBeVisible({
      timeout: REAL_TIMEOUT,
    });

    await page.reload();
    await searchMerchants(page, approvedBusiness);
    await expect(
      page
        .getByRole('row')
        .filter({ hasText: approvedBusiness })
        .getByText('SUSPENDED', { exact: true }),
    ).toBeVisible({ timeout: REAL_TIMEOUT });
  });
});

async function signIn(page: Page, credentials: Credentials) {
  await signInThroughForm(page, credentials.email, credentials.password);
  await expect(page.locator('#login-email')).toHaveCount(0);
}

function userRow(page: Page, email: string) {
  return page.getByRole('row').filter({ hasText: email });
}

async function searchUsers(page: Page, query: string) {
  await page.locator('#user-search').fill(query);
  await page.getByRole('button', { name: 'Search' }).click();
  await expect(page.locator('#user-search')).toHaveValue(query);
}

async function searchMerchants(page: Page, query: string) {
  await page.locator('#merchant-search').fill(query);
  await page.getByRole('button', { name: 'Filter' }).click();
  await expect(page.locator('#merchant-search')).toHaveValue(query);
}

type Zone = { id: string; code: string };

async function loadZones(request: APIRequestContext): Promise<Zone[]> {
  const response = await request.get('/api/v1/zones');
  expect(response.ok()).toBeTruthy();
  return ((await response.json()) as { data: Zone[] }).data;
}

async function registerMerchant(
  request: APIRequestContext,
  email: string,
  businessName: string,
  zoneID: string,
) {
  const response = await request.post('/api/v1/merchants/register', {
    data: {
      fullName: 'Phase Four Merchant',
      email,
      phone: '+94771234567',
      password: 'Str0ngPass!',
      business: {
        name: businessName,
        registrationNumber: `PV ${Date.now()}`,
        contactEmail: email,
        contactPhone: '+94771234567',
        businessAddress: '88 Galle Road, Colombo 03',
        pickupAddress: '88 Galle Road, Colombo 03',
        pickupZoneId: zoneID,
      },
    },
  });
  expect(response.status(), await response.text()).toBe(201);
}
