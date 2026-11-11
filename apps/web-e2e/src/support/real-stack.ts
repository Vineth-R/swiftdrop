import type { APIRequestContext } from '@playwright/test';

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
 * A test that needs an account without its variables is skipped, not failed.
 */
export type SeededAccount =
  'ADMIN' | 'DISPATCHER' | 'COURIER_1' | 'COURIER_2' | 'CUSTOMER';

export type Credentials = { email: string; password: string };

export function credentialsFor(account: SeededAccount): Credentials | null {
  const email = process.env[`REAL_STACK_${account}_EMAIL`];
  const password = process.env[`REAL_STACK_${account}_PASSWORD`];
  return email && password ? { email, password } : null;
}

/** Logs in through the API and returns the access token. Waits out the shared login rate limit (429). */
export async function apiLogin(
  request: APIRequestContext,
  { email, password }: Credentials,
): Promise<string> {
  for (let attempt = 0; attempt < 6; attempt += 1) {
    const response = await request.post('/api/v1/auth/login', {
      headers: { 'X-Token-Mode': 'body' },
      data: { email, password },
    });
    if (response.status() === 429) {
      const retryAfter = Number(response.headers()['retry-after']);
      const waitMs =
        Number.isFinite(retryAfter) && retryAfter > 0
          ? retryAfter * 1000
          : 5000;
      await new Promise((resolve) =>
        setTimeout(resolve, Math.min(waitMs, 30_000)),
      );
      continue;
    }
    if (!response.ok()) {
      throw new Error(`login for ${email} returned ${response.status()}`);
    }
    const body = (await response.json()) as {
      data?: { accessToken?: string };
    };
    if (!body.data?.accessToken) throw new Error('login returned no token');
    return body.data.accessToken;
  }
  throw new Error(`login for ${email} stayed rate limited`);
}
