import { expect, test, type APIRequestContext } from '@playwright/test';

const mailpit = process.env['MAILPIT_URL'] || 'http://localhost:8025';
const baseURL = process.env['BASE_URL'] || 'http://localhost:8080';

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
      const messages = (
        (await response.json()) as {
          messages?: Array<{
            ID?: string;
            Subject?: string;
            To?: Array<{ Address?: string }>;
          }>;
        }
      ).messages;
      const match = messages?.find(
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

function tokenFromEmail(payload: unknown, path: string): string {
  const text = JSON.stringify(payload);
  const match = text.match(
    new RegExp(`${path.replaceAll('/', '\\/')}\\?token=([A-Za-z0-9_-]+)`),
  );
  if (!match) throw new Error(`token was not found in the ${path} email`);
  return match[1];
}

test.describe('real-stack password recovery', () => {
  test('a customer resets the password from a Mailpit link and the old password stops working', async ({
    page,
    request,
  }) => {
    test.setTimeout(120_000);
    const email = uniqueEmail('reset-journey');
    const oldPassword = 'Str0ngPass!';
    const newPassword = 'N3wStr0ngPass!';

    const registered = await request.post('/api/v1/auth/register', {
      data: {
        fullName: 'Reset Journey',
        email,
        phone: '+94771234567',
        password: oldPassword,
      },
    });
    expect(registered.status(), await registered.text()).toBe(201);

    const verifyEmail = await waitForEmail(
      request,
      email,
      'Verify your SwiftDrop account',
    );
    const verifyToken = tokenFromEmail(
      await verifyEmail.json(),
      '/verify-email',
    );
    const verified = await request.post('/api/v1/auth/verify-email', {
      data: { token: verifyToken },
    });
    expect(verified.ok(), await verified.text()).toBeTruthy();

    await page.goto('/forgot-password');
    await page.getByLabel('Email').fill(email);
    await page.getByRole('button', { name: /^Send reset link$/ }).click();

    const resetEmail = await waitForEmail(
      request,
      email,
      'Reset your SwiftDrop password',
    );
    const resetToken = tokenFromEmail(
      await resetEmail.json(),
      '/reset-password',
    );

    await page.goto(`/reset-password?token=${resetToken}`);
    await page.getByLabel('New password', { exact: true }).fill(newPassword);
    await page
      .getByLabel('Confirm new password', { exact: true })
      .fill(newPassword);
    await page.getByRole('button', { name: 'Save new password' }).click();
    await expect(page.getByText('Password updated')).toBeVisible({
      timeout: 20_000,
    });

    const oldLogin = await request.post('/api/v1/auth/login', {
      headers: { 'X-Token-Mode': 'body' },
      data: { email, password: oldPassword },
    });
    expect(oldLogin.status()).toBe(401);
    const newLogin = await request.post('/api/v1/auth/login', {
      headers: { 'X-Token-Mode': 'body' },
      data: { email, password: newPassword },
    });
    expect(newLogin.ok()).toBeTruthy();

    const reused = await request.post('/api/v1/auth/reset-password', {
      data: { token: resetToken, newPassword: 'An0therStr0ngPass!' },
    });
    expect(reused.status()).toBe(400);

    // A bogus token gets the same public error, never a hint about the account.
    const invalid = await fetch(`${baseURL}/api/v1/auth/reset-password`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        token: 'not-a-real-reset-token',
        newPassword: 'An0therStr0ngPass!',
      }),
    });
    expect(invalid.status).toBe(400);
  });
});
