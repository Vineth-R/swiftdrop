CREATE TABLE IF NOT EXISTS identity.merchant_lifecycle_transitions (
  transition_id UUID        PRIMARY KEY,
  user_id       UUID        NOT NULL REFERENCES identity.users(id) ON DELETE CASCADE,
  sequence      BIGINT      NOT NULL CHECK (sequence > 0),
  operation     VARCHAR(16) NOT NULL CHECK (operation IN ('APPROVE','REJECT','SUSPEND')),
  applied_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT merchant_lifecycle_user_sequence_uq UNIQUE (user_id, sequence)
);
CREATE INDEX IF NOT EXISTS merchant_lifecycle_user_idx
  ON identity.merchant_lifecycle_transitions (user_id);
