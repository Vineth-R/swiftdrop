/**
 * The password rule shown to people, matching the API (Registration, reset and
 * change password): at least 8 Unicode characters, at most 72 UTF-8 bytes,
 * with an upper case letter, a lower case letter and a digit. The backend stays
 * authoritative; this only guides.
 */
export const PASSWORD_MIN_CHARACTERS = 8;
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

/** Unicode code points, matching Go's utf8.RuneCountInString. */
export function passwordCharacterLength(value: string): number {
  return [...value].length;
}

/** UTF-8 byte length, the measurement the API's 72-byte limit uses. */
export function passwordByteLength(value: string): number {
  return new TextEncoder().encode(value).length;
}

export function checkPassword(value: string): PasswordCheck {
  const characters = passwordCharacterLength(value);
  const bytes = passwordByteLength(value);
  const rules: PasswordRule[] = [
    {
      id: 'length',
      label: 'At least 8 characters, maximum 72 bytes',
      isMet:
        characters >= PASSWORD_MIN_CHARACTERS && bytes <= PASSWORD_MAX_BYTES,
    },
    {
      id: 'upper',
      label: 'One upper case letter',
      isMet: /\p{Lu}/u.test(value),
    },
    {
      id: 'lower',
      label: 'One lower case letter',
      isMet: /\p{Ll}/u.test(value),
    },
    { id: 'digit', label: 'One digit', isMet: /\p{Nd}/u.test(value) },
  ];
  const score = rules.filter((rule) => rule.isMet).length;
  return { rules, score, isValid: score === rules.length };
}
