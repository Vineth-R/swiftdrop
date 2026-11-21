import type { APIRequestContext, APIResponse } from '@playwright/test';

/**
 * Logins for the seeded demo users on the real Compose stack.
 *
 * Nothing secret lives here. Each account comes from environment variables, so
 * a real password never reaches the repository:
 *
 *   REAL_STACK_ADMIN_EMAIL / REAL_STACK_ADMIN_PASSWORD
 *   REAL_STACK_DISPATCHER_EMAIL / REAL_STACK_DISPATCHER_PASSWORD
 *   REAL_STACK_COURIER_1_EMAIL / REAL_STACK_COURIER_1_PASSWORD
 *   REAL_STACK_COURIER_2_EMAIL / REAL_STACK_COURIER_2_PASSWORD
 *   REAL_STACK_CUSTOMER_EMAIL / REAL_STACK_CUSTOMER_PASSWORD
 *
 * Use the same values you gave `make seed` (SEED_*_EMAIL, SEED_*_PASSWORD).
 * Locally a test that needs an account without its variables is skipped. In CI
 * every account is required, so a missing one fails the test instead of hiding
 * a scenario that never ran.
 */
export type SeededAccount =
  'ADMIN' | 'DISPATCHER' | 'COURIER_1' | 'COURIER_2' | 'CUSTOMER';

export type Credentials = { email: string; password: string };

/** True in CI, where the provisioning step must supply every demo account. */
export const isCI = process.env['CI'] === 'true' || process.env['CI'] === '1';

export function credentialsFor(account: SeededAccount): Credentials | null {
  const email = process.env[`REAL_STACK_${account}_EMAIL`];
  const password = process.env[`REAL_STACK_${account}_PASSWORD`];
  return email && password ? { email, password } : null;
}

/**
 * Credentials for one test run. In CI a missing account throws, so the job
 * fails loudly rather than reporting a green run where the test skipped.
 */
export function credentialsForRun(account: SeededAccount): Credentials | null {
  const credentials = credentialsFor(account);
  if (credentials) return credentials;
  if (isCI) {
    throw new Error(
      `REAL_STACK_${account}_EMAIL and REAL_STACK_${account}_PASSWORD are required in CI. Seed the demo accounts before running the real-stack suite.`,
    );
  }
  return null;
}

/** Logs in through the API once, waiting out the shared login rate limit (429). */
export async function loginRequest(
  request: APIRequestContext,
  email: string,
  password: string,
  { tokenMode = true }: { tokenMode?: boolean } = {},
): Promise<APIResponse> {
  for (let attempt = 0; attempt < 6; attempt += 1) {
    const response = await request.post('/api/v1/auth/login', {
      headers: tokenMode ? { 'X-Token-Mode': 'body' } : undefined,
      data: { email, password },
    });
    if (response.status() !== 429) return response;
    const retryAfter = Number(response.headers()['retry-after']);
    const waitMs =
      Number.isFinite(retryAfter) && retryAfter > 0 ? retryAfter * 1000 : 5000;
    await new Promise((resolve) =>
      setTimeout(resolve, Math.min(waitMs, 30_000)),
    );
  }
  throw new Error(`login for ${email} stayed rate limited`);
}

/** Logs in through the API and returns the access token. */
export async function apiLogin(
  request: APIRequestContext,
  email: string,
  password: string,
): Promise<string> {
  const response = await loginRequest(request, email, password);
  if (!response.ok()) {
    throw new Error(`login for ${email} returned ${response.status()}`);
  }
  const body = (await response.json()) as {
    data?: { accessToken?: string };
  };
  if (!body.data?.accessToken) throw new Error('login returned no token');
  return body.data.accessToken;
}

/**
 * Signs in through the login form, waiting out the shared login rate limit.
 * Callers assert the page they expect after the helper returns. It does not
 * assume a successful session: a pending merchant correctly stays on a notice.
 */
export async function signInThroughForm(
  page: import('@playwright/test').Page,
  email: string,
  password: string,
): Promise<void> {
  for (let attempt = 0; attempt < 3; attempt += 1) {
    await page.goto('/login');
    await page.locator('#login-email').fill(email);
    await page.locator('#login-password').fill(password);
    await page.getByRole('button', { name: 'Log in' }).click();
    await page.waitForTimeout(1000);
    const limited = await page
      .getByText('Too many attempts from this device')
      .isVisible()
      .catch(() => false);
    if (!limited) return;
    await page.waitForTimeout(65_000);
  }
  throw new Error('login stayed rate limited');
}

type Zone = { id: string; name: string };

const contact = (name: string, zone: Zone) => ({
  name,
  phone: '+94771234567',
  address: '12 Test Lane',
  city: zone.name,
  zoneId: zone.id,
});

export type CreatedParcel = { id: string; trackingNumber: string };

/**
 * Creates a parcel the way the booking form does (zones, fee estimate, then
 * POST /parcels), for the customer who owns `token`. It looks for a pair of
 * zones that has a price, so it does not depend on the seed's zone names.
 */
export async function createParcel(
  request: APIRequestContext,
  token: string,
  idempotencyKey: string,
): Promise<CreatedParcel> {
  const auth = { Authorization: `Bearer ${token}` };

  const zonesResponse = await request.get('/api/v1/zones');
  if (!zonesResponse.ok()) throw new Error('zones could not be loaded');
  const zones = ((await zonesResponse.json()) as { data: Zone[] }).data;

  for (const from of zones) {
    for (const to of zones) {
      if (from.id === to.id) continue;
      const estimate = await request.post('/api/v1/pricing/estimate', {
        headers: auth,
        data: {
          originZoneId: from.id,
          destinationZoneId: to.id,
          weightKg: 1,
          sizeCategory: 'SMALL',
        },
      });
      if (!estimate.ok()) continue;
      const fee = ((await estimate.json()) as { data: { total: number } }).data
        .total;

      const created = await request.post('/api/v1/parcels', {
        headers: { ...auth, 'Idempotency-Key': idempotencyKey },
        data: {
          sender: contact('Test Sender', from),
          recipient: contact('Test Recipient', to),
          weightKg: 1,
          sizeCategory: 'SMALL',
          expectedFee: fee,
        },
      });
      if (created.status() !== 201) {
        throw new Error(
          `creating a parcel returned ${created.status()}: ${await created.text()}`,
        );
      }
      const data = ((await created.json()) as { data: CreatedParcel }).data;
      return { id: data.id, trackingNumber: data.trackingNumber };
    }
  }
  throw new Error('no priced pair of zones in the seed data');
}

/** The signed-in user's id (GET /users/me). */
export async function apiUserId(
  request: APIRequestContext,
  token: string,
): Promise<string> {
  const response = await request.get('/api/v1/users/me', {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (!response.ok())
    throw new Error(`/users/me returned ${response.status()}`);
  return ((await response.json()) as { data: { id: string } }).data.id;
}
