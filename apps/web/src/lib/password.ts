/**
 * The password rule shown to people, matching the API (Registration, reset and
 * change password): 8 to 72 bytes with one upper case letter, one lower case
 * letter and one digit. The backend stays authoritative; this only guides.
 */
export const PASSWORD_MIN_LENGTH = 8;
export const PASSWORD_MAX_BYTES = 72;

export type PasswordRuleId = 'length' | 'upper' | 'lower' | 'digit';

export type PasswordRule = {
  id: PasswordRuleId;
  label: string;
  isMet: boolean;
};

export type PasswordCheck = {
  rules: PasswordRule[];
  /** How many of the four rules are met, 0 to 4. */
  score: number;
  isValid: boolean;
};

/** Copy for each score, in the order of the design's strength line. */
export const strengthLabels = [
  'Does not meet the rule yet',
  'Weak — two more to go',
  'Almost there',
  'One rule left',
  'Meets the rule',
] as const;

/** UTF-8 byte length, the measurement the API's 72-byte limit uses. */
export function passwordByteLength(value: string): number {
  return new TextEncoder().encode(value).length;
}

export function checkPassword(value: string): PasswordCheck {
  const bytes = passwordByteLength(value);
  const rules: PasswordRule[] = [
    {
      id: 'length',
      label: '8 to 72 characters, maximum 72 bytes',
      isMet: bytes >= PASSWORD_MIN_LENGTH && bytes <= PASSWORD_MAX_BYTES,
    },
    { id: 'upper', label: 'One upper case letter', isMet: /[A-Z]/.test(value) },
    { id: 'lower', label: 'One lower case letter', isMet: /[a-z]/.test(value) },
    { id: 'digit', label: 'One digit', isMet: /[0-9]/.test(value) },
  ];
  const score = rules.filter((rule) => rule.isMet).length;
  return { rules, score, isValid: score === rules.length };
}
