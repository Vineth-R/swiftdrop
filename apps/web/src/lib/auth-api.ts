import { api } from './api';
import type { Role } from './tokens';

/** The signed-in user, as the API describes them (API v1.1 sections 2.2 and 2.7). */
export type User = {
  id: string;
  email: string;
  fullName: string;
  phone: string;
  role: Role;
  authProvider?: 'LOCAL' | 'GOOGLE';
  createdAt?: string;
};

type LoginResult = { user: User; expiresIn: number };

export function login(email: string, password: string): Promise<User> {
  return api<LoginResult>('/auth/login', {
    method: 'POST',
    body: { email: email.trim(), password },
  }).then((result) => result.user);
}

export type RegisterInput = {
  fullName: string;
  email: string;
  phone: string;
  password: string;
};

export function register(input: RegisterInput): Promise<User> {
  return api<User>('/auth/register', {
    method: 'POST',
    body: {
      ...input,
      fullName: input.fullName.trim(),
      email: input.email.trim(),
    },
  });
}

export function logout(): Promise<void> {
  return api('/auth/logout', { method: 'POST' });
}

/** Always answers the same way, whether or not the email has an account. */
export function forgotPassword(email: string): Promise<void> {
  return api('/auth/forgot-password', {
    method: 'POST',
    body: { email: email.trim() },
  });
}

export function resetPassword(
  token: string,
  newPassword: string,
): Promise<void> {
  return api('/auth/reset-password', {
    method: 'POST',
    body: { token, newPassword },
  });
}

/** Who is signed in. Renews the session once if the access token has expired. */
export function getMe(): Promise<User> {
  return api<User>('/users/me');
}

/**
 * The link that starts Google sign-in. It is a normal navigation, not a fetch:
 * the API answers with a redirect to Google and sets a browser-binding cookie.
 * `next` must already have passed safeNext().
 */
export function googleLoginUrl(next: string): string {
  return `/api/v1/auth/google/login?redirectTo=${encodeURIComponent(next)}`;
}
