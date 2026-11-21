import { api } from './api';
import { API_BASE } from './constants';
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

export type RegisteredAccount = User & { emailVerified?: boolean };

/** Creates a customer account. It never signs the person in. */
export function register(input: RegisterInput): Promise<RegisteredAccount> {
  return api<RegisteredAccount>('/auth/register', {
    method: 'POST',
    body: {
      ...input,
      fullName: input.fullName.trim(),
      email: input.email.trim(),
    },
  });
}

export function verifyEmail(token: string): Promise<void> {
  return api('/auth/verify-email', { method: 'POST', body: { token } });
}

export function resendVerification(email: string): Promise<void> {
  return api('/auth/resend-verification', {
    method: 'POST',
    body: { email: email.trim() },
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

/** The fields the profile form can change. Send only the ones that changed. */
export type ProfileChange = { fullName?: string; phone?: string };

/** Saves the person's own name and phone (API v1.1 section 2.8). */
export function updateProfile(change: ProfileChange): Promise<User> {
  return api<User>('/users/me', { method: 'PATCH', body: change });
}

/**
 * Changes the person's own password (API v1.1 section 2.9). On success the API
 * signs every session out, this browser's included, so the caller sends the
 * person to the login page.
 */
export function changePassword(
  currentPassword: string,
  newPassword: string,
): Promise<void> {
  return api('/users/me/password', {
    method: 'PUT',
    body: { currentPassword, newPassword },
  });
}

/**
 * The link that starts Google sign-in. It is a normal navigation, not a fetch:
 * the API answers with a redirect to Google and sets a browser-binding cookie.
 * `next` must already have passed safeNext().
 */
export function googleLoginUrl(next: string): string {
  return `${API_BASE}/auth/google/login?redirectTo=${encodeURIComponent(next)}`;
}
