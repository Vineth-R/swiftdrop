'use client';

import { useCallback, useEffect, useState } from 'react';

import { applyPreference, readPreference, type ThemePreference } from './theme';

/**
 * Reads, applies and remembers the theme preference. The initial value is
 * `system` until the client can read storage, matching the pre-paint script in
 * app/layout.tsx.
 */
export function useThemePreference() {
  const [preference, setPreference] = useState<ThemePreference>('system');

  useEffect(() => {
    setPreference(readPreference());
  }, []);

  useEffect(() => {
    if (preference !== 'system') return;
    const query = window.matchMedia('(prefers-color-scheme: dark)');
    const follow = () => applyPreference('system');
    query.addEventListener('change', follow);
    return () => query.removeEventListener('change', follow);
  }, [preference]);

  const sync = useCallback(() => setPreference(readPreference()), []);

  function choose(next: ThemePreference) {
    setPreference(next);
    applyPreference(next);
  }

  return { preference, choose, sync };
}
