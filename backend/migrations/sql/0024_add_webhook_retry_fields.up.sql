-- Add retry tracking fields to webhook_delivery_logs
ALTER TABLE webhook_delivery_logs
    ADD COLUMN IF NOT EXISTS max_retries    INT       NOT NULL DEFAULT 5,
    ADD COLUMN IF NOT EXISTS last_error     TEXT,
    ADD COLUMN IF NOT EXISTS retry_history  JSONB,
    ADD COLUMN IF NOT EXISTS dlq_reason     TEXT;

CREATE INDEX IF NOT EXISTS idx_webhook_delivery_logs_dlq
    ON webhook_delivery_logs(status)
    WHERE status = 'abandoned' AND dlq_reason IS NOT NULL;
