CREATE TABLE IF NOT EXISTS identity.email_verification_tokens (
  id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id      UUID        NOT NULL REFERENCES identity.users(id) ON DELETE CASCADE,
  token_hash   CHAR(64)    NOT NULL UNIQUE,
  expires_at   TIMESTAMPTZ NOT NULL,
  used_at      TIMESTAMPTZ,
  requested_ip INET,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS email_verification_tokens_user_idx ON identity.email_verification_tokens (user_id);
CREATE INDEX IF NOT EXISTS email_verification_tokens_expires_idx ON identity.email_verification_tokens (expires_at);
CREATE UNIQUE INDEX IF NOT EXISTS email_verification_tokens_one_unused_uq
  ON identity.email_verification_tokens (user_id) WHERE used_at IS NULL;
