const sessionListeners = new Set<() => void>();
const identityListeners = new Set<() => void>();

let identityVersion = 0;
let currentIdentity: string | null | undefined = undefined;

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
 * Records the current authentication identity. The version changes only when
 * the identity actually changes, so the chat widget can reset its conversation
 * on sign-in, sign-out, user replacement or expiry without resetting on every
 * render.
 */
export function emitSessionIdentity(userID: string | null): void {
  if (currentIdentity === userID) return;
  currentIdentity = userID;
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
