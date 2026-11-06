import { afterEach, describe, expect, it, vi } from 'vitest';

import { DEFAULT_AUTH_ROUTE } from './constants';
import { homeFor } from './nav';
import { safeNext } from './redirect';
import { roles } from './tokens';

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('safeNext', () => {
  it('keeps a safe site path, including its query string', () => {
    expect(safeNext('/dashboard?tab=parcels')).toBe('/dashboard?tab=parcels');
  });

  it('rejects an absolute external URL', () => {
    expect(safeNext('https://evil.example/x')).toBe(DEFAULT_AUTH_ROUTE);
  });

  it('rejects a protocol-relative URL', () => {
    expect(safeNext('//evil.example')).toBe(DEFAULT_AUTH_ROUTE);
  });

  it('rejects a path that uses a backslash', () => {
    expect(safeNext('/\\evil.example')).toBe(DEFAULT_AUTH_ROUTE);
  });

  it('rejects control characters', () => {
    expect(safeNext('/dash\nboard')).toBe(DEFAULT_AUTH_ROUTE);
    expect(safeNext('/dash\u0000board')).toBe(DEFAULT_AUTH_ROUTE);
  });

  it('does not redirect back to an auth page', () => {
    for (const path of [
      '/login',
      '/register',
      '/forgot-password',
      '/reset-password?token=x',
      '/signed-out',
    ]) {
      expect(safeNext(path)).toBe(DEFAULT_AUTH_ROUTE);
    }
  });

  it('uses the fallback for empty input', () => {
    expect(safeNext(null)).toBe(DEFAULT_AUTH_ROUTE);
    expect(safeNext(undefined)).toBe(DEFAULT_AUTH_ROUTE);
    expect(safeNext('')).toBe(DEFAULT_AUTH_ROUTE);
  });

  it('honours a caller-provided fallback, including an empty one', () => {
    expect(safeNext(null, '/signed-out')).toBe('/signed-out');
    expect(safeNext(null, '')).toBe('');
  });

  it('rejects the DEL control character', () => {
    expect(safeNext('/\u007f')).toBe(DEFAULT_AUTH_ROUTE);
  });

  it('falls back when URL construction throws', () => {
    vi.stubGlobal(
      'URL',
      class {
        constructor() {
          throw new TypeError('bad URL');
        }
      },
    );

    expect(safeNext('/dashboard')).toBe(DEFAULT_AUTH_ROUTE);
  });
});

describe('homeFor', () => {
  it('returns a defined home for every role', () => {
    for (const role of roles) {
      expect(homeFor(role)).toBe(DEFAULT_AUTH_ROUTE);
    }
  });
});
