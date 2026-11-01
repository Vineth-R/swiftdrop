CREATE TABLE merchant.merchants (
    id UUID PRIMARY KEY,
    owner_user_id UUID NOT NULL UNIQUE,
    business_name VARCHAR(150) NOT NULL CHECK (btrim(business_name) <> ''),
    business_reg_no VARCHAR(50),
    contact_email VARCHAR(255) NOT NULL CHECK (btrim(contact_email) <> ''),
    contact_phone VARCHAR(20) NOT NULL CHECK (btrim(contact_phone) <> ''),
    pickup_address VARCHAR(255) NOT NULL CHECK (btrim(pickup_address) <> ''),
    pickup_zone_id UUID NOT NULL,
    status VARCHAR(12) NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'APPROVED', 'REJECTED', 'SUSPENDED')),
    approved_by UUID,
    approved_at TIMESTAMPTZ,
    rejection_reason VARCHAR(255),
    suspension_reason VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((status = 'APPROVED' AND approved_by IS NOT NULL AND approved_at IS NOT NULL) OR status <> 'APPROVED'),
    CHECK (status <> 'REJECTED' OR rejection_reason IS NOT NULL),
    CHECK (status <> 'SUSPENDED' OR suspension_reason IS NOT NULL)
);

CREATE INDEX merchants_status_created_at_idx ON merchant.merchants (status, created_at DESC);
