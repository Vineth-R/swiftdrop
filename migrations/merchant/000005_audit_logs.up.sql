CREATE TABLE merchant.audit_logs (
    id UUID PRIMARY KEY,
    actor_user_id UUID NOT NULL,
    action VARCHAR(60) NOT NULL,
    entity_type VARCHAR(40) NOT NULL,
    entity_id UUID,
    details JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX audit_logs_entity_idx ON merchant.audit_logs (entity_type, entity_id);
CREATE INDEX audit_logs_created_at_idx ON merchant.audit_logs (created_at DESC);
