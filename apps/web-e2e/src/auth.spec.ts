import { test, expect, type Page } from '@playwright/test';

import {
  fail,
  mockApi,
  ok,
  REQUEST_ID,
  SERVER_WORDS,
  signedOut,
  user,
  type Reply,
} from './support/mock-api';

/**
 * The alert on the page. Next also keeps a hidden role="alert" route announcer
 * in every page, so a bare getByRole('alert') matches two elements.
 */
const alertBox = (page: Page) =>
  page.locator('[role="alert"]:not(#__next-route-announcer__)');

test.use({ colorScheme: 'light' });

test.describe('session check and splash', () => {
  test('a signed-out visitor to an app page lands on login and is sent back after', async ({
    page,
  }) => {
    await mockApi(page, {
      ...signedOut,
      'GET /users/me': { ...fail(401, 'UNAUTHENTICATED'), delayMs: 700 },
    });

    await page.goto('/dashboard');
    await expect(page.getByText('Checking your session')).toBeVisible();
    await expect(page).toHaveURL(/\/login\?next=%2Fdashboard$/);
  });

  test('a signed-in user sees their own dashboard and can log out', async ({
    page,
  }) => {
    let signedIn = true;
    const { calls } = await mockApi(page, {
      'GET /users/me': () =>
        signedIn ? ok(user) : fail(401, 'UNAUTHENTICATED'),
      'POST /auth/refresh': fail(401, 'REFRESH_TOKEN_INVALID'),
      'POST /auth/logout': () => {
        signedIn = false;
        return { status: 204 };
      },
    });

    await page.goto('/dashboard');
    await expect(page.getByText('Signed in as')).toContainText('Nimal Perera');
    await expect(page.getByText('CUSTOMER')).toBeVisible();

    await page.getByRole('button', { name: 'Account menu' }).click();
    await page.getByRole('menuitem', { name: 'Log out' }).click();
    await expect(page).toHaveURL(/\/signed-out$/);
    await expect(
      page.getByRole('heading', { name: 'You are signed out' }),
    ).toBeVisible();
    expect(calls.map((call) => call.key)).toContain('POST /auth/logout');
  });

  test('an expired access token is renewed quietly and the page loads', async ({
    page,
  }) => {
    let renewed = false;
    const { calls } = await mockApi(page, {
      'GET /users/me': () => (renewed ? ok(user) : fail(401, 'TOKEN_EXPIRED')),
      'POST /auth/refresh': () => {
        renewed = true;
        return ok({ expiresIn: 900 });
      },
    });

    await page.goto('/dashboard');
    await expect(page.getByText('Signed in as')).toContainText('Nimal Perera');
    expect(calls.map((call) => call.key)).toEqual([
      'GET /users/me',
      'POST /auth/refresh',
      'GET /users/me',
    ]);
  });

  test('an unreachable API is not mistaken for being signed out', async ({
    page,
  }) => {
    let online = false;
    await mockApi(page, {
      'GET /users/me': () => (online ? ok(user) : { status: 503 }),
    });

    await page.goto('/dashboard');
    await expect(
      page.getByRole('heading', { name: 'We could not reach SwiftDrop' }),
    ).toBeVisible();
    await expect(page).toHaveURL(/\/dashboard$/);

    online = true;
    await page.getByRole('button', { name: 'Try again' }).click();
    await expect(page.getByText('Signed in as')).toContainText('Nimal Perera');
  });

  test('someone already signed in is taken past the login page', async ({
    page,
  }) => {
    await mockApi(page, { 'GET /users/me': ok(user) });

    await page.goto('/login');
    await expect(page).toHaveURL(/\/dashboard$/);
  });
});

