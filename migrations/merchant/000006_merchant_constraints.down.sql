ALTER TABLE merchant.merchant_orders DROP CONSTRAINT IF EXISTS merchant_orders_currency_lkr;
ALTER TABLE merchant.merchant_orders DROP CONSTRAINT IF EXISTS merchant_orders_fee_breakdown_object;
ALTER TABLE merchant.merchant_orders DROP CONSTRAINT IF EXISTS merchant_orders_fee_non_negative;
ALTER TABLE merchant.merchant_orders DROP CONSTRAINT IF EXISTS merchant_orders_weight_positive;
ALTER TABLE merchant.merchant_orders DROP CONSTRAINT IF EXISTS merchant_orders_dropoff_city_not_blank;
ALTER TABLE merchant.merchant_orders DROP CONSTRAINT IF EXISTS merchant_orders_recipient_name_not_blank;
ALTER TABLE merchant.merchant_orders DROP CONSTRAINT IF EXISTS merchant_orders_tracking_number_not_blank;
ALTER TABLE merchant.merchant_orders DROP CONSTRAINT IF EXISTS merchant_orders_request_hash_format;
ALTER TABLE merchant.merchant_orders DROP CONSTRAINT IF EXISTS merchant_orders_idempotency_key_not_blank;
ALTER TABLE merchant.merchant_orders DROP CONSTRAINT IF EXISTS merchant_orders_external_order_id_not_blank;

ALTER TABLE merchant.merchant_api_clients DROP CONSTRAINT IF EXISTS merchant_api_clients_key_hash_format;
ALTER TABLE merchant.merchant_api_clients DROP CONSTRAINT IF EXISTS merchant_api_clients_key_prefix_not_blank;
ALTER TABLE merchant.merchant_api_clients DROP CONSTRAINT IF EXISTS merchant_api_clients_name_not_blank;

ALTER TABLE merchant.merchants DROP CONSTRAINT IF EXISTS merchants_rejected_reason;
ALTER TABLE merchant.merchants DROP CONSTRAINT IF EXISTS merchants_approved_fields;
ALTER TABLE merchant.merchants DROP CONSTRAINT IF EXISTS merchants_pickup_address_not_blank;
ALTER TABLE merchant.merchants DROP CONSTRAINT IF EXISTS merchants_contact_phone_not_blank;
ALTER TABLE merchant.merchants DROP CONSTRAINT IF EXISTS merchants_contact_email_not_blank;
ALTER TABLE merchant.merchants DROP CONSTRAINT IF EXISTS merchants_business_name_not_blank;
