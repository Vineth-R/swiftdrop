import { describe, expect, it } from 'vitest';

import {
  checkPassword,
  PASSWORD_MAX_BYTES,
  passwordByteLength,
} from '@/lib/password';

describe('passwordByteLength', () => {
  it('counts one byte per ASCII character', () => {
    expect(passwordByteLength('abc')).toBe(3);
    expect(passwordByteLength('Str0ngPass!')).toBe(11);
  });

  it('counts UTF-8 bytes rather than UTF-16 code units', () => {
    expect(passwordByteLength('é')).toBe(2);
    expect(passwordByteLength('😀')).toBe(4);
    // 38 characters, but 72 bytes.
    expect(passwordByteLength(`A1a${'é'.repeat(34)}b`)).toBe(72);
  });

  it('reports the exact byte limit', () => {
    expect(passwordByteLength('a'.repeat(PASSWORD_MAX_BYTES))).toBe(
      PASSWORD_MAX_BYTES,
    );
  });
});

describe('checkPassword length rule', () => {
  it('accepts a password at the byte limit', () => {
    const atLimit = `A1${'a'.repeat(PASSWORD_MAX_BYTES - 2)}`;
    expect(passwordByteLength(atLimit)).toBe(PASSWORD_MAX_BYTES);
    expect(checkPassword(atLimit).isValid).toBe(true);
  });

  it('accepts multibyte input measured in bytes', () => {
    const atLimit = `A1a${'é'.repeat(34)}b`;
    expect(passwordByteLength(atLimit)).toBe(PASSWORD_MAX_BYTES);
    expect(checkPassword(atLimit).isValid).toBe(true);
  });

  it('rejects one byte over the limit', () => {
    const over = `A1${'a'.repeat(PASSWORD_MAX_BYTES - 1)}`;
    expect(passwordByteLength(over)).toBe(PASSWORD_MAX_BYTES + 1);
    const check = checkPassword(over);
    expect(check.isValid).toBe(false);
    expect(check.rules.find((rule) => rule.id === 'length')?.isMet).toBe(false);
  });

  it('still requires the other three rules', () => {
    expect(checkPassword('aaaaaaaa').isValid).toBe(false);
  });
});