test.describe('login', () => {
  test('logs in, sends exactly the email and password, and lands on the dashboard', async ({
    page,
  }) => {
    let signedIn = false;
    const { calls } = await mockApi(page, {
      'GET /users/me': () =>
        signedIn ? ok(user) : fail(401, 'UNAUTHENTICATED'),
      'POST /auth/refresh': fail(401, 'REFRESH_TOKEN_INVALID'),
      'POST /auth/login': () => {
        signedIn = true;
        return ok({ user, expiresIn: 900 });
      },
    });

    await page.goto('/login');
    await page.getByLabel('Email').fill('  nimal@example.com ');
    await page.getByLabel('Password', { exact: true }).fill('Str0ngPass!');
    await page.getByRole('button', { name: 'Log in' }).click();

    await expect(page).toHaveURL(/\/dashboard$/);
    expect(calls.find((call) => call.key === 'POST /auth/login')?.body).toEqual(
      {
        email: 'nimal@example.com',
        password: 'Str0ngPass!',
      },
    );
  });

  test('returns to the page the visitor came from, and only to a page on this site', async ({
    page,
  }) => {
    for (const [next, landsOn] of [
      ['/dashboard?tab=parcels', /\/dashboard\?tab=parcels$/],
      ['//evil.example', /\/dashboard$/],
      ['https://evil.example/x', /\/dashboard$/],
      ['/\\evil.example', /\/dashboard$/],
      ['/login', /\/dashboard$/],
    ] as const) {
      let signedIn = false;
      await page.unroute('**/api/v1/**');
      await mockApi(page, {
        'GET /users/me': () =>
          signedIn ? ok(user) : fail(401, 'UNAUTHENTICATED'),
        'POST /auth/refresh': fail(401, 'REFRESH_TOKEN_INVALID'),
        'POST /auth/login': () => {
          signedIn = true;
          return ok({ user, expiresIn: 900 });
        },
      });

      await page.goto(`/login?next=${encodeURIComponent(next)}`);
      await page.getByLabel('Email').fill('nimal@example.com');
      await page.getByLabel('Password', { exact: true }).fill('Str0ngPass!');
      await page.getByRole('button', { name: 'Log in' }).click();
      await expect(page).toHaveURL(landsOn);
    }
  });

  test('nothing is checked before a field is left, then errors show without a request', async ({
    page,
  }) => {
    const { calls } = await mockApi(page, signedOut);
    await page.goto('/login');
    await expect(page.getByText('Enter your email address.')).toHaveCount(0);

    await page.getByLabel('Email').fill('nimal@');
    await page.getByLabel('Email').blur();
    await expect(
      page.getByText('Enter a complete email address.'),
    ).toBeVisible();

    await page.getByRole('button', { name: 'Log in' }).click();
    await expect(page.getByText('Enter your password.')).toBeVisible();
    await expect(page.getByLabel('Email')).toHaveAttribute(
      'aria-invalid',
      'true',
    );
    expect(calls.map((call) => call.key)).not.toContain('POST /auth/login');

    await page.getByLabel('Email').fill('nimal@example.com');
    await expect(page.getByText('Enter a complete email address.')).toHaveCount(
      0,
    );
  });

  test('wrong credentials: one message for both fields, the email stays, the password clears', async ({
    page,
  }) => {
    await mockApi(page, {
      ...signedOut,
      'POST /auth/login': fail(401, 'INVALID_CREDENTIALS'),
    });
    await page.goto('/login');
    await page.getByLabel('Email').fill('nimal@example.com');
    await page.getByLabel('Password', { exact: true }).fill('Wr0ngPass!');
    await page.getByRole('button', { name: 'Log in' }).click();

    const alert = alertBox(page);
    await expect(alert).toContainText('Email or password is incorrect');
    await expect(alert).toBeFocused();
    await expect(page.getByLabel('Email')).toHaveValue('nimal@example.com');
    await expect(page.getByLabel('Password', { exact: true })).toHaveValue('');
    await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
    await expect(page.getByText('Reference')).toHaveCount(0);
  });

  test('a locked account says so and offers the reset route', async ({
    page,
  }) => {
    await mockApi(page, {
      ...signedOut,
      'POST /auth/login': fail(423, 'ACCOUNT_LOCKED'),
    });
    await page.goto('/login');
    await page.getByLabel('Email').fill('nimal@example.com');
    await page.getByLabel('Password', { exact: true }).fill('Str0ngPass!');
    await page.getByRole('button', { name: 'Log in' }).click();

    await expect(alertBox(page)).toContainText('Account locked for 15 minutes');
    await expect(
      alertBox(page).getByRole('link', { name: 'reset your password now' }),
    ).toHaveAttribute('href', '/forgot-password');
    await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
  });

  test('a deactivated account loses the form and keeps one way out', async ({
    page,
  }) => {
    await mockApi(page, {
      ...signedOut,
      'POST /auth/login': fail(403, 'ACCOUNT_DISABLED'),
    });
    await page.goto('/login');
    await page.getByLabel('Email').fill('nimal@example.com');
    await page.getByLabel('Password', { exact: true }).fill('Str0ngPass!');
    await page.getByRole('button', { name: 'Log in' }).click();

    await expect(alertBox(page)).toContainText(
      'This account has been deactivated',
    );
    await expect(page.getByLabel('Email')).toHaveCount(0);
    await expect(
      page.getByRole('link', { name: 'Back to home' }),
    ).toBeVisible();
  });

  test('rate limited: the button counts down, then returns, and the typed password is kept', async ({
    page,
  }) => {
    await mockApi(page, {
      ...signedOut,
      'POST /auth/login': fail(429, 'RATE_LIMITED', {
        headers: { 'Retry-After': '3' },
      }),
    });
    await page.goto('/login');
    await page.getByLabel('Email').fill('nimal@example.com');
    await page.getByLabel('Password', { exact: true }).fill('Str0ngPass!');
    await page.getByRole('button', { name: 'Log in' }).click();

    await expect(
      page.getByRole('status').filter({ hasText: 'Too many attempts' }),
    ).toContainText('Too many attempts from this device');
    const button = page.getByRole('button', { name: /Try again in 0:0\d/ });
    await expect(button).toBeDisabled();
    await expect(page.getByLabel('Password', { exact: true })).toHaveValue(
      'Str0ngPass!',
    );

    await expect(page.getByRole('button', { name: 'Log in' })).toBeEnabled({
      timeout: 8000,
    });
  });

  test('a server fault shows a Reference; a validation error does not', async ({
    page,
  }) => {
    let reply: Reply = fail(500, 'INTERNAL_ERROR');
    await mockApi(page, { ...signedOut, 'POST /auth/login': () => reply });
    await page.goto('/login');
    await page.getByLabel('Email').fill('nimal@example.com');
    await page.getByLabel('Password', { exact: true }).fill('Str0ngPass!');
    await page.getByRole('button', { name: 'Log in' }).click();

    await expect(alertBox(page)).toContainText('Something broke on our side');
    await expect(page.getByText(`Reference ${REQUEST_ID}`)).toBeVisible();
    await expect(page.getByLabel('Password', { exact: true })).toHaveValue(
      'Str0ngPass!',
    );
    await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);

    reply = fail(422, 'VALIDATION_ERROR', {
      details: [{ field: 'email', issue: SERVER_WORDS }],
    });
    await page.getByRole('button', { name: 'Log in' }).click();
    await expect(
      page.getByText('Enter a complete email address.'),
    ).toBeVisible();
    await expect(page.getByText('Reference')).toHaveCount(0);
    await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
  });

  test('a failed Google sign-in is explained once, without the cause', async ({
    page,
  }) => {
    await mockApi(page, signedOut);
    await page.goto('/login?error=oauth_failed');

    await expect(alertBox(page)).toContainText(
      'Google sign-in did not complete',
    );
  });

  test('Google sign-in is a link that carries only a safe return path', async ({
    page,
  }) => {
    await mockApi(page, signedOut);
    await page.goto('/login?next=%2Fdashboard%3Ftab%3Dparcels');
    await expect(
      page.getByRole('link', { name: 'Continue with Google' }),
    ).toHaveAttribute(
      'href',
      '/api/v1/auth/google/login?redirectTo=%2Fdashboard%3Ftab%3Dparcels',
    );

    await page.goto('/login?next=https%3A%2F%2Fevil.example');
    await expect(
      page.getByRole('link', { name: 'Continue with Google' }),
    ).toHaveAttribute(
      'href',
      '/api/v1/auth/google/login?redirectTo=%2Fdashboard',
    );
  });

  test('at 375px the page does not scroll sideways and the panel shrinks to a header', async ({
    page,
  }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    await mockApi(page, signedOut);
    await page.goto('/login');

    await expect(page.getByRole('heading', { name: 'Log in' })).toBeVisible();
    await expect(page.getByText('Every parcel, point to point.')).toBeHidden();
    const overflow = await page.evaluate(
      'document.documentElement.scrollWidth - document.documentElement.clientWidth',
    );
    expect(overflow).toBe(0);
    await expect(page.getByLabel('Email')).toHaveCSS('height', '48px');
  });
});

