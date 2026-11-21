import type { Role } from './tokens';

const sessionListeners = new Set<() => void>();
const identityListeners = new Set<() => void>();

let identityVersion = 0;
let currentIdentity: string | null | undefined = undefined;
let currentRole: Role | null = null;

/** Called when a signed-in request could not be saved by renewing the session. */
export function onSessionExpired(listener: () => void): () => void {
  sessionListeners.add(listener);
  return () => sessionListeners.delete(listener);
}

/** Tell the app that the session is gone. Internal to the API layer. */
export function emitSessionExpired(): void {
  sessionListeners.forEach((listener) => listener());
}

/**
 * Records the current authentication identity and its verified role. The
 * version changes only when the identity actually changes, so the chat widget
 * can reset its conversation on sign-in, sign-out, user replacement or expiry
 * without resetting on every render. The role only ever comes from the
 * `GET /users/me` response; it is never derived from a URL or user input.
 */
export function emitSessionIdentity(
  userID: string | null,
  role: Role | null = null,
): void {
  const nextRole = userID === null ? null : role;
  if (currentIdentity === userID) {
    // A role-only correction (for example a re-check that returns the same
    // user) updates the snapshot without resetting the widget or its
    // conversation.
    if (currentRole !== nextRole) {
      currentRole = nextRole;
      identityListeners.forEach((listener) => listener());
    }
    return;
  }
  currentIdentity = userID;
  currentRole = userID === null ? null : role;
  identityVersion += 1;
  identityListeners.forEach((listener) => listener());
}

export function subscribeSessionIdentity(listener: () => void): () => void {
  identityListeners.add(listener);
  return () => identityListeners.delete(listener);
}

export function getSessionIdentityVersion(): number {
  return identityVersion;
}

export function getServerSessionIdentityVersion(): number {
  return 0;
}

/** The verified role for the current session, or null for a guest. */
export function getSessionRole(): Role | null {
  return currentRole;
}

/** Roles are never part of server rendering; the panel is client-only. */
export function getServerSessionRole(): Role | null {
  return null;
}
