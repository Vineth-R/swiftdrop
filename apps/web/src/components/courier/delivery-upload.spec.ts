import { describe, expect, it } from 'vitest';

import {
  validateDelivery,
  validateFailure,
  validatePickup,
} from './delivery-upload';

const jpeg = { type: 'image/jpeg', size: 1024 };
const png = { type: 'image/png', size: 1024 };

describe('validatePickup', () => {
  it('requires a photo', () => {
    expect(validatePickup(null).photo).toBeDefined();
  });

  it('rejects a non-JPEG or PNG file', () => {
    expect(
      validatePickup({ type: 'image/gif', size: 1024 }).photo,
    ).toBeDefined();
  });

  it('rejects a photo over 5 MiB', () => {
    expect(
      validatePickup({ type: 'image/png', size: 5 * 1024 * 1024 + 1 }).photo,
    ).toBeDefined();
  });

  it('accepts a JPEG or PNG up to the limit', () => {
    expect(validatePickup(jpeg)).toEqual({});
    expect(validatePickup(png)).toEqual({});
    expect(
      validatePickup({ type: 'image/png', size: 5 * 1024 * 1024 }),
    ).toEqual({});
  });
});

describe('validateDelivery', () => {
  it('requires the recipient name', () => {
    expect(
      validateDelivery({ photo: png, recipientName: '   ', note: '' })
        .recipientName,
    ).toBeDefined();
  });

  it('requires a proof photo', () => {
    expect(
      validateDelivery({ photo: null, recipientName: 'Amali', note: '' }).photo,
    ).toBeDefined();
  });

  it('rejects an unsupported or oversized photo', () => {
    expect(
      validateDelivery({
        photo: { type: 'image/webp', size: 1024 },
        recipientName: 'Amali',
        note: '',
      }).photo,
    ).toBeDefined();
    expect(
      validateDelivery({
        photo: { type: 'image/jpeg', size: 6 * 1024 * 1024 },
        recipientName: 'Amali',
        note: '',
      }).photo,
    ).toBeDefined();
  });

  it('limits recipient and note length', () => {
    const errors = validateDelivery({
      photo: png,
      recipientName: '名'.repeat(151),
      note: 'n'.repeat(501),
    });
    expect(errors.recipientName).toBeDefined();
    expect(errors.note).toBeDefined();
  });

  it('accepts a complete delivery', () => {
    expect(
      validateDelivery({
        photo: jpeg,
        recipientName: ' Amali Fernando ',
        note: 'Handed over',
      }),
    ).toEqual({});
  });
});

describe('validateFailure', () => {
  it('requires 5 to 255 characters after trimming', () => {
    expect(validateFailure('    ')).toBeDefined();
    expect(validateFailure(' abcd ')).toBeDefined();
    expect(validateFailure('a'.repeat(256))).toBeDefined();
    expect(validateFailure(' abcde ')).toBeUndefined();
    expect(validateFailure('a'.repeat(255))).toBeUndefined();
  });
});
