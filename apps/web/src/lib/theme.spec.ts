import { afterEach, describe, expect, it, vi } from 'vitest';

import {
  readPreference,
  resolveTheme,
  THEME_INIT,
  THEME_STORAGE_KEY,
} from './theme';

type StorageLike = {
  getItem: (key: string) => string | null;
  setItem: (key: string, value: string) => void;
  removeItem: (key: string) => void;
};

function makeStorage(initial?: string): StorageLike {
  const values = new Map<string, string>();
  if (initial !== undefined) values.set(THEME_STORAGE_KEY, initial);
  return {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => void values.set(key, value),
    removeItem: (key: string) => void values.delete(key),
  };
}

/** A storage whose every access throws, as in a blocked/private context. */
function makeBlockedStorage(): StorageLike {
  const blocked = () => {
    throw new Error('storage blocked');
  };
  return { getItem: blocked, setItem: blocked, removeItem: blocked };
}

function makeMatchMedia(prefersDark: boolean) {
  return () => ({ matches: prefersDark });
}

/** Runs the inline script the way <head> does, against a fake document. */
function runInit(storage: StorageLike, prefersDark: boolean): string {
  const attributes: Record<string, string> = {};
  const fakeWindow = {
    localStorage: storage,
    matchMedia: makeMatchMedia(prefersDark),
  };
  const fakeDocument = {
    documentElement: {
      setAttribute: (name: string, value: string) => {
        attributes[name] = value;
      },
    },
  };

  new Function('window', 'document', 'localStorage', THEME_INIT)(
    fakeWindow,
    fakeDocument,
    storage,
  );

  return attributes['data-theme'];
}

/** The normal runtime path: read the preference, then resolve it. */
function resolveNormally(
  storage: StorageLike,
  prefersDark: boolean,
): 'light' | 'dark' {
  vi.stubGlobal('window', {
    localStorage: storage,
    matchMedia: makeMatchMedia(prefersDark),
  });
  return resolveTheme(readPreference());
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('THEME_INIT', () => {
  const cases: Array<{ name: string; stored?: string; prefersDark: boolean }> =
    [
      {
        name: 'a stored light preference beats a dark OS',
        stored: 'light',
        prefersDark: true,
      },
      {
        name: 'a stored dark preference beats a light OS',
        stored: 'dark',
        prefersDark: false,
      },
      { name: 'no stored preference follows a dark OS', prefersDark: true },
      { name: 'no stored preference follows a light OS', prefersDark: false },
      {
        name: 'an unknown stored value follows the OS',
        stored: 'blue',
        prefersDark: true,
      },
    ];

  it.each(cases)('$name', ({ stored, prefersDark }) => {
    expect(runInit(makeStorage(stored), prefersDark)).toBe(
      resolveNormally(makeStorage(stored), prefersDark),
    );
  });

  it('follows a dark OS when storage is blocked', () => {
    const storage = makeBlockedStorage();
    expect(runInit(storage, true)).toBe('dark');
    expect(resolveNormally(storage, true)).toBe('dark');
  });

  it('follows a light OS when storage is blocked', () => {
    const storage = makeBlockedStorage();
    expect(runInit(storage, false)).toBe('light');
    expect(resolveNormally(storage, false)).toBe('light');
  });
});
