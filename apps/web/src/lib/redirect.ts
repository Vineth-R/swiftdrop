const AUTH_PATHS =
  /^\/(login|register|forgot-password|reset-password|signed-out)(\/|\?|#|$)/;

function hasControlCharacter(value: string): boolean {
  return [...value].some((char) => {
    const code = char.charCodeAt(0);
    return code < 0x20 || code === 0x7f;
  });
}

/**
 * Where to send someone after signing in. `next` comes from the URL, so it is
 * only trusted when it is a path on this site: starts with one slash, no
 * scheme, no host, no backslash or control character, and not an auth page
 * (which would loop). Anything else falls back.
 */
export function safeNext(
  value: string | null | undefined,
  fallback = '/dashboard',
): string {
  if (!value || !value.startsWith('/') || value.startsWith('//'))
    return fallback;
  if (value.includes('\\') || hasControlCharacter(value)) return fallback;
  try {
    if (new URL(value, 'http://sd.invalid').origin !== 'http://sd.invalid') {
      return fallback;
    }
  } catch {
    return fallback;
  }
  return AUTH_PATHS.test(value) ? fallback : value;
}
