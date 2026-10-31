CREATE TABLE IF NOT EXISTS identity.roles (
  id          SMALLINT     PRIMARY KEY,
  name        VARCHAR(30)  NOT NULL UNIQUE
              CHECK (name IN ('CUSTOMER','MERCHANT','COURIER','DISPATCHER','ADMIN')),
  description VARCHAR(255)
);


CREATE TABLE IF NOT EXISTS identity.users (
  id                 UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
  email              VARCHAR(255) NOT NULL,
  password_hash      VARCHAR(100),                       -- bcrypt, NULL for Google only accounts
  full_name          VARCHAR(150) NOT NULL CHECK (char_length(full_name) BETWEEN 2 AND 150),
  phone              VARCHAR(20),
  role_id            SMALLINT     NOT NULL REFERENCES identity.roles(id),
  auth_provider      VARCHAR(10)  NOT NULL DEFAULT 'LOCAL' CHECK (auth_provider IN ('LOCAL','GOOGLE')),
  google_sub         VARCHAR(100) UNIQUE,
  email_verified     BOOLEAN      NOT NULL DEFAULT false,
  is_active          BOOLEAN      NOT NULL DEFAULT true,
  failed_login_count INT          NOT NULL DEFAULT 0 CHECK (failed_login_count >= 0),
  locked_until       TIMESTAMPTZ,
  last_login_at      TIMESTAMPTZ,
  created_at         TIMESTAMPTZ  NOT NULL DEFAULT now(),
  updated_at         TIMESTAMPTZ  NOT NULL DEFAULT now(),
  -- ADDED: every account needs some way to sign in
  CONSTRAINT users_has_login_method CHECK (password_hash IS NOT NULL OR google_sub IS NOT NULL)
);
CREATE UNIQUE INDEX IF NOT EXISTS users_email_lower_uq ON identity.users (lower(email));
CREATE INDEX IF NOT EXISTS users_role_idx        ON identity.users (role_id);
CREATE INDEX IF NOT EXISTS users_role_active_idx ON identity.users (role_id, is_active);


CREATE TABLE IF NOT EXISTS identity.courier_profiles (
  user_id        UUID        PRIMARY KEY REFERENCES identity.users(id),
  vehicle_type   VARCHAR(30),
  vehicle_number VARCHAR(20),
  home_zone_id   UUID,                                   -- ref pricing.zones (no FK)
  is_available   BOOLEAN     NOT NULL DEFAULT true,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);


CREATE TABLE IF NOT EXISTS identity.refresh_tokens (
  id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id     UUID         NOT NULL REFERENCES identity.users(id),
  token_hash  CHAR(64)     NOT NULL UNIQUE,              -- SHA-256 hex, raw token never stored
  expires_at  TIMESTAMPTZ  NOT NULL,
  revoked_at  TIMESTAMPTZ,
  replaced_by UUID,
  user_agent  VARCHAR(255),
  ip_address  INET,
  created_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS refresh_tokens_user_idx    ON identity.refresh_tokens (user_id);
CREATE INDEX IF NOT EXISTS refresh_tokens_expires_idx ON identity.refresh_tokens (expires_at);


CREATE TABLE IF NOT EXISTS identity.oauth_states (
  state         VARCHAR(64)  PRIMARY KEY,
  code_verifier VARCHAR(128),
  redirect_to   VARCHAR(255),
  expires_at    TIMESTAMPTZ  NOT NULL,
  created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
  -- ADDED: only relative paths, so the login cannot redirect to another site
  CONSTRAINT oauth_redirect_relative CHECK (redirect_to IS NULL OR (redirect_to LIKE '/%' AND redirect_to NOT LIKE '//%'))
);
CREATE INDEX IF NOT EXISTS oauth_states_expires_idx ON identity.oauth_states (expires_at);


CREATE TABLE IF NOT EXISTS identity.password_reset_tokens (
  id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id      UUID        NOT NULL REFERENCES identity.users(id),
  token_hash   CHAR(64)    NOT NULL UNIQUE,              -- SHA-256 of the emailed token
  expires_at   TIMESTAMPTZ NOT NULL,                     -- 30 minutes after creation
  used_at      TIMESTAMPTZ,
  requested_ip INET,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS reset_tokens_user_idx    ON identity.password_reset_tokens (user_id);
CREATE INDEX IF NOT EXISTS reset_tokens_expires_idx ON identity.password_reset_tokens (expires_at);
-- ADDED: only the newest link works. Mark older unused tokens as used BEFORE inserting a new one.
CREATE UNIQUE INDEX IF NOT EXISTS reset_tokens_one_unused_uq
  ON identity.password_reset_tokens (user_id) WHERE used_at IS NULL;


CREATE TABLE IF NOT EXISTS identity.audit_logs (
  id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  actor_user_id UUID        NOT NULL,                    -- ref users
  action        VARCHAR(60) NOT NULL,
  entity_type   VARCHAR(40) NOT NULL,
  entity_id     UUID,
  details       JSONB,                                   -- never secrets, tokens or passwords
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS identity_audit_entity_idx  ON identity.audit_logs (entity_type, entity_id);
CREATE INDEX IF NOT EXISTS identity_audit_created_idx ON identity.audit_logs (created_at);


DO $$
DECLARE t text;
BEGIN
  FOREACH t IN ARRAY ARRAY[
    'identity.users','identity.courier_profiles'
  ] LOOP
    EXECUTE format('DROP TRIGGER IF EXISTS set_updated_at ON %s', t);
    EXECUTE format('CREATE TRIGGER set_updated_at BEFORE UPDATE ON %s FOR EACH ROW EXECUTE FUNCTION public.set_updated_at()', t);
  END LOOP;
END $$;


INSERT INTO identity.roles (id, name, description) VALUES
  (1, 'CUSTOMER',   'Sends and tracks parcels'),
  (2, 'MERCHANT',   'Online store that creates orders through the API'),
  (3, 'COURIER',    'Collects and delivers assigned parcels'),
  (4, 'DISPATCHER', 'Assigns couriers and monitors workload'),
  (5, 'ADMIN',      'Manages users, merchants, zones, pricing and reports')
ON CONFLICT DO NOTHING;
