import { execFileSync } from 'node:child_process';

import {
  expect,
  test,
  type APIRequestContext,
  type Page,
} from '@playwright/test';

const mailpit = process.env['MAILPIT_URL'] || 'http://localhost:8025';
const adminEmail =
  process.env['REAL_STACK_ADMIN_EMAIL'] || 'seed.admin@example.com';
const adminPassword = process.env['REAL_STACK_ADMIN_PASSWORD'] || 'Str0ngPass!';
const composeProject = process.env['COMPOSE_PROJECT_NAME'] || 'swiftdrop';

function uniqueEmail(prefix: string) {
  return `${prefix}.${Date.now()}.${Math.floor(Math.random() * 10000)}@example.test`;
}

async function waitForEmail(
  request: APIRequestContext,
  to: string,
  subjectPart: string,
  timeoutMs = 30_000,
) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const response = await request.get(`${mailpit}/api/v1/messages?limit=100`);
    if (response.ok()) {
      const body = (await response.json()) as {
        messages?: Array<{
          ID?: string;
          Subject?: string;
          To?: Array<{ Address?: string }>;
        }>;
      };
      const match = body.messages?.find(
        (message) =>
          message.Subject?.includes(subjectPart) &&
          message.To?.some((recipient) => recipient.Address === to),
      );
      if (match?.ID) {
        const detail = await request.get(
          `${mailpit}/api/v1/message/${match.ID}`,
        );
        if (detail.ok()) return detail;
      }
    }
    await new Promise((resolve) => setTimeout(resolve, 500));
  }
  throw new Error(`timed out waiting for ${subjectPart} to ${to}`);
}

function tokenFromEmail(payload: unknown): string {
  const text = JSON.stringify(payload);
  const match = text.match(/\/verify-email\?token=([A-Za-z0-9_-]+)/);
  if (!match) throw new Error('verification link was not found in the email');
  return match[1];
}

async function signIn(page: Page, email: string, password: string) {
  await page.goto('/login');
  await page.locator('#login-email').fill(email);
  await page.locator('#login-password').fill(password);
  await page.getByRole('button', { name: 'Log in' }).click();
}

type SessionTokens = { accessToken: string; refreshToken: string };

async function apiLogin(
  request: APIRequestContext,
  email: string,
  password: string,
): Promise<SessionTokens> {
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
      data?: SessionTokens;
    };
    if (!body.data?.accessToken || !body.data?.refreshToken) {
      throw new Error('login did not return tokens');
    }
    return body.data;
  }
  throw new Error(`login for ${email} stayed rate limited`);
}

let cachedAdminAccess: string | null = null;

async function adminToken(
  _page: Page,
  request: APIRequestContext,
): Promise<string> {
  if (cachedAdminAccess) return cachedAdminAccess;
  const tokens = await apiLogin(request, adminEmail, adminPassword);
  cachedAdminAccess = tokens.accessToken;
  return tokens.accessToken;
}

async function waitForHealthy(service: string, timeoutMs = 60_000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    try {
      const status = execFileSync(
        'docker',
        [
          'inspect',
          '--format',
          '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}',
          `swiftdrop-${service}-1`,
        ],
        { encoding: 'utf8' },
      ).trim();
      if (status === 'healthy' || status === 'running') return;
    } catch {
      // The container may still be starting.
    }
    await new Promise((resolve) => setTimeout(resolve, 1000));
  }
  throw new Error(`timed out waiting for ${service} to become healthy`);
}

async function submitWithRateLimitRetry(
  page: Page,
  buttonName: string,
  successText: string,
) {
  const button = page.getByRole('button', { name: buttonName });
  const success = page.getByText(successText);
  for (let attempt = 0; attempt < 4; attempt += 1) {
    await button.click();
    try {
      await success.waitFor({ timeout: 8_000 });
      return;
    } catch {
      const error = page.getByText(/could not|already|is invalid|is required/i);
      if (
        await error
          .first()
          .isVisible()
          .catch(() => false)
      ) {
        throw new Error(`form failed: ${await error.first().innerText()}`);
      }
      await expect(button).toBeEnabled({ timeout: 75_000 });
    }
  }
  throw new Error(`${buttonName} stayed rate limited`);
}

