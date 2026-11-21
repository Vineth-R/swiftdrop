-- Merchant-specific constraints from the merchant service design that the
-- M1 baseline (000002_tables) does not already enforce.
--
-- The duplicate merchant migrations (000002_merchants, 000003_api_clients,
-- 000004_orders_and_events, 000005_audit_logs) were removed because their
-- version numbers collided with the applied baseline and they re-created
-- tables that already exist. This forward migration preserves the useful
-- stricter checks as additive constraints only.
--
-- Constraints are added NOT VALID and then validated in the same transaction:
-- existing rows are not locked exclusively while being checked, but invalid
-- data still fails the migration loudly instead of being silently accepted.

-- merchant.merchants ---------------------------------------------------------

ALTER TABLE merchant.merchants
    ADD CONSTRAINT merchants_business_name_not_blank
    CHECK (btrim(business_name) <> '') NOT VALID;
ALTER TABLE merchant.merchants VALIDATE CONSTRAINT merchants_business_name_not_blank;

ALTER TABLE merchant.merchants
    ADD CONSTRAINT merchants_contact_email_not_blank
    CHECK (btrim(contact_email) <> '') NOT VALID;
ALTER TABLE merchant.merchants VALIDATE CONSTRAINT merchants_contact_email_not_blank;

ALTER TABLE merchant.merchants
    ADD CONSTRAINT merchants_contact_phone_not_blank
    CHECK (btrim(contact_phone) <> '') NOT VALID;
ALTER TABLE merchant.merchants VALIDATE CONSTRAINT merchants_contact_phone_not_blank;

ALTER TABLE merchant.merchants
    ADD CONSTRAINT merchants_pickup_address_not_blank
    CHECK (btrim(pickup_address) <> '') NOT VALID;
ALTER TABLE merchant.merchants VALIDATE CONSTRAINT merchants_pickup_address_not_blank;

ALTER TABLE merchant.merchants
    ADD CONSTRAINT merchants_approved_fields
    CHECK (status <> 'APPROVED' OR (approved_by IS NOT NULL AND approved_at IS NOT NULL)) NOT VALID;
ALTER TABLE merchant.merchants VALIDATE CONSTRAINT merchants_approved_fields;

ALTER TABLE merchant.merchants
    ADD CONSTRAINT merchants_rejected_reason
    CHECK (status <> 'REJECTED' OR rejection_reason IS NOT NULL) NOT VALID;
ALTER TABLE merchant.merchants VALIDATE CONSTRAINT merchants_rejected_reason;

-- merchant.merchant_api_clients ---------------------------------------------

ALTER TABLE merchant.merchant_api_clients
    ADD CONSTRAINT merchant_api_clients_name_not_blank
    CHECK (btrim(name) <> '') NOT VALID;
ALTER TABLE merchant.merchant_api_clients VALIDATE CONSTRAINT merchant_api_clients_name_not_blank;

ALTER TABLE merchant.merchant_api_clients
    ADD CONSTRAINT merchant_api_clients_key_prefix_not_blank
    CHECK (btrim(key_prefix) <> '') NOT VALID;
ALTER TABLE merchant.merchant_api_clients VALIDATE CONSTRAINT merchant_api_clients_key_prefix_not_blank;

ALTER TABLE merchant.merchant_api_clients
    ADD CONSTRAINT merchant_api_clients_key_hash_format
    CHECK (key_hash ~ '^[0-9a-f]{64}$') NOT VALID;
ALTER TABLE merchant.merchant_api_clients VALIDATE CONSTRAINT merchant_api_clients_key_hash_format;

-- merchant.merchant_orders --------------------------------------------------

ALTER TABLE merchant.merchant_orders
    ADD CONSTRAINT merchant_orders_external_order_id_not_blank
    CHECK (btrim(external_order_id) <> '') NOT VALID;
ALTER TABLE merchant.merchant_orders VALIDATE CONSTRAINT merchant_orders_external_order_id_not_blank;

ALTER TABLE merchant.merchant_orders
    ADD CONSTRAINT merchant_orders_idempotency_key_not_blank
    CHECK (idempotency_key IS NULL OR btrim(idempotency_key) <> '') NOT VALID;
ALTER TABLE merchant.merchant_orders VALIDATE CONSTRAINT merchant_orders_idempotency_key_not_blank;

ALTER TABLE merchant.merchant_orders
    ADD CONSTRAINT merchant_orders_request_hash_format
    CHECK (request_hash ~ '^[0-9a-f]{64}$') NOT VALID;
ALTER TABLE merchant.merchant_orders VALIDATE CONSTRAINT merchant_orders_request_hash_format;

ALTER TABLE merchant.merchant_orders
    ADD CONSTRAINT merchant_orders_tracking_number_not_blank
    CHECK (btrim(tracking_number) <> '') NOT VALID;
ALTER TABLE merchant.merchant_orders VALIDATE CONSTRAINT merchant_orders_tracking_number_not_blank;

ALTER TABLE merchant.merchant_orders
    ADD CONSTRAINT merchant_orders_recipient_name_not_blank
    CHECK (btrim(recipient_name) <> '') NOT VALID;
ALTER TABLE merchant.merchant_orders VALIDATE CONSTRAINT merchant_orders_recipient_name_not_blank;

ALTER TABLE merchant.merchant_orders
    ADD CONSTRAINT merchant_orders_dropoff_city_not_blank
    CHECK (btrim(dropoff_city) <> '') NOT VALID;
ALTER TABLE merchant.merchant_orders VALIDATE CONSTRAINT merchant_orders_dropoff_city_not_blank;

ALTER TABLE merchant.merchant_orders
    ADD CONSTRAINT merchant_orders_weight_positive
    CHECK (weight_kg > 0) NOT VALID;
ALTER TABLE merchant.merchant_orders VALIDATE CONSTRAINT merchant_orders_weight_positive;

ALTER TABLE merchant.merchant_orders
    ADD CONSTRAINT merchant_orders_fee_non_negative
    CHECK (fee_amount >= 0) NOT VALID;
ALTER TABLE merchant.merchant_orders VALIDATE CONSTRAINT merchant_orders_fee_non_negative;

ALTER TABLE merchant.merchant_orders
    ADD CONSTRAINT merchant_orders_fee_breakdown_object
    CHECK (jsonb_typeof(fee_breakdown) = 'object') NOT VALID;
ALTER TABLE merchant.merchant_orders VALIDATE CONSTRAINT merchant_orders_fee_breakdown_object;

ALTER TABLE merchant.merchant_orders
    ADD CONSTRAINT merchant_orders_currency_lkr
    CHECK (currency = 'LKR') NOT VALID;
ALTER TABLE merchant.merchant_orders VALIDATE CONSTRAINT merchant_orders_currency_lkr;
