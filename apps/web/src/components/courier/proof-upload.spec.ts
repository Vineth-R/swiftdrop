import { describe, expect, it } from 'vitest';

import {
  MAX_PROOF_PHOTO_BYTES,
  isProofLimitReached,
  proofPhotoError,
  validateProofDraft,
} from './proof-upload';

const jpeg = { type: 'image/jpeg', size: 1024 };
const png = { type: 'image/png', size: 1024 };

describe('proofPhotoError', () => {
  it('accepts JPEG and PNG photos', () => {
    expect(proofPhotoError(jpeg)).toBeUndefined();
    expect(proofPhotoError(png)).toBeUndefined();
  });

  it('rejects an unsupported file type', () => {
    expect(proofPhotoError({ type: 'image/gif', size: 1024 })).toBe(
      'Choose a JPEG or PNG image.',
    );
    expect(proofPhotoError({ type: 'text/plain', size: 1024 })).toBe(
      'Choose a JPEG or PNG image.',
    );
    expect(proofPhotoError({ type: '', size: 1024 })).toBe(
      'Choose a JPEG or PNG image.',
    );
  });

  it('accepts exactly 5 MiB and rejects one byte more', () => {
    expect(
      proofPhotoError({ type: 'image/jpeg', size: MAX_PROOF_PHOTO_BYTES }),
    ).toBeUndefined();
    expect(
      proofPhotoError({ type: 'image/jpeg', size: MAX_PROOF_PHOTO_BYTES + 1 }),
    ).toBe('Photo must be 5 MiB or smaller.');
  });
});

describe('validateProofDraft', () => {
  it('requires a photo, recipient name or delivery note', () => {
    expect(
      validateProofDraft({ photo: null, recipientName: '', note: '' }),
    ).toEqual({ form: 'Add a photo, recipient name or delivery note.' });
    expect(
      validateProofDraft({ photo: null, recipientName: '   ', note: '\t' }),
    ).toEqual({ form: 'Add a photo, recipient name or delivery note.' });
  });

  it('accepts any single proof field', () => {
    expect(
      validateProofDraft({ photo: jpeg, recipientName: '', note: '' }),
    ).toEqual({});
    expect(
      validateProofDraft({ photo: null, recipientName: 'Nimali', note: '' }),
    ).toEqual({});
    expect(
      validateProofDraft({
        photo: null,
        recipientName: '',
        note: 'Handed over',
      }),
    ).toEqual({});
  });

  it('counts Unicode code points for the recipient and note limits', () => {
    const recipient = '\u{1F600}'.repeat(150);
    const note = '\u{1F600}'.repeat(500);

    expect(
      validateProofDraft({
        photo: null,
        recipientName: recipient,
        note,
      }),
    ).toEqual({});
    expect(
      validateProofDraft({
        photo: null,
        recipientName: `${recipient}\u{1F600}`,
        note,
      }),
    ).toEqual({
      recipientName: 'Recipient name must be 150 characters or fewer.',
    });
    expect(
      validateProofDraft({
        photo: null,
        recipientName: recipient,
        note: `${note}\u{1F600}`,
      }),
    ).toEqual({
      note: 'Delivery note must be 500 characters or fewer.',
    });
  });
});

describe('isProofLimitReached', () => {
  it('matches the explicit proof-limit code', () => {
    expect(
      isProofLimitReached({ code: 'PROOF_LIMIT_REACHED', status: 409 }),
    ).toBe(true);
  });

  it('matches the mapped 409 conflict fallback', () => {
    expect(isProofLimitReached({ code: 'CONFLICT', status: 409 })).toBe(true);
  });

  it('ignores unrelated errors', () => {
    expect(isProofLimitReached(null)).toBe(false);
    expect(isProofLimitReached(undefined)).toBe(false);
    expect(isProofLimitReached({ code: 'CONFLICT', status: 400 })).toBe(false);
    expect(isProofLimitReached({ code: 'RATE_LIMITED', status: 429 })).toBe(
      false,
    );
    expect(isProofLimitReached({ code: 'INTERNAL', status: 500 })).toBe(false);
  });
});