test.describe('register', () => {
  async function fillValid(page: Page) {
    await page.getByLabel('Full name').fill('Nimal Perera');
    await page.getByLabel('Email').fill('nimal@example.com');
    await page.getByLabel('Phone').fill('077 123 4567');
    await page.locator('#register-password').fill('Str0ngPass!');
    await page.getByLabel('Confirm password').fill('Str0ngPass!');
  }

  test('creates the account, signs in, and goes to the dashboard', async ({
    page,
  }) => {
    let signedIn = false;
    const { calls } = await mockApi(page, {
      'GET /users/me': () =>
        signedIn ? ok(user) : fail(401, 'UNAUTHENTICATED'),
      'POST /auth/refresh': fail(401, 'REFRESH_TOKEN_INVALID'),
      'POST /auth/register': ok(user, 201),
      'POST /auth/login': () => {
        signedIn = true;
        return ok({ user, expiresIn: 900 });
      },
    });
    await page.goto('/register');
    await fillValid(page);
    await page.getByRole('button', { name: 'Create account' }).click();

    await expect(page.getByText('Account created')).toBeVisible();
    await expect(page.getByText('nimal@example.com').first()).toBeVisible();
    await expect(page).toHaveURL(/\/dashboard$/, { timeout: 8000 });
    expect(
      calls.find((call) => call.key === 'POST /auth/register')?.body,
    ).toEqual({
      fullName: 'Nimal Perera',
      email: 'nimal@example.com',
      phone: '+94771234567',
      password: 'Str0ngPass!',
    });
  });

  test('a failed submit counts the problems, takes focus and links to each field', async ({
    page,
  }) => {
    const { calls } = await mockApi(page, signedOut);
    await page.goto('/register');
    await page.getByRole('button', { name: 'Create account' }).click();

    const summary = page.getByRole('alert').filter({ hasText: 'attention' });
    await expect(summary).toContainText('4 fields need attention');
    await expect(summary).toBeFocused();
    await expect(
      summary.getByRole('link', { name: 'Full name' }),
    ).toHaveAttribute('href', '#register-fullName');
    expect(calls.map((call) => call.key)).not.toContain('POST /auth/register');

    await page.getByLabel('Full name').fill('Nimal Perera');
    await page.getByLabel('Email').fill('nimal@example.com');
    await page.getByLabel('Phone').fill('0771234567');
    await page.locator('#register-password').fill('Str0ngPass!');
    await page.getByLabel('Confirm password').fill('different');
    await expect(summary).toContainText('One field needs attention');
    await expect(
      page.getByText('The two passwords do not match.').first(),
    ).toBeVisible();
  });

  test('the password rules flip as the person types', async ({ page }) => {
    await mockApi(page, signedOut);
    await page.goto('/register');
    const password = page.locator('#register-password');

    await password.fill('abc');
    await expect(page.getByText('Weak — two more to go')).toBeVisible();
    await password.fill('Colombo26');
    await expect(page.getByText('Meets the rule')).toBeVisible();
  });

  test('an email that is already registered offers log in and reset in the same slot', async ({
    page,
  }) => {
    await mockApi(page, {
      ...signedOut,
      'POST /auth/register': fail(409, 'EMAIL_ALREADY_EXISTS'),
    });
    await page.goto('/register');
    await fillValid(page);
    await page.getByRole('button', { name: 'Create account' }).click();

    const email = page.getByLabel('Email');
    await expect(email).toHaveAttribute('aria-invalid', 'true');
    await expect(page.locator('#register-email-error')).toContainText(
      'This email is already registered.',
    );
    await expect(
      page
        .locator('#register-email-error')
        .getByRole('link', { name: 'Log in instead' }),
    ).toHaveAttribute('href', '/login');
    await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);

    await email.fill('other@example.com');
    await expect(page.locator('#register-email-error')).toHaveCount(0);
  });

  test('server field problems use our wording, not the server text', async ({
    page,
  }) => {
    await mockApi(page, {
      ...signedOut,
      'POST /auth/register': fail(422, 'VALIDATION_ERROR', {
        details: [{ field: 'phone', issue: SERVER_WORDS }],
      }),
    });
    await page.goto('/register');
    await fillValid(page);
    await page.getByRole('button', { name: 'Create account' }).click();

    await expect(page.locator('#register-phone-error')).toContainText(
      'Enter a full number',
    );
    await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
  });
});