async function registerCustomer(page: Page, email: string) {
  await page.goto('/register/customer');
  await page.getByLabel('Full name').fill('Real Stack Customer');
  await page.getByLabel('Email').fill(email);
  await page.getByLabel('Phone').fill('+94771234567');
  await page.locator('#register-password').fill('Str0ngPass!');
  await page.getByLabel('Confirm password').fill('Str0ngPass!');
  await submitWithRateLimitRetry(page, 'Create account', 'Check your email');
}

async function registerMerchant(
  page: Page,
  email: string,
  businessName: string,
  br: string,
) {
  await page.goto('/register/merchant');
  await page.getByLabel('Full name').fill('Real Stack Merchant');
  await page.locator('#merchant-email').fill(email);
  await page.getByLabel('Phone number').fill('+94771234567');
  await page.locator('#merchant-password').fill('Str0ngPass!');
  await page.getByLabel('Confirm password').fill('Str0ngPass!');
  await page.getByLabel('Business name').fill(businessName);
  await page.getByLabel('Business registration number').fill(br);
  await page.getByLabel('Business address').fill('88 Galle Road, Colombo 03');
  await page
    .getByLabel('Default pickup address')
    .fill('88 Galle Road, Colombo 03');
  await page.getByLabel('Default pickup zone').selectOption({ index: 1 });
  await submitWithRateLimitRetry(
    page,
    'Submit application',
    'Application submitted',
  );
}

async function findMerchantID(
  request: APIRequestContext,
  token: string,
  businessName: string,
) {
  const response = await request.get(
    `/api/v1/admin/merchants?q=${encodeURIComponent(businessName)}`,
    { headers: { Authorization: `Bearer ${token}` } },
  );
  expect(response.ok()).toBeTruthy();
  const body = (await response.json()) as { data?: Array<{ id: string }> };
  const id = body.data?.[0]?.id;
  if (!id) throw new Error('merchant was not found in the admin list');
  return id;
}

