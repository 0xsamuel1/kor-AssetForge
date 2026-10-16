ALTER TABLE webhook_delivery_logs
    DROP COLUMN IF EXISTS max_retries,
    DROP COLUMN IF EXISTS last_error,
    DROP COLUMN IF EXISTS retry_history,
    DROP COLUMN IF EXISTS dlq_reason;

DROP INDEX IF EXISTS idx_webhook_delivery_logs_dlq;
