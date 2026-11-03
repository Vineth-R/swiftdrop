import { test, expect, type Page } from '@playwright/test';

import { fail, ok, REQUEST_ID, SERVER_WORDS, user } from './support/mock-api';
import { names, signInAs, type Role } from './support/sign-in';

test.use({ colorScheme: 'light' });

const name = (page: Page) => page.locator('#profile-name');
const phone = (page: Page) => page.locator('#profile-phone');
const save = (page: Page) =>
  page.getByRole('button', { name: /^(Save changes|Saving)$/ });

/** What the API returns after a save: the signed-in customer with some changes. */
const saved = (changes: Record<string, unknown>) =>
  ok({
    ...user,
    role: 'CUSTOMER',
    fullName: names.CUSTOMER,
    email: 'customer@example.com',
    ...changes,
  });

test.describe('profile', () => {
  test.beforeEach(async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
  });

  test('shows who is signed in and the details that can change', async ({
    page,
  }) => {
    await signInAs(page, 'CUSTOMER');
    await page.goto('/profile');

    await expect(
      page.getByRole('heading', { level: 1, name: 'Profile' }),
    ).toBeVisible();
    const summary = page.getByRole('region', { name: 'Account' });
    await expect(summary).toContainText('Nimali Perera');
    await expect(summary).toContainText('customer@example.com');
    await expect(summary).toContainText('CUSTOMER');
    await expect(summary).toContainText('Email and password');
    await expect(summary).toContainText('2 November 2026');

    await expect(name(page)).toHaveValue('Nimali Perera');
    await expect(phone(page)).toHaveValue('771234567');
    await expect(page.locator('#profile-email')).toHaveValue(
      'customer@example.com',
    );
    await expect(page.locator('#profile-email')).toHaveJSProperty(
      'readOnly',
      true,
    );
  });

  for (const role of [
    'CUSTOMER',
    'MERCHANT',
    'COURIER',
    'DISPATCHER',
    'ADMIN',
  ] as Role[]) {
    test(`every role can open it: ${role}`, async ({ page }) => {
      await signInAs(page, role);
      await page.goto('/profile');

      await expect(
        page.getByRole('heading', { level: 1, name: 'Profile' }),
      ).toBeVisible();
      await expect(page.getByRole('region', { name: 'Account' })).toContainText(
        names[role],
      );
    });
  }

  test('the account menu leads to both pages', async ({ page }) => {
    await signInAs(page, 'ADMIN');
    await page.goto('/dashboard');

    await page.getByRole('button', { name: 'Account menu' }).click();
    await page.getByRole('menuitem', { name: 'Profile' }).click();
    await expect(page).toHaveURL(/\/profile$/);

    await page.getByRole('button', { name: 'Account menu' }).click();
    await page.getByRole('menuitem', { name: 'Change password' }).click();
    await expect(page).toHaveURL(/\/profile\/security$/);
    await expect(
      page.getByRole('heading', { level: 1, name: 'Security' }),
    ).toBeVisible();
  });

  test('saving is off until something changes', async ({ page }) => {
    await signInAs(page, 'CUSTOMER');
    await page.goto('/profile');

    await expect(save(page)).toBeDisabled();
    await name(page).fill('Nimali P. Perera');
    await expect(save(page)).toBeEnabled();
    await name(page).fill('Nimali Perera');
    await expect(save(page)).toBeDisabled();
  });

  test('sends only what changed, trimmed, then shows the new details everywhere', async ({
    page,
  }) => {
    const { calls } = await signInAs(page, 'CUSTOMER', {
      'PATCH /users/me': () => saved({ fullName: 'Nimali P. Perera' }),
    });
    await page.goto('/profile');

    await name(page).fill('  Nimali P. Perera  ');
    await save(page).click();

    await expect(
      page.getByRole('status').filter({ hasText: 'Details saved' }),
    ).toBeVisible();
    expect(calls.find((call) => call.key === 'PATCH /users/me')?.body).toEqual({
      fullName: 'Nimali P. Perera',
    });
    await expect(page.getByRole('region', { name: 'Account' })).toContainText(
      'Nimali P. Perera',
    );
    await expect(
      page.getByRole('button', { name: 'Account menu' }),
    ).toContainText('Nimali P. Perera');
    await expect(name(page)).toHaveValue('Nimali P. Perera');
    await expect(save(page)).toBeDisabled();
  });

  test('a typed Sri Lankan number is sent in international form', async ({
    page,
  }) => {
    const { calls } = await signInAs(page, 'CUSTOMER', {
      'PATCH /users/me': () => saved({ phone: '+94712223344' }),
    });
    await page.goto('/profile');

    await phone(page).fill('071 222 3344');
    await save(page).click();

    await expect(page.getByText('Details saved')).toBeVisible();
    expect(calls.find((call) => call.key === 'PATCH /users/me')?.body).toEqual({
      phone: '+94712223344',
    });
    await expect(phone(page)).toHaveValue('712223344');
  });

  test('changing both fields sends both', async ({ page }) => {
    const { calls } = await signInAs(page, 'CUSTOMER', {
      'PATCH /users/me': () =>
        saved({ fullName: 'Nimali P. Perera', phone: '+94712223344' }),
    });
    await page.goto('/profile');

    await name(page).fill('Nimali P. Perera');
    await phone(page).fill('712223344');
    await save(page).click();

    await expect(page.getByText('Details saved')).toBeVisible();
    expect(calls.find((call) => call.key === 'PATCH /users/me')?.body).toEqual({
      fullName: 'Nimali P. Perera',
      phone: '+94712223344',
    });
  });

  test('checks the fields before sending anything', async ({ page }) => {
    const { calls } = await signInAs(page, 'CUSTOMER');
    await page.goto('/profile');

    await name(page).fill('A');
    await phone(page).fill('123');
    await save(page).click();

    await expect(
      page.getByText('Enter between 2 and 150 characters.'),
    ).toBeVisible();
    await expect(
      page.getByText('Enter a full number, for example 71 234 5678.'),
    ).toBeVisible();
    await expect(name(page)).toBeFocused();
    expect(calls.some((call) => call.key === 'PATCH /users/me')).toBe(false);

    await name(page).fill('Nimali Perera');
    await expect(
      page.getByText('Enter between 2 and 150 characters.'),
    ).toBeHidden();
  });

  test('a phone number cannot be cleared once the account has one', async ({
    page,
  }) => {
    const { calls } = await signInAs(page, 'CUSTOMER');
    await page.goto('/profile');

    await phone(page).fill('');
    await name(page).fill('Nimali P. Perera');
    await save(page).click();

    await expect(
      page.getByText('Enter a full number, for example 71 234 5678.'),
    ).toBeVisible();
    expect(calls.some((call) => call.key === 'PATCH /users/me')).toBe(false);
  });

  test("a field the API rejects is explained in our words, never the server's", async ({
    page,
  }) => {
    await signInAs(page, 'CUSTOMER', {
      'PATCH /users/me': fail(422, 'VALIDATION_ERROR', {
        details: [{ field: 'phone', issue: SERVER_WORDS }],
      }),
    });
    await page.goto('/profile');

    await phone(page).fill('712223344');
    await save(page).click();

    await expect(
      page.getByText('Enter a full number, for example 71 234 5678.'),
    ).toBeVisible();
    await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
    await phone(page).fill('712223345');
    await expect(
      page.getByText('Enter a full number, for example 71 234 5678.'),
    ).toBeHidden();
  });

  test('too many requests keeps what was typed and makes the button wait', async ({
    page,
  }) => {
    await signInAs(page, 'CUSTOMER', {
      'PATCH /users/me': fail(429, 'RATE_LIMITED', {
        headers: { 'Retry-After': '30' },
      }),
    });
    await page.goto('/profile');

    await name(page).fill('Nimali P. Perera');
    await save(page).click();

    await expect(
      page.getByRole('status').filter({ hasText: 'Too many attempts' }),
    ).toBeVisible();
    await expect(name(page)).toHaveValue('Nimali P. Perera');
    await expect(save(page)).toBeDisabled();
  });

  test('a server fault shows a Reference and keeps what was typed', async ({
    page,
  }) => {
    await signInAs(page, 'CUSTOMER', {
      'PATCH /users/me': fail(500, 'INTERNAL_ERROR'),
    });
    await page.goto('/profile');

    await name(page).fill('Nimali P. Perera');
    await save(page).click();

    // Next also renders a hidden role="alert" route announcer, so pick ours by its text.
    const fault = page
      .getByRole('alert')
      .filter({ hasText: 'Something broke on our side' });
    await expect(fault).toContainText(REQUEST_ID);
    await expect(name(page)).toHaveValue('Nimali P. Perera');
    await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
  });

  test('an expired access token is renewed and the save goes through', async ({
    page,
  }) => {
    let attempts = 0;
    const { calls } = await signInAs(page, 'CUSTOMER', {
      'PATCH /users/me': () => {
        attempts += 1;
        return attempts === 1
          ? fail(401, 'TOKEN_EXPIRED')
          : saved({ fullName: 'Nimali P. Perera' });
      },
      'POST /auth/refresh': ok({ expiresIn: 900 }),
    });
    await page.goto('/profile');

    await name(page).fill('Nimali P. Perera');
    await save(page).click();

    await expect(page.getByText('Details saved')).toBeVisible();
    expect(
      calls.map((call) => call.key).filter((key) => key !== 'GET /users/me'),
    ).toEqual(['PATCH /users/me', 'POST /auth/refresh', 'PATCH /users/me']);
  });

  test('an account with no phone yet, as after a Google sign-up, can save a name without one', async ({
    page,
  }) => {
    const { calls } = await signInAs(
      page,
      'CUSTOMER',
      {
        'PATCH /users/me': () =>
          saved({
            phone: '',
            authProvider: 'GOOGLE',
            fullName: 'Nimali P. Perera',
          }),
      },
      { phone: '', authProvider: 'GOOGLE' },
    );
    await page.goto('/profile');

    await expect(page.getByRole('region', { name: 'Account' })).toContainText(
      'Google',
    );
    await expect(phone(page)).toHaveValue('');
    await name(page).fill('Nimali P. Perera');
    await save(page).click();

    await expect(page.getByText('Details saved')).toBeVisible();
    expect(calls.find((call) => call.key === 'PATCH /users/me')?.body).toEqual({
      fullName: 'Nimali P. Perera',
    });
  });

  test('a number that is not Sri Lankan is shown and edited whole', async ({
    page,
  }) => {
    const { calls } = await signInAs(
      page,
      'CUSTOMER',
      { 'PATCH /users/me': () => saved({ phone: '+442071239999' }) },
      { phone: '+442071234567' },
    );
    await page.goto('/profile');

    await expect(phone(page)).toHaveValue('+442071234567');
    await phone(page).fill('+44 20 7123 9999');
    await save(page).click();

    await expect(page.getByText('Details saved')).toBeVisible();
    expect(calls.find((call) => call.key === 'PATCH /users/me')?.body).toEqual({
      phone: '+442071239999',
    });
  });

  test('at 375px both pages fit without scrolling sideways', async ({
    page,
  }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    await signInAs(page, 'CUSTOMER');

    for (const path of ['/profile', '/profile/security']) {
      await page.goto(path);
      await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
      const overflow = await page.evaluate(
        'document.documentElement.scrollWidth - document.documentElement.clientWidth',
      );
      expect(overflow, path).toBe(0);
    }
  });

  test('a courier sees it inside the courier shell with Profile marked', async ({
    page,
  }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await signInAs(page, 'COURIER');
    await page.goto('/profile');

    await expect(
      page.getByRole('heading', { level: 1, name: 'Profile' }),
    ).toBeVisible();
    await expect(page.getByRole('link', { name: 'Profile' })).toHaveAttribute(
      'aria-current',
      'page',
    );
  });
});