test.describe('real stack onboarding', () => {
  test('customer registers, verifies email, and signs in', async ({
    page,
    request,
  }) => {
    const email = uniqueEmail('customer');
    await registerCustomer(page, email);
    const message = await waitForEmail(
      request,
      email,
      'Verify your SwiftDrop account',
    );
    const token = tokenFromEmail(await message.json());
    await page.goto(`/verify-email?token=${token}`);
    await expect(page.getByText('Your email has been verified')).toBeVisible();
    await page.getByRole('link', { name: 'Go to log in' }).click();
    await expect(page.getByText('Your email has been verified')).toBeVisible();
    await page.locator('#login-email').fill(email);
    await page.locator('#login-password').fill('Str0ngPass!');
    await page.getByRole('button', { name: 'Log in' }).click();
    await expect(page.getByText('Signed in as')).toBeVisible();
  });

  test('merchant stays inactive until admin approval', async ({
    page,
    request,
  }) => {
    const email = uniqueEmail('merchant');
    const business = `Real Shop ${Date.now()}`;
    await registerMerchant(page, email, business, `PV ${Date.now()}`);
    await waitForEmail(request, email, 'SwiftDrop merchant application');

    await signIn(page, email, 'Str0ngPass!');
    await expect(page.getByText('Merchant account not active')).toBeVisible();

    const admin = await adminToken(page, request);
    const merchantID = await findMerchantID(request, admin, business);
    const approved = await request.post(
      `/api/v1/admin/merchants/${merchantID}/approve`,
      { headers: { Authorization: `Bearer ${admin}` } },
    );
    expect(approved.ok()).toBeTruthy();
    await waitForEmail(
      request,
      email,
      'Your SwiftDrop merchant account is approved',
    );

    await page.context().clearCookies();
    await signIn(page, email, 'Str0ngPass!');
    await expect(page.getByText('Signed in as')).toBeVisible({
      timeout: 20_000,
    });
    await page.goto('/api-keys');
    await expect(
      page.getByRole('heading', { name: 'API keys', exact: true }),
    ).toBeVisible();
  });

  test('rejection with and without a reason sends the right email', async ({
    page,
    request,
  }) => {
    const admin = await adminToken(page, request);
    for (const withReason of [false, true]) {
      const email = uniqueEmail(
        withReason ? 'reject-reason' : 'reject-neutral',
      );
      const business = `Reject Shop ${Date.now()} ${withReason}`;
      await page.context().clearCookies();
      await registerMerchant(
        page,
        email,
        business,
        `GV ${Date.now()}${withReason ? 1 : 2}`,
      );
      const merchantID = await findMerchantID(request, admin, business);
      const reason = withReason
        ? 'The registration number could not be verified.'
        : undefined;
      const rejected = await request.post(
        `/api/v1/admin/merchants/${merchantID}/reject`,
        {
          headers: { Authorization: `Bearer ${admin}` },
          data: reason ? { reason } : {},
        },
      );
      expect(rejected.ok()).toBeTruthy();
      const emailMessage = await waitForEmail(
        request,
        email,
        'Update on your SwiftDrop merchant application',
      );
      const text = JSON.stringify(await emailMessage.json());
      if (withReason)
        expect(text).toContain('registration number could not be verified');
      else
        expect(text).not.toContain('registration number could not be verified');
      const login = await request.post('/api/v1/auth/login', {
        data: { email, password: 'Str0ngPass!' },
      });
      expect(login.status()).toBe(403);
    }
  });

  test('suspension survives failed Identity deactivation and rejects old tokens after reapproval', async ({
    page,
    request,
  }) => {
    const email = uniqueEmail('suspend');
    const business = `Suspend Shop ${Date.now()}`;
    await registerMerchant(page, email, business, `PB ${Date.now()}`);
    const admin = await adminToken(page, request);
    const merchantID = await findMerchantID(request, admin, business);
    const approved = await request.post(
      `/api/v1/admin/merchants/${merchantID}/approve`,
      { headers: { Authorization: `Bearer ${admin}` } },
    );
    expect(approved.ok()).toBeTruthy();
    const old = await apiLogin(request, email, 'Str0ngPass!');

    execFileSync(
      'docker',
      ['compose', '-p', composeProject, 'stop', 'identity'],
      {
        stdio: 'ignore',
      },
    );
    try {
      const suspended = await request.post(
        `/api/v1/admin/merchants/${merchantID}/suspend`,
        {
          headers: { Authorization: `Bearer ${admin}` },
          data: { reason: 'Real-stack suspension check' },
        },
      );
      expect(suspended.ok()).toBeTruthy();
      const body = (await suspended.json()) as {
        data?: { identitySync?: string };
      };
      expect(body.data?.identitySync).toBe('PENDING');
      const blocked = await request.get('/api/v1/merchants/me/keys', {
        headers: { Authorization: `Bearer ${old.accessToken}` },
      });
      expect([401, 403, 503]).toContain(blocked.status());
    } finally {
      execFileSync(
        'docker',
        ['compose', '-p', composeProject, 'start', 'identity'],
        {
          stdio: 'ignore',
        },
      );
    }

    await waitForHealthy('identity');

    const reapproved = await request.post(
      `/api/v1/admin/merchants/${merchantID}/approve`,
      { headers: { Authorization: `Bearer ${admin}` } },
    );
    expect(reapproved.ok()).toBeTruthy();

    const stale = await request.get('/api/v1/merchants/me/keys', {
      headers: { Authorization: `Bearer ${old.accessToken}` },
    });
    expect(stale.status()).toBe(401);

    const fresh = await apiLogin(request, email, 'Str0ngPass!');
    const allowed = await request.get('/api/v1/merchants/me/keys', {
      headers: { Authorization: `Bearer ${fresh.accessToken}` },
    });
    expect(allowed.status()).toBe(200);
  });
});
