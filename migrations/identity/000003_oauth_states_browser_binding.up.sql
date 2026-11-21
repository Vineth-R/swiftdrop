-- Bind a Google OAuth login attempt to the browser that started it.
-- Only the SHA-256 hash of the browser-binding cookie value is stored.
-- Existing rows keep NULL and cannot be consumed by the new callback flow;
-- they expire within the normal 10 minute state lifetime.
ALTER TABLE identity.oauth_states
  ADD COLUMN IF NOT EXISTS browser_binding_hash CHAR(64);
