import { expect, test } from '@playwright/test';

import { fail, mockApi, ok, signedOut } from './support/mock-api';
import { signInAs } from './support/sign-in';

const merchant = {
  id: '3f1c7a52-0000-4000-8000-0000000000m1'.replace('m', 'a'),
  ownerUserId: '3f1c7a52-0000-4000-8000-0000000000u1'.replace('u', 'b'),
  businessName: 'ShopZone Lanka',
  registrationNumber: 'PV 123456',
  businessAddress: '88 Galle Road, Colombo 03',
  contactEmail: 'support@shopzone.lk',
  contactPhone: '+94112223344',
  pickupAddress: '88 Galle Road, Colombo 03',
  pickupZoneId: '7a1e0000-0000-4000-8000-000000000001',
  status: 'PENDING' as const,
  createdAt: '2026-11-04T08:00:00Z',
};

test.use({ colorScheme: 'light' });

test.describe('merchant registration', () => {
  test('submits a complete application without signing in', async ({
    page,
  }) => {
    const { calls } = await mockApi(page, {
      ...signedOut,
      'GET /zones': ok([
        { id: merchant.pickupZoneId, code: 'CMB', name: 'Colombo' },
      ]),
      'POST /merchants/register': ok(
        {
          merchantId: merchant.id,
          userId: merchant.ownerUserId,
          status: 'PENDING',
        },
        201,
      ),
    });
    await page.goto('/register/merchant');
    await page.getByLabel('Full name').fill('Ruwan Jayasuriya');
    await page.locator('#merchant-email').fill('owner@shop.lk');
    await page.getByLabel('Phone number').fill('+94773334455');
    await page.locator('#merchant-password').fill('Str0ngPass!');
    await page.getByLabel('Confirm password').fill('Str0ngPass!');
    await page.getByLabel('Business name').fill('ShopZone Lanka');
    await page.getByLabel('Business registration number').fill('PV 123456');
    await page.getByLabel('Business address').fill('88 Galle Road, Colombo 03');
    await page
      .getByLabel('Default pickup address')
      .fill('88 Galle Road, Colombo 03');
    await page
      .getByLabel('Default pickup zone')
      .selectOption(merchant.pickupZoneId);
    await page.getByRole('button', { name: 'Submit application' }).click();

    await expect(page.getByText('Application submitted')).toBeVisible();
    await expect(
      page.getByText('You cannot sign in until the application is approved.'),
    ).toBeVisible();
    expect(calls.map((call) => call.key)).not.toContain('POST /auth/login');
    const register = calls.find(
      (call) => call.key === 'POST /merchants/register',
    );
    expect(register?.body).toMatchObject({
      email: 'owner@shop.lk',
      business: { name: 'ShopZone Lanka', registrationNumber: 'PV 123456' },
    });
  });

  test('the public pending page is informational and makes no API calls', async ({
    page,
  }) => {
    const { calls } = await mockApi(page, signedOut);
    await page.goto('/merchant/pending');
    await expect(
      page.getByRole('heading', { name: 'Merchant application' }),
    ).toBeVisible();
    await expect(
      page.getByText('Login is disabled until approval'),
    ).toBeVisible();
    expect(calls).toEqual([]);
  });
});

test.describe('merchant login states', () => {
  test('an unverified customer is offered another verification email', async ({
    page,
  }) => {
    await mockApi(page, {
      ...signedOut,
      'POST /auth/login': fail(403, 'EMAIL_VERIFICATION_REQUIRED'),
      'POST /auth/resend-verification': ok({ message: 'sent' }, 202),
    });
    await page.goto('/login');
    await page.locator('#login-email').fill('nimal@example.com');
    await page.locator('#login-password').fill('Str0ngPass!');
    await page.getByRole('button', { name: 'Log in' }).click();
    await expect(page.getByText('Verify your email first')).toBeVisible();
    await page
      .getByRole('button', { name: 'Send another verification email' })
      .click();
    await expect(
      page.getByText('If that address needs verification'),
    ).toBeVisible();
  });

  test('an inactive merchant is told approval is required', async ({
    page,
  }) => {
    await mockApi(page, {
      ...signedOut,
      'POST /auth/login': fail(403, 'MERCHANT_ACCOUNT_INACTIVE'),
    });
    await page.goto('/login');
    await page.locator('#login-email').fill('owner@shop.lk');
    await page.locator('#login-password').fill('Str0ngPass!');
    await page.getByRole('button', { name: 'Log in' }).click();
    await expect(page.getByText('Merchant account not active')).toBeVisible();
  });

  test('the email verification success banner appears after redirect', async ({
    page,
  }) => {
    await mockApi(page, signedOut);
    await page.goto('/login?verified=1');
    await expect(page.getByText('Your email has been verified')).toBeVisible();
  });
});

test.describe('admin merchants', () => {
  test('lists applications and rejects without a reason after confirmation', async ({
    page,
  }) => {
    const { calls } = await signInAs(page, 'ADMIN', {
      'GET /admin/merchants': ok([merchant]),
      [`POST /admin/merchants/${merchant.id}/reject`]: ok({
        id: merchant.id,
        status: 'REJECTED',
        identitySync: 'APPLIED',
        alreadyDone: false,
      }),
    });
    await page.goto('/admin/merchants');
    await expect(
      page.getByRole('button', { name: 'ShopZone Lanka' }).first(),
    ).toBeVisible();
    await page.getByRole('button', { name: 'Reject' }).click();
    await expect(
      page.getByRole('heading', { name: 'Reject application?' }),
    ).toBeVisible();
    const reason = page.getByLabel('Reason (optional)');
    await expect(reason).toHaveAttribute('maxlength', '1000');
    await page.getByRole('button', { name: 'Reject' }).last().click();
    await expect(page.getByText('Merchant rejected.')).toBeVisible();
    const call = calls.find(
      (item) => item.key === 'POST /admin/merchants/' + merchant.id + '/reject',
    );
    expect(call?.body).toEqual({});
  });

  test('approval shows a pending identity sync warning', async ({ page }) => {
    await signInAs(page, 'ADMIN', {
      'GET /admin/merchants': ok([merchant]),
      [`POST /admin/merchants/${merchant.id}/approve`]: ok({
        id: merchant.id,
        status: 'APPROVED',
        identitySync: 'PENDING',
        alreadyDone: false,
      }),
    });
    await page.goto('/admin/merchants');
    await page.getByRole('button', { name: 'Approve' }).click();
    await page.getByRole('button', { name: 'Approve' }).last().click();
    await expect(
      page.getByText('Account session invalidation is pending'),
    ).toBeVisible();
  });
});
