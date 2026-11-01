ALTER TABLE identity.oauth_states
  DROP COLUMN IF EXISTS browser_binding_hash;
