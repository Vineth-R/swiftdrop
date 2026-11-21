import apiConfig from './api-config.json';

/**
 * Shared frontend constants.
 *
 * The API prefix lives in `api-config.json` because `next.config.js` runs in
 * Node and cannot import this TypeScript module. Both read the same file.
 */

/** The public API prefix, also used by the development proxy in next.config.js. */
export const API_BASE = apiConfig.base;

/** Where a person lands when no explicit, safe return path is given. */
export const DEFAULT_AUTH_ROUTE = '/dashboard';

/** Seconds to wait when a 429 carries no usable Retry-After header. */
export const RETRY_AFTER_FALLBACK_SECONDS = 60;

/** How long the "account created" screen waits before navigating. */
export const REGISTER_REDIRECT_SECONDS = 3;

/** How long the "password updated" screen waits before navigating. */
export const RESET_REDIRECT_SECONDS = 5;

/** How long the session splash waits before admitting the check is slow. */
export const SPLASH_SLOW_MS = 3000;

/** How often the countdown re-reads the clock, in milliseconds. */
export const COUNTDOWN_TICK_MS = 250;
