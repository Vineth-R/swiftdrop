import { execFileSync } from 'node:child_process';

import {
  expect,
  test,
  type APIRequestContext,
  type APIResponse,
  type Page,
} from '@playwright/test';

import {
  apiLogin,
  loginRequest,
  signInThroughForm,
} from './support/real-stack';

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

/** How many messages Mailpit holds for `to` whose subject contains `subjectPart`. */
async function countEmails(
  request: APIRequestContext,
  to: string,
  subjectPart: string,
) {
  const response = await request.get(`${mailpit}/api/v1/messages?limit=100`);
  if (!response.ok()) return 0;
  const body = (await response.json()) as {
    messages?: Array<{
      Subject?: string;
      To?: Array<{ Address?: string }>;
    }>;
  };
  return (body.messages ?? []).filter(
    (message) =>
      message.Subject?.includes(subjectPart) &&
      message.To?.some((recipient) => recipient.Address === to),
  ).length;
}

function tokenFromEmail(payload: unknown): string {
  const text = JSON.stringify(payload);
  const match = text.match(/\/verify-email\?token=([A-Za-z0-9_-]+)/);
  if (!match) throw new Error('verification link was not found in the email');
  return match[1];
}

async function signIn(page: Page, email: string, password: string) {
  await signInThroughForm(page, email, password);
}

/** One login, answered as it came, for the refusals a test reads. */
function loginResponse(
  request: APIRequestContext,
  email: string,
  password: string,
): Promise<APIResponse> {
  return loginRequest(request, email, password, { tokenMode: false });
}

let cachedAdminAccess: string | null = null;

async function adminToken(
  _page: Page,
  request: APIRequestContext,
): Promise<string> {
  if (cachedAdminAccess) return cachedAdminAccess;
  cachedAdminAccess = await apiLogin(request, adminEmail, adminPassword);
  return cachedAdminAccess;
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
          `${composeProject}-${service}-1`,
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
  const rateLimited = page.getByText('Too many attempts from this device');
  for (let attempt = 0; attempt < 4; attempt += 1) {
    await button.click();
    try {
      await success.waitFor({ timeout: 8_000 });
      return;
    } catch {
      // The public register page's footer literally says "Already have one?
      // Log in", so arbitrary page text must never be read as a submit error.
      // A real failure is a danger alert (role="alert"); rate limiting is the
      // one alert that means "wait and try again" rather than "stop".
      if (await rateLimited.isVisible().catch(() => false)) {
        await expect(button).toBeEnabled({ timeout: 75_000 });
        if (await success.isVisible().catch(() => false)) return;
        continue;
      }
      const alert = page.getByRole('alert');
      if (
        await alert
          .first()
          .isVisible()
          .catch(() => false)
      ) {
        throw new Error(`form failed: ${await alert.first().innerText()}`);
      }
      await expect(button).toBeEnabled({ timeout: 75_000 });
      if (await success.isVisible().catch(() => false)) return;
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

/** The `field` and `issue` pairs of a refused login (API 2.2), by field. */
async function refusalDetails(response: APIResponse) {
  const body = (await response.json()) as {
    error?: {
      code?: string;
      details?: Array<{ field: string; issue: string }>;
    };
  };
  expect(body.error?.code).toBe('MERCHANT_ACCOUNT_INACTIVE');
  const details: Record<string, string> = {};
  for (const item of body.error?.details ?? [])
    details[item.field] = item.issue;
  return details;
}

test.describe('real stack onboarding', () => {
  // Registration email waits and login rate-limit backoff share this budget.
  test.describe.configure({ timeout: 180_000 });

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
    await expect(page.getByText('Application under review')).toBeVisible();

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
      // Logins share one rate limit with registration (10 a minute), and this
      // suite is close to it, so this stays the one login of this step.
      const login = await loginResponse(request, email, 'Str0ngPass!');
      expect(login.status()).toBe(403);
      // After the correct password the owner is told where the application
      // stands, with the administrator's reason when there is one.
      const details = await refusalDetails(login);
      expect(details['status']).toBe('REJECTED');
      expect(details['reason']).toBe(reason);
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
        headers: { Authorization: `Bearer ${old}` },
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

    // Identity never got the suspension, and the administrator can see that.
    const adminHeaders = { Authorization: `Bearer ${admin}` };
    const detail = async () =>
      (await (
        await request.get(`/api/v1/admin/merchants/${merchantID}`, {
          headers: adminHeaders,
        })
      ).json()) as { data?: { identitySync?: string } };
    expect((await detail()).data?.identitySync).toBe('PENDING');
    await expect
      .poll(() => countEmails(request, email, 'merchant account is suspended'))
      .toBe(1);

    // Repeating the suspension replays the same transition now Identity is back.
    const retried = await request.post(
      `/api/v1/admin/merchants/${merchantID}/suspend`,
      {
        headers: adminHeaders,
        data: { reason: 'Real-stack suspension check' },
      },
    );
    expect(retried.ok()).toBeTruthy();
    const retriedBody = (await retried.json()) as {
      data?: { identitySync?: string };
    };
    expect(retriedBody.data?.identitySync).toBe('APPLIED');
    expect((await detail()).data?.identitySync).toBeUndefined();
    // The merchant was told once, with the decision, and not again by the retry.
    await new Promise((resolve) => setTimeout(resolve, 2_000));
    expect(
      await countEmails(request, email, 'merchant account is suspended'),
    ).toBe(1);

    const reapproved = await request.post(
      `/api/v1/admin/merchants/${merchantID}/approve`,
      { headers: { Authorization: `Bearer ${admin}` } },
    );
    expect(reapproved.ok()).toBeTruthy();

    const stale = await request.get('/api/v1/merchants/me/keys', {
      headers: { Authorization: `Bearer ${old}` },
    });
    expect(stale.status()).toBe(401);

    const fresh = await apiLogin(request, email, 'Str0ngPass!');
    const allowed = await request.get('/api/v1/merchants/me/keys', {
      headers: { Authorization: `Bearer ${fresh}` },
    });
    expect(allowed.status()).toBe(200);
  });
});
