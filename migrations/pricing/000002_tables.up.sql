CREATE TABLE IF NOT EXISTS pricing.zones (
  id         UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
  code       VARCHAR(20)  NOT NULL UNIQUE,               -- 409 ZONE_CODE_EXISTS
  name       VARCHAR(100) NOT NULL,
  districts  TEXT[],
  is_active  BOOLEAN      NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);


CREATE TABLE IF NOT EXISTS pricing.pricing_rules (
  id                  UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
  origin_zone_id      UUID          NOT NULL REFERENCES pricing.zones(id),
  destination_zone_id UUID          NOT NULL REFERENCES pricing.zones(id),
  base_fee            NUMERIC(12,2) NOT NULL CHECK (base_fee >= 0),
  currency            CHAR(3)       NOT NULL DEFAULT 'LKR',
  is_active           BOOLEAN       NOT NULL DEFAULT true,
  updated_by          UUID,                              -- admin user (ref users)
  created_at          TIMESTAMPTZ   NOT NULL DEFAULT now(),
  updated_at          TIMESTAMPTZ   NOT NULL DEFAULT now()
);
-- one active rule per zone pair (409 RULE_EXISTS)
CREATE UNIQUE INDEX IF NOT EXISTS pricing_rules_active_pair_uq
  ON pricing.pricing_rules (origin_zone_id, destination_zone_id) WHERE is_active;


CREATE TABLE IF NOT EXISTS pricing.weight_bands (
  id            UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
  min_weight_kg NUMERIC(6,2)  NOT NULL CHECK (min_weight_kg >= 0),   -- exclusive
  max_weight_kg NUMERIC(6,2)  NOT NULL,                              -- inclusive
  charge        NUMERIC(12,2) NOT NULL CHECK (charge >= 0),
  is_active     BOOLEAN       NOT NULL DEFAULT true,
  created_at    TIMESTAMPTZ   NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ   NOT NULL DEFAULT now(),
  CONSTRAINT weight_bands_range CHECK (max_weight_kg > min_weight_kg),
  -- active bands must not overlap (409 BAND_OVERLAP)
  CONSTRAINT weight_bands_no_overlap
    EXCLUDE USING gist (numrange(min_weight_kg, max_weight_kg, '(]') WITH &&) WHERE (is_active)
);


CREATE TABLE IF NOT EXISTS pricing.size_surcharges (
  id            UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
  size_category VARCHAR(12)   NOT NULL UNIQUE
                CHECK (size_category IN ('SMALL','MEDIUM','LARGE','OVERSIZED')),
  surcharge     NUMERIC(12,2) NOT NULL CHECK (surcharge >= 0),
  updated_at    TIMESTAMPTZ   NOT NULL DEFAULT now()
);


CREATE TABLE IF NOT EXISTS pricing.audit_logs (
  id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  actor_user_id UUID        NOT NULL,
  action        VARCHAR(60) NOT NULL,
  entity_type   VARCHAR(40) NOT NULL,
  entity_id     UUID,
  details       JSONB,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS pricing_audit_entity_idx  ON pricing.audit_logs (entity_type, entity_id);
CREATE INDEX IF NOT EXISTS pricing_audit_created_idx ON pricing.audit_logs (created_at);


DO $$
DECLARE t text;
BEGIN
  FOREACH t IN ARRAY ARRAY[
    'pricing.zones','pricing.pricing_rules','pricing.weight_bands','pricing.size_surcharges'
  ] LOOP
    EXECUTE format('DROP TRIGGER IF EXISTS set_updated_at ON %s', t);
    EXECUTE format('CREATE TRIGGER set_updated_at BEFORE UPDATE ON %s FOR EACH ROW EXECUTE FUNCTION public.set_updated_at()', t);
  END LOOP;
END $$;


-- Zone codes CMB, KDY, GLE come from the document; GMP and JFN are chosen here.
INSERT INTO pricing.zones (code, name, districts) VALUES
  ('CMB', 'Colombo', ARRAY['Colombo']),
  ('GMP', 'Gampaha', ARRAY['Gampaha','Negombo']),
  ('KDY', 'Kandy',   ARRAY['Kandy']),
  ('GLE', 'Galle',   ARRAY['Galle']),
  ('JFN', 'Jaffna',  ARRAY['Jaffna'])
ON CONFLICT DO NOTHING;


-- Illustrative LKR base fees, same fee both directions. CMB to KDY = 450.00 as in the API example.
WITH fees(a, b, fee) AS (VALUES
  ('CMB','CMB',250.00), ('GMP','GMP',250.00), ('KDY','KDY',250.00), ('GLE','GLE',250.00), ('JFN','JFN',250.00),
  ('CMB','GMP',300.00), ('CMB','KDY',450.00), ('CMB','GLE',450.00), ('CMB','JFN',750.00),
  ('GMP','KDY',450.00), ('GMP','GLE',500.00), ('GMP','JFN',700.00),
  ('KDY','GLE',600.00), ('KDY','JFN',650.00),
  ('GLE','JFN',850.00)),
pairs AS (
  SELECT a, b, fee FROM fees
  UNION
  SELECT b, a, fee FROM fees)
INSERT INTO pricing.pricing_rules (origin_zone_id, destination_zone_id, base_fee)
SELECT zo.id, zd.id, p.fee
FROM pairs p
JOIN pricing.zones zo ON zo.code = p.a
JOIN pricing.zones zd ON zd.code = p.b
ON CONFLICT DO NOTHING;


-- Lower bound exclusive, upper bound inclusive. The 10 to 25 kg charge is illustrative.
INSERT INTO pricing.weight_bands (min_weight_kg, max_weight_kg, charge) VALUES
  (0,  1,  0.00),
  (1,  5,  100.00),
  (5,  10, 250.00),
  (10, 25, 500.00)
ON CONFLICT DO NOTHING;


INSERT INTO pricing.size_surcharges (size_category, surcharge) VALUES
  ('SMALL', 0.00), ('MEDIUM', 150.00), ('LARGE', 400.00), ('OVERSIZED', 900.00)
ON CONFLICT DO NOTHING;
