import { describe, expect, it } from 'vitest';

import {
  attemptOutcome,
  validateWebhookUrl,
  type WebhookDeliveryAttempt,
} from './webhook-api';

const attempt: WebhookDeliveryAttempt = {
  eventId: 'b7e8c3a2-1d4f-4c6a-9e0b-3f5a7d9c1e2b',
  trackingNumber: 'SDK7M2Q9XP4',
  attemptNumber: 1,
  httpStatus: 200,
  success: true,
  nextRetryAt: null,
  deadLettered: false,
  createdAt: '2026-11-11T04:00:00Z',
};

describe('validateWebhookUrl', () => {
  it('accepts a public https address, with or without spaces around it', () => {
    expect(
      validateWebhookUrl('https://shop.example.com/hooks'),
    ).toBeUndefined();
    expect(validateWebhookUrl('  https://shop.example.com  ')).toBeUndefined();
  });

  it('asks for an address when it is empty', () => {
    expect(validateWebhookUrl('')).toContain('Enter the address');
    expect(validateWebhookUrl('   ')).toContain('Enter the address');
  });

  it('rejects text that is not an address', () => {
    expect(validateWebhookUrl('shop.example.com')).toContain('full address');
  });

  it('rejects http and other schemes', () => {
    expect(validateWebhookUrl('http://shop.example.com')).toContain('https://');
    expect(validateWebhookUrl('ftp://shop.example.com')).toContain('https://');
  });

  it('rejects an address with a user name or password', () => {
    expect(validateWebhookUrl('https://user:pass@shop.example.com')).toContain(
      'user name',
    );
  });

  it('rejects an address longer than 500 characters', () => {
    const long = `https://shop.example.com/${'a'.repeat(500)}`;
    expect(validateWebhookUrl(long)).toContain('500');
  });
});

describe('attemptOutcome', () => {
  it('is delivered for a success', () => {
    expect(attemptOutcome(attempt)).toBe('delivered');
  });

  it('is retrying when a later attempt is scheduled', () => {
    expect(
      attemptOutcome({
        ...attempt,
        success: false,
        httpStatus: 500,
        nextRetryAt: '2026-11-11T04:01:00Z',
      }),
    ).toBe('retrying');
  });

  it('is gave-up after the last attempt', () => {
    expect(
      attemptOutcome({
        ...attempt,
        success: false,
        attemptNumber: 6,
        httpStatus: null,
        deadLettered: true,
      }),
    ).toBe('gave-up');
  });

  it('is failed when nothing else applies', () => {
    expect(
      attemptOutcome({ ...attempt, success: false, httpStatus: 400 }),
    ).toBe('failed');
  });
});
