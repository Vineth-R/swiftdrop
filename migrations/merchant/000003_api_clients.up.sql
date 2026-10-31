CREATE TABLE merchant.merchant_api_clients (
    id UUID PRIMARY KEY,
    merchant_id UUID NOT NULL REFERENCES merchant.merchants (id),
    name VARCHAR(100) NOT NULL CHECK (btrim(name) <> ''),
    key_prefix VARCHAR(20) NOT NULL CHECK (btrim(key_prefix) <> ''),
    lookup_id VARCHAR(16) NOT NULL UNIQUE,
    key_hash CHAR(64) NOT NULL CHECK (key_hash ~ '^[0-9a-f]{64}$'),
    status VARCHAR(10) NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'REVOKED')),
    last_used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ
);

CREATE INDEX merchant_api_clients_merchant_status_idx ON merchant.merchant_api_clients (merchant_id, status);