test.describe('security', () => {
  test.beforeEach(async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
  });

  const current = (page: Page) => page.locator('#security-current');
  const next = (page: Page) => page.locator('#security-password');
  const confirm = (page: Page) => page.locator('#security-confirm');
  const submit = (page: Page) =>
    page.getByRole('button', { name: /^(Change password|Saving)$/ });

  async function fillValid(page: Page) {
    await current(page).fill('Str0ngPass!');
    await next(page).fill('N3wStrongPass!');
    await confirm(page).fill('N3wStrongPass!');
  }

  test('checks the form before sending anything', async ({ page }) => {
    const { calls } = await signInAs(page, 'CUSTOMER');
    await page.goto('/profile/security');

    await submit(page).click();
    await expect(page.getByText('Enter your current password.')).toBeVisible();
    await expect(current(page)).toBeFocused();

    await current(page).fill('Str0ngPass!');
    await next(page).fill('short');
    await confirm(page).fill('different');
    await submit(page).click();
    await expect(
      page.getByText(
        'Use 8 to 72 characters with an upper case letter, a lower case letter and a digit.',
      ),
    ).toBeVisible();
    await expect(next(page)).toBeFocused();

    await next(page).fill('N3wStrongPass!');
    await submit(page).click();
    await expect(
      page.getByText('The two passwords do not match.'),
    ).toBeVisible();
    expect(calls.some((call) => call.key === 'PUT /users/me/password')).toBe(
      false,
    );
  });

  test('changes the password, says every session ended, and leads to login', async ({
    page,
  }) => {
    let signedIn = true;
    const { calls } = await signInAs(page, 'CUSTOMER', {
      'GET /users/me': () =>
        signedIn ? saved({}) : fail(401, 'UNAUTHENTICATED'),
      'PUT /users/me/password': () => {
        signedIn = false; // the API clears the cookies with its 204
        return { status: 204 };
      },
    });
    await page.goto('/profile/security');

    await fillValid(page);
    await submit(page).click();

    await expect(
      page.getByRole('heading', { name: 'Password changed' }),
    ).toBeVisible();
    await expect(
      page.getByText('signed out on every device, this one included'),
    ).toBeVisible();
    expect(
      calls.find((call) => call.key === 'PUT /users/me/password')?.body,
    ).toEqual({
      currentPassword: 'Str0ngPass!',
      newPassword: 'N3wStrongPass!',
    });

    await page.getByRole('link', { name: 'Go to log in' }).click();
    await expect(page).toHaveURL(/\/login$/);
    await expect(page.getByRole('heading', { name: 'Log in' })).toBeVisible();
  });

  test('a wrong current password is said at the field and does not sign anyone out', async ({
    page,
  }) => {
    const { calls } = await signInAs(page, 'CUSTOMER', {
      'PUT /users/me/password': fail(401, 'INVALID_CREDENTIALS'),
      'POST /auth/refresh': ok({ expiresIn: 900 }),
    });
    await page.goto('/profile/security');

    await fillValid(page);
    await submit(page).click();

    await expect(
      page.getByText('That is not your current password.'),
    ).toBeVisible();
    await expect(current(page)).toBeFocused();
    await expect(page).toHaveURL(/\/profile\/security$/);
    await expect(next(page)).toHaveValue('N3wStrongPass!');
    await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
    // The 401 is a wrong password, not an expired session: nothing is renewed
    // and the person stays signed in.
    expect(calls.some((call) => call.key === 'POST /auth/refresh')).toBe(false);
    await expect(
      page.getByRole('button', { name: 'Account menu' }),
    ).toBeVisible();

    await current(page).fill('Str0ngPass!!');
    await expect(
      page.getByText('That is not your current password.'),
    ).toBeHidden();
  });

  test('a Google-only account is told there is no password to change', async ({
    page,
  }) => {
    await signInAs(page, 'CUSTOMER', {
      'PUT /users/me/password': fail(409, 'PASSWORD_NOT_SET'),
    });
    await page.goto('/profile/security');

    await fillValid(page);
    await submit(page).click();

    const notice = page
      .getByRole('status')
      .filter({ hasText: 'This account signs in with Google' });
    await expect(notice).toBeVisible();
    await expect(
      notice.getByRole('link', { name: 'Back to profile' }),
    ).toBeVisible();
    await expect(current(page)).toHaveCount(0);
  });

  test('an account known to sign in with Google never sees the form', async ({
    page,
  }) => {
    await signInAs(page, 'CUSTOMER', {}, { authProvider: 'GOOGLE' });
    await page.goto('/profile/security');

    await expect(
      page.getByText('This account signs in with Google'),
    ).toBeVisible();
    await expect(current(page)).toHaveCount(0);
  });

  test('a password the API rejects is explained in our words', async ({
    page,
  }) => {
    await signInAs(page, 'CUSTOMER', {
      'PUT /users/me/password': fail(422, 'VALIDATION_ERROR', {
        details: [{ field: 'newPassword', issue: SERVER_WORDS }],
      }),
    });
    await page.goto('/profile/security');

    await fillValid(page);
    await submit(page).click();

    await expect(
      page.getByText('Choose a password that meets the rules below.'),
    ).toBeVisible();
    await expect(page.getByText(SERVER_WORDS)).toHaveCount(0);
  });

  test('too many tries keeps what was typed and makes the button wait', async ({
    page,
  }) => {
    await signInAs(page, 'CUSTOMER', {
      'PUT /users/me/password': fail(429, 'RATE_LIMITED', {
        headers: { 'Retry-After': '40' },
      }),
    });
    await page.goto('/profile/security');

    await fillValid(page);
    await submit(page).click();

    await expect(
      page.getByRole('status').filter({ hasText: 'Too many attempts' }),
    ).toBeVisible();
    await expect(submit(page)).toBeDisabled();
    await expect(current(page)).toHaveValue('Str0ngPass!');
  });

  test('a server fault shows a Reference and keeps the form', async ({
    page,
  }) => {
    await signInAs(page, 'CUSTOMER', {
      'PUT /users/me/password': fail(500, 'INTERNAL_ERROR'),
    });
    await page.goto('/profile/security');

    await fillValid(page);
    await submit(page).click();

    await expect(
      page
        .getByRole('alert')
        .filter({ hasText: 'Something broke on our side' }),
    ).toContainText(REQUEST_ID);
    await expect(next(page)).toHaveValue('N3wStrongPass!');
  });
});
