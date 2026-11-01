DROP TRIGGER IF EXISTS api_clients_key_limit ON merchant.merchant_api_clients;
DROP FUNCTION IF EXISTS merchant.enforce_active_key_limit();

DROP TABLE IF EXISTS merchant.webhook_delivery_attempts;
DROP TABLE IF EXISTS merchant.merchant_order_status_events;
DROP TABLE IF EXISTS merchant.merchant_orders;
DROP TABLE IF EXISTS merchant.merchant_api_clients;
DROP TABLE IF EXISTS merchant.merchant_webhooks;
DROP TABLE IF EXISTS merchant.merchants;
DROP TABLE IF EXISTS merchant.audit_logs;
