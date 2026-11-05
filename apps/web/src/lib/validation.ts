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

export function validateCurrentPassword(value: string): string | undefined {
  return value ? undefined : 'Enter your current password.';
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

/** The part after +94 of a Sri Lankan number, or undefined for any other number. */
export function toSriLankanLocal(e164: string): string | undefined {
  return e164.startsWith('+94') ? e164.slice(3) : undefined;
}

/** Spaces are allowed while typing; the API wants the digits after a plus sign. */
export function toInternational(value: string): string {
  return value.replace(/\s/g, '');
}

/** A number in international form, for the rare account that is not Sri Lankan. */
export function validateInternationalPhone(value: string): string | undefined {
  return /^\+[1-9]\d{1,14}$/.test(toInternational(value))
    ? undefined
    : 'Enter the full number with its country code, for example +44 20 7123 4567.';
}

export function validateNewPassword(value: string): string | undefined {
  return checkPassword(value).isValid
    ? undefined
    : 'Use at least 8 characters (and no more than 72 bytes) with an upper case letter, a lower case letter and a digit.';
}

export function validateConfirmation(
  password: string,
  confirmation: string,
): string | undefined {
  return password === confirmation
    ? undefined
    : 'The two passwords do not match.';
}

/** Up to two decimals, as the API stores money and weights. */
const DECIMAL = /^\d+(\.\d{1,2})?$/;

/** A base fee must be above 0. A band charge or a surcharge may be 0. */
export function validateAmount(
  value: string,
  { isZeroAllowed }: { isZeroAllowed: boolean },
): string | undefined {
  const text = value.trim();
  const isValid = DECIMAL.test(text) && (isZeroAllowed || Number(text) > 0);
  if (isValid) return undefined;
  return isZeroAllowed
    ? 'Enter an amount of 0 or more, with up to two decimals.'
    : 'Enter an amount above 0, with up to two decimals.';
}

export function validateWeight(value: string): string | undefined {
  return DECIMAL.test(value.trim())
    ? undefined
    : 'Enter a weight of 0 or more, with up to two decimals.';
}
/** Our wording for a tracking number in the wrong format (board page 37). */
export const TRACKING_NUMBER_MESSAGE =
  'Check the number. Ours are 11 characters, starting SDK or SDN.';

/**
 * Our own copy for a field the API rejected. The server's wording is never
 * shown: it is written for developers and may repeat what was typed.
 */
const serverMessages: Record<string, string> = {
  email: 'Enter a complete email address.',
  password: 'Choose a password that meets the rules below.',
  newPassword: 'Choose a password that meets the rules below.',
  currentPassword: 'Enter your current password.',
  fullName: 'Enter between 2 and 150 characters.',
  phone: 'Enter a full number, for example 71 234 5678.',
  code: 'Enter a code of up to 20 characters.',
  name: 'Enter a name of up to 100 characters.',
  districts: 'Enter the districts, separated by commas.',
  originZoneId: 'Choose a zone.',
  destinationZoneId: 'Choose a zone.',
  baseFee: 'Enter an amount above 0, with up to two decimals.',
  charge: 'Enter an amount of 0 or more, with up to two decimals.',
  surcharge: 'Enter an amount of 0 or more, with up to two decimals.',
  minWeightKg: 'Enter a weight of 0 or more, with up to two decimals.',
  maxWeightKg: 'Enter a weight above the "over" weight.',
  trackingNumber: TRACKING_NUMBER_MESSAGE,
  role: 'Choose a role.',
  courierProfile: 'Check the courier details.',
  'courierProfile.vehicleType': 'Use 30 characters or fewer.',
  'courierProfile.vehicleNumber': 'Use 20 characters or fewer.',
  'courierProfile.homeZoneId': 'Choose a zone from the list.',
};

export function serverFieldMessage(field: string): string | undefined {
  return serverMessages[field];
}
