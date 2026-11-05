import { describe, expect, it } from 'vitest';

import {
  checkPassword,
  PASSWORD_MAX_BYTES,
  PASSWORD_MIN_CHARACTERS,
  passwordByteLength,
  passwordCharacterLength,
} from '@/lib/password';

const lengthRuleMet = (value: string) =>
  checkPassword(value).rules.find((rule) => rule.id === 'length')?.isMet ??
  false;

describe('password measurements', () => {
  it('counts ASCII characters and bytes the same', () => {
    expect(passwordCharacterLength('Str0ngPass!')).toBe(11);
    expect(passwordByteLength('Str0ngPass!')).toBe(11);
  });

  it('counts Unicode code points, not UTF-16 code units', () => {
    expect(passwordCharacterLength('😀')).toBe(1);
    expect(passwordByteLength('😀')).toBe(4);
    expect(passwordCharacterLength('𐐀')).toBe(1);
    expect(passwordByteLength('𐐀')).toBe(4);
    expect(passwordCharacterLength('e\u0301')).toBe(2);
    expect(passwordByteLength('e\u0301')).toBe(3);
    expect(passwordCharacterLength('A1a😀😀')).toBe(5);
    expect(passwordByteLength('A1a😀😀')).toBe(11);
  });
});

describe('checkPassword minimum length', () => {
  it('rejects seven ASCII characters', () => {
    const value = 'A1abcde';
    expect(passwordCharacterLength(value)).toBe(PASSWORD_MIN_CHARACTERS - 1);
    expect(lengthRuleMet(value)).toBe(false);
    expect(checkPassword(value).isValid).toBe(false);
  });

  it('accepts eight ASCII characters', () => {
    const value = 'A1abcdef';
    expect(passwordCharacterLength(value)).toBe(PASSWORD_MIN_CHARACTERS);
    expect(lengthRuleMet(value)).toBe(true);
    expect(checkPassword(value).isValid).toBe(true);
  });

  it('rejects five multibyte characters even though they are 11 bytes', () => {
    const value = 'A1a😀😀';
    expect(passwordCharacterLength(value)).toBe(5);
    expect(passwordByteLength(value)).toBe(11);
    expect(lengthRuleMet(value)).toBe(false);
    expect(checkPassword(value).isValid).toBe(false);
  });

  it('accepts eight multibyte characters within the byte limit', () => {
    const value = 'A1aééééé';
    expect(passwordCharacterLength(value)).toBe(PASSWORD_MIN_CHARACTERS);
    expect(passwordByteLength(value)).toBe(13);
    expect(lengthRuleMet(value)).toBe(true);
    expect(checkPassword(value).isValid).toBe(true);
  });
});

describe('checkPassword maximum bytes', () => {
  it('rejects more than 72 UTF-8 bytes', () => {
    const value = `A1${'a'.repeat(PASSWORD_MAX_BYTES - 1)}`;
    expect(passwordByteLength(value)).toBe(PASSWORD_MAX_BYTES + 1);
    expect(lengthRuleMet(value)).toBe(false);
    expect(checkPassword(value).isValid).toBe(false);
  });

  it('accepts exactly 72 UTF-8 bytes', () => {
    const value = `A1${'a'.repeat(PASSWORD_MAX_BYTES - 2)}`;
    expect(passwordByteLength(value)).toBe(PASSWORD_MAX_BYTES);
    expect(checkPassword(value).isValid).toBe(true);
  });

  it('accepts multibyte input that reaches exactly 72 bytes', () => {
    const value = `A1a${'é'.repeat(34)}b`;
    expect(passwordCharacterLength(value)).toBe(38);
    expect(passwordByteLength(value)).toBe(PASSWORD_MAX_BYTES);
    expect(checkPassword(value).isValid).toBe(true);
  });
});

describe('checkPassword Unicode classes', () => {
  it('accepts Unicode upper case, lower case and decimal digit', () => {
    const value = 'Ää١bcdef';
    expect(passwordCharacterLength(value)).toBe(8);
    expect(passwordByteLength(value)).toBe(11);
    const result = checkPassword(value);
    expect(result.rules.find((rule) => rule.id === 'upper')?.isMet).toBe(true);
    expect(result.rules.find((rule) => rule.id === 'lower')?.isMet).toBe(true);
    expect(result.rules.find((rule) => rule.id === 'digit')?.isMet).toBe(true);
    expect(result.isValid).toBe(true);
  });

  it('handles supplementary-plane letters', () => {
    const value = '𐐀𐐨1abcde';
    expect(passwordCharacterLength(value)).toBe(8);
    expect(passwordByteLength(value)).toBe(14);
    const result = checkPassword(value);
    expect(result.rules.find((rule) => rule.id === 'upper')?.isMet).toBe(true);
    expect(result.rules.find((rule) => rule.id === 'lower')?.isMet).toBe(true);
    expect(result.rules.find((rule) => rule.id === 'digit')?.isMet).toBe(true);
    expect(result.isValid).toBe(true);
  });

  it('counts combining marks as separate code points, like Go runes', () => {
    const eight = 'A1ae\u0301bcd';
    expect(passwordCharacterLength(eight)).toBe(8);
    expect(checkPassword(eight).isValid).toBe(true);

    const seven = 'A1ae\u0301bc';
    expect(passwordCharacterLength(seven)).toBe(7);
    expect(checkPassword(seven).isValid).toBe(false);
  });

  it('does not treat a non-decimal numeric as a digit', () => {
    const value = 'Äbcdefg²';
    expect(passwordCharacterLength(value)).toBe(8);
    const result = checkPassword(value);
    expect(result.rules.find((rule) => rule.id === 'digit')?.isMet).toBe(false);
    expect(result.isValid).toBe(false);
  });

  it('does not treat a titlecase letter as upper or lower case', () => {
    const value = 'ǅbcdefg1';
    expect(passwordCharacterLength(value)).toBe(8);
    const result = checkPassword(value);
    expect(result.rules.find((rule) => rule.id === 'upper')?.isMet).toBe(false);
    expect(result.isValid).toBe(false);

    const titlecaseOnly = 'ǅ1234567';
    const onlyResult = checkPassword(titlecaseOnly);
    expect(onlyResult.rules.find((rule) => rule.id === 'upper')?.isMet).toBe(
      false,
    );
    expect(onlyResult.rules.find((rule) => rule.id === 'lower')?.isMet).toBe(
      false,
    );
    expect(onlyResult.isValid).toBe(false);
  });

  it('still requires the other three rules', () => {
    expect(checkPassword('aaaaaaaa').isValid).toBe(false);
  });
});
