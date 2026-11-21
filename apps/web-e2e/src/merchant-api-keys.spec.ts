import { expect, test } from '@playwright/test';

import { fail, ok } from './support/mock-api';
import { signInAs } from './support/sign-in';

test.use({ colorScheme: 'light' });

const merchant = {
  id: '3f1c7a52-0000-4000-8000-0000000000a1',
  ownerUserId: '3f1c7a52-0000-4000-8000-0000000000b1',
  businessName: 'ShopZone Lanka',
  registrationNumber: 'PV 123456',
  contactEmail: 'support@shopzone.lk',
  contactPhone: '+94112223344',
  pickupAddress: '88 Galle Road, Colombo 03',
  pickupZoneId: '7a1e0000-0000-4000-8000-000000000001',
  status: 'APPROVED',
};

const createdKey = {
  id: 'key-1',
  name: 'Production checkout',
  keyPrefix: 'sdk_live_a1b2c3d4',
  status: 'ACTIVE' as const,
  createdAt: '2026-11-04T08:00:00Z',
  lastUsedAt: null,
};

test.describe('merchant API keys', () => {
  test('creates a key, shows the secret once and revokes it', async ({
    page,
  }) => {
    let revoked = false;
    await signInAs(page, 'MERCHANT', {
      'GET /merchants/me': ok(merchant),
      'GET /merchants/me/keys': () =>
        ok([{ ...createdKey, status: revoked ? 'REVOKED' : 'ACTIVE' }]),
      'POST /merchants/me/keys': ok(
        {
          id: 'key-1',
          name: 'Production checkout',
          keyPrefix: createdKey.keyPrefix,
          apiKey:
            'sdk_live_a1b2c3d4e5f60708_0123456789abcdef0123456789abcdef0123456789abcdef',
        },
        201,
      ),
      'DELETE /merchants/me/keys/key-1': () => {
        revoked = true;
        return { status: 204 };
      },
    });
    await page.goto('/api-keys');

    await expect(
      page.getByRole('heading', { level: 1, name: 'API keys', exact: true }),
    ).toBeVisible();
    await page.getByRole('button', { name: 'Create key' }).click();
    await page.getByLabel('Key name').fill('Production checkout');
    await page.getByRole('button', { name: 'Create key' }).last().click();

    await expect(page.getByText('Key created — copy it now')).toBeVisible();
    await expect(
      page.getByText(
        'sdk_live_a1b2c3d4e5f60708_0123456789abcdef0123456789abcdef0123456789abcdef',
      ),
    ).toBeVisible();
    await expect(
      page.getByText('This is the only time the full secret is shown.'),
    ).toBeVisible();

    await page.getByRole('button', { name: 'I have saved this key' }).click();
    await expect(
      page.getByText(
        'sdk_live_a1b2c3d4e5f60708_0123456789abcdef0123456789abcdef0123456789abcdef',
      ),
    ).toHaveCount(0);
    await expect(page.getByText('sdk_live_a1b2c3d4••••••••')).toBeVisible();

    await page
      .getByRole('button', { name: 'Revoke Production checkout' })
      .click();
    await page.getByRole('button', { name: 'Revoke key' }).click();
    await expect(page.getByText('API key revoked.')).toBeVisible();
    await expect(page.getByText('REVOKED', { exact: true })).toBeVisible();
  });

  test('a sixth active key shows the conflict without exposing another secret', async ({
    page,
  }) => {
    await signInAs(page, 'MERCHANT', {
      'GET /merchants/me': ok(merchant),
      'GET /merchants/me/keys': ok([]),
      'POST /merchants/me/keys': fail(409, 'KEY_LIMIT_REACHED'),
    });
    await page.goto('/api-keys');

    await page.getByRole('button', { name: 'Create key' }).click();
    await page.getByLabel('Key name').fill('Sixth key');
    await page.getByRole('button', { name: 'Create key' }).last().click();

    await expect(page.getByText('Could not create key')).toBeVisible();
    await expect(
      page.getByText('This action conflicts with an existing record.').first(),
    ).toBeVisible();
    await expect(page.getByText(/sdk_live_/)).toHaveCount(0);
  });

  test('an unapproved merchant cannot create a key', async ({ page }) => {
    const pending = { ...merchant, status: 'PENDING' };
    const { calls } = await signInAs(page, 'MERCHANT', {
      'GET /merchants/me': ok(pending),
      'GET /merchants/me/keys': fail(403, 'MERCHANT_NOT_APPROVED'),
    });
    await page.goto('/api-keys');

    await expect(
      page.getByText('Merchant account pending approval'),
    ).toBeVisible();
    await expect(
      page.getByRole('button', { name: 'Create key' }),
    ).toBeDisabled();
    expect(calls.some((call) => call.key === 'POST /merchants/me/keys')).toBe(
      false,
    );
  });
});
