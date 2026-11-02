export const THEME_STORAGE_KEY = 'sd-theme';

export type ThemePreference = 'light' | 'dark' | 'system';

/**
 * Runs inline in <head> before first paint so the page never flashes the
 * wrong theme. A stored 'light' or 'dark' wins; anything else follows the
 * operating system. Mirrors resolveTheme() below.
 */
export const THEME_INIT = `(function(){var p=null;try{p=localStorage.getItem('${THEME_STORAGE_KEY}')}catch(e){}var d=p==='dark'||(p!=='light'&&window.matchMedia('(prefers-color-scheme: dark)').matches);document.documentElement.setAttribute('data-theme',d?'dark':'light')})()`;

export function readPreference(): ThemePreference {
  try {
    const stored = window.localStorage.getItem(THEME_STORAGE_KEY);
    return stored === 'light' || stored === 'dark' ? stored : 'system';
  } catch {
    return 'system';
  }
}

export function resolveTheme(preference: ThemePreference): 'light' | 'dark' {
  if (preference === 'system') {
    return window.matchMedia('(prefers-color-scheme: dark)').matches
      ? 'dark'
      : 'light';
  }
  return preference;
}

/** Applies the preference to <html> and remembers it, when storage allows. */
export function applyPreference(preference: ThemePreference): void {
  document.documentElement.setAttribute('data-theme', resolveTheme(preference));
  try {
    if (preference === 'system') {
      window.localStorage.removeItem(THEME_STORAGE_KEY);
    } else {
      window.localStorage.setItem(THEME_STORAGE_KEY, preference);
    }
  } catch {
    // Storage can be blocked; the theme still applies for this page view.
  }
}
