const sessionListeners = new Set<() => void>();

/** Called when a signed-in request could not be saved by renewing the session. */
export function onSessionExpired(listener: () => void): () => void {
  sessionListeners.add(listener);
  return () => sessionListeners.delete(listener);
}

/** Tell the app that the session is gone. Internal to the API layer. */
export function emitSessionExpired(): void {
  sessionListeners.forEach((listener) => listener());
}
