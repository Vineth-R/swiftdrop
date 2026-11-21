import { describe, expect, it } from 'vitest';

import { ApiError } from './api-errors';
import { classifyApiError } from './api-problem';
import { RETRY_AFTER_FALLBACK_SECONDS } from './constants';

const REQUEST_ID = '7f2c9a14-0b63-4de8-9f10-5c2e8d3a1b77';

describe('classifyApiError', () => {
  it('keeps the Retry-After seconds on a rate limit', () => {
    const error = new ApiError({
      code: 'RATE_LIMITED',
      status: 429,
      retryAfter: 30,
    });

    expect(classifyApiError(error)).toEqual({
      kind: 'rate-limited',
      seconds: 30,
    });
  });

  it('falls back when a rate limit has no usable Retry-After', () => {
    const error = new ApiError({ code: 'RATE_LIMITED', status: 429 });

    expect(classifyApiError(error)).toEqual({
      kind: 'rate-limited',
      seconds: RETRY_AFTER_FALLBACK_SECONDS,
    });
  });

  it('preserves validation issues', () => {
    const issues = [
      { field: 'email', issue: 'must be an email' },
      { field: 'phone', issue: 'must be a number' },
    ];
    const error = new ApiError({
      code: 'VALIDATION_FAILED',
      status: 422,
      issues,
    });

    expect(classifyApiError(error)).toEqual({ kind: 'validation', issues });
  });

  it('classifies a network failure', () => {
    const error = new ApiError({ code: 'NETWORK', status: 0 });

    expect(classifyApiError(error)).toEqual({ kind: 'network' });
  });

  it('keeps the request ID for an internal fault', () => {
    const error = new ApiError({
      code: 'INTERNAL',
      status: 500,
      requestId: REQUEST_ID,
    });

    expect(classifyApiError(error)).toEqual({
      kind: 'server',
      requestId: REQUEST_ID,
    });
  });

  it('handles an internal fault without a request ID', () => {
    const error = new ApiError({ code: 'INTERNAL', status: 500 });

    expect(classifyApiError(error)).toEqual({
      kind: 'server',
      requestId: undefined,
    });
  });

  it('never exposes a reference for a non-server code', () => {
    const error = new ApiError({
      code: 'FORBIDDEN',
      status: 403,
      requestId: REQUEST_ID,
    });

    expect(classifyApiError(error)).toEqual({ kind: 'server' });
  });

  it('treats an unknown thrown value as a server fault', () => {
    expect(classifyApiError(new Error('boom'))).toEqual({ kind: 'server' });
    expect(classifyApiError('boom')).toEqual({ kind: 'server' });
    expect(classifyApiError(null)).toEqual({ kind: 'server' });
  });

  it('treats an object that only looks like an API error as unknown', () => {
    const impostor = Object.assign(new Error('boom'), {
      code: 'RATE_LIMITED',
      status: 429,
    });

    expect(classifyApiError(impostor)).toEqual({ kind: 'server' });
  });
});
