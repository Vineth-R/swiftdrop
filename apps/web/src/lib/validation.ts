import { checkPassword } from './password';

/**
 * Client checks that guide people before they submit. The API stays the
 * authority: a 422 from the server is shown in the same place (see
 * serverFieldMessage), so a field never carries two messages.
 */

const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

export function validateEmail(value: string): string | undefined {
  const email = value.trim();
  if (!email) return 'Enter your email address.';
  if (!EMAIL.test(email)) return 'Enter a complete email address.';
  return undefined;
}

export function validateRequiredPassword(value: string): string | undefined {
  return value ? undefined : 'Enter your password.';
}

export function validateFullName(value: string): string | undefined {
  const length = [...value.trim()].length;
  return length >= 2 && length <= 150
    ? undefined
    : 'Enter between 2 and 150 characters.';
}

/** Sri Lankan numbers: nine digits after +94, with or without a leading 0. */
export function toSriLankanE164(local: string): string {
  const digits = local
    .replace(/\D/g, '')
    .replace(/^94(?=\d{9}$)/, '')
    .replace(/^0/, '');
  return `+94${digits}`;
}

export function validatePhone(local: string): string | undefined {
  return /^\+94[1-9]\d{8}$/.test(toSriLankanE164(local))
    ? undefined
    : 'Enter a full number, for example 71 234 5678.';
}

export function validateNewPassword(value: string): string | undefined {
  return checkPassword(value).isValid
    ? undefined
    : 'Use 8 to 72 characters with an upper case letter, a lower case letter and a digit.';
}

export function validateConfirmation(
  password: string,
  confirmation: string,
): string | undefined {
  return password === confirmation
    ? undefined
    : 'The two passwords do not match.';
}

/**
 * Our own copy for a field the API rejected. The server's wording is never
 * shown: it is written for developers and may repeat what was typed.
 */
const serverMessages: Record<string, string> = {
  email: 'Enter a complete email address.',
  password: 'Choose a password that meets the rules below.',
  newPassword: 'Choose a password that meets the rules below.',
  fullName: 'Enter between 2 and 150 characters.',
  phone: 'Enter a full number, for example 71 234 5678.',
};

export function serverFieldMessage(field: string): string | undefined {
  return serverMessages[field];
}