test.describe('forgot and reset password', () => {
  test('the reply is the same screen whatever the answer was', async ({
    page,
  }) => {
    const { calls } = await mockApi(page, {
      ...signedOut,
      'POST /auth/forgot-password': ok({ message: SERVER_WORDS }, 202),
    });
    await page.goto('/forgot-password');
    await page.getByLabel('Email').fill('anyone@example.com');
    await page.getByRole('button', { name: 'Send reset link' }).click();

    await expect(
      page.getByRole('heading', { name: 'Check your email' }),
    ).toBeVisible();
    await expect(
      page.getByText('If an account exists for this email'),
    ).toBeVisible();
    await expect(page.getByText('anyone@example.com')).toBeVisible();
    await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
    expect(
      calls.find((call) => call.key === 'POST /auth/forgot-password')?.body,
    ).toEqual({ email: 'anyone@example.com' });

    await page.getByRole('button', { name: 'send it again' }).click();
    await expect(page.getByText('We sent it again.')).toBeVisible();
  });

  test('a rate limited request keeps the form and says to wait', async ({
    page,
  }) => {
    await mockApi(page, {
      ...signedOut,
      'POST /auth/forgot-password': fail(429, 'RATE_LIMITED', {
        headers: { 'Retry-After': '30' },
      }),
    });
    await page.goto('/forgot-password');
    await page.getByLabel('Email').fill('anyone@example.com');
    await page.getByRole('button', { name: 'Send reset link' }).click();

    await expect(
      page.getByText('Too many attempts from this device'),
    ).toBeVisible();
    await expect(page.getByLabel('Email')).toHaveValue('anyone@example.com');
  });

  test('a valid link sets the new password and then goes to login', async ({
    page,
  }) => {
    const { calls } = await mockApi(page, {
      ...signedOut,
      'POST /auth/reset-password': { status: 204 },
    });
    await page.goto('/reset-password?token=rt-abc123');
    await page
      .getByLabel('New password', { exact: true })
      .fill('N3wStrongPass');
    await page.getByLabel('Confirm new password').fill('N3wStrongPass');
    await page.getByRole('button', { name: 'Save new password' }).click();

    await expect(
      page.getByRole('heading', { name: 'Password updated' }),
    ).toBeVisible();
    expect(
      calls.find((call) => call.key === 'POST /auth/reset-password')?.body,
    ).toEqual({ token: 'rt-abc123', newPassword: 'N3wStrongPass' });
    await expect(page).toHaveURL(/\/login$/, { timeout: 9000 });
  });

  test('an expired, used or missing link gets one message and a way to ask again', async ({
    page,
  }) => {
    await mockApi(page, {
      ...signedOut,
      'POST /auth/reset-password': fail(400, 'INVALID_OR_EXPIRED_RESET_TOKEN'),
    });

    await page.goto('/reset-password?token=old');
    await page
      .getByLabel('New password', { exact: true })
      .fill('N3wStrongPass');
    await page.getByLabel('Confirm new password').fill('N3wStrongPass');
    await page.getByRole('button', { name: 'Save new password' }).click();
    await expect(
      page.getByText('This reset link no longer works'),
    ).toBeVisible();
    await expect(
      page.getByRole('button', { name: 'Send a new link' }),
    ).toBeVisible();

    await page.goto('/reset-password');
    await expect(
      page.getByText('This reset link no longer works'),
    ).toBeVisible();
  });

  test('the reset page asks browsers not to send the token as a referrer', async ({
    page,
  }) => {
    await mockApi(page, signedOut);
    await page.goto('/reset-password?token=abc');
    await expect(page.locator('meta[name="referrer"]')).toHaveAttribute(
      'content',
      'no-referrer',
    );
  });
});

test.describe('theme switch', () => {
  test('every auth page can switch theme, and the brand panel stays ink', async ({
    page,
  }) => {
    await mockApi(page, signedOut);
    await page.goto('/login');
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
    const panel = page.locator('aside');
    await expect(panel).toHaveCSS('background-color', 'rgb(20, 23, 26)');

    await page.getByRole('button', { name: 'Dark' }).click();
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
    await expect(page.locator('body')).toHaveCSS(
      'background-color',
      'rgb(14, 17, 19)',
    );
    await expect(panel).toHaveCSS('background-color', 'rgb(20, 23, 26)');

    await page.getByRole('button', { name: 'Light' }).click();
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
  });

  test('the brand panel carries one line about the page and no technical wording', async ({
    page,
  }) => {
    await mockApi(page, signedOut);
    for (const [path, line] of [
      ['/login', 'Every parcel, point to point.'],
      ['/register', 'From first parcel to hundredth.'],
      ['/forgot-password', 'Back on the route in a minute.'],
    ] as const) {
      await page.goto(path);
      await expect(page.getByRole('heading', { name: line })).toBeVisible();
      await expect(page.getByText(/cookie|token|HttpOnly|E\.164/i)).toHaveCount(
        0,
      );
    }
  });
});

test.describe('dark theme', () => {
  test.use({ colorScheme: 'dark' });

  test('the login page follows the dark tokens and the panel stays ink', async ({
    page,
  }) => {
    await mockApi(page, signedOut);
    await page.goto('/login');

    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
    await expect(page.locator('body')).toHaveCSS(
      'background-color',
      'rgb(14, 17, 19)',
    );
    await expect(page.locator('aside')).toHaveCSS(
      'background-color',
      'rgb(20, 23, 26)',
    );
  });
});
