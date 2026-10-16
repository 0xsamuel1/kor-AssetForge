CREATE TABLE IF NOT EXISTS scheduled_reports (
    id                BIGSERIAL PRIMARY KEY,
    user_id           BIGINT       NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name              VARCHAR(160) NOT NULL,
    report_type       VARCHAR(64)  NOT NULL,
    frequency         VARCHAR(16)  NOT NULL CHECK (frequency IN ('daily', 'weekly', 'monthly')),
    cron_expression   VARCHAR(128) NOT NULL,
    timezone          VARCHAR(64)  NOT NULL DEFAULT 'UTC',
    format            VARCHAR(16)  NOT NULL DEFAULT 'pdf' CHECK (format IN ('pdf', 'csv', 'excel')),
    delivery_method   VARCHAR(16)  NOT NULL CHECK (delivery_method IN ('email', 'webhook')),
    email_recipients  TEXT,
    webhook_url       TEXT,
    filters           JSONB        NOT NULL DEFAULT '{}',
    retention_days    INT         NOT NULL DEFAULT 90 CHECK (retention_days > 0 AND retention_days <= 365),
    status            VARCHAR(16)  NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'paused', 'disabled')),
    last_run_at       TIMESTAMPTZ,
    next_run_at       TIMESTAMPTZ,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at        TIMESTAMPTZ,
    CONSTRAINT chk_scheduled_reports_delivery_target CHECK (
        (delivery_method = 'email' AND email_recipients IS NOT NULL AND email_recipients <> '')
        OR
        (delivery_method = 'webhook' AND webhook_url IS NOT NULL AND webhook_url <> '')
    )
);

CREATE INDEX IF NOT EXISTS idx_scheduled_reports_user_id ON scheduled_reports(user_id);
CREATE INDEX IF NOT EXISTS idx_scheduled_reports_status ON scheduled_reports(status);
CREATE INDEX IF NOT EXISTS idx_scheduled_reports_report_type ON scheduled_reports(report_type);
CREATE INDEX IF NOT EXISTS idx_scheduled_reports_next_run_at ON scheduled_reports(next_run_at);
CREATE INDEX IF NOT EXISTS idx_scheduled_reports_deleted_at ON scheduled_reports(deleted_at);

CREATE TABLE IF NOT EXISTS report_delivery_histories (
    id                  BIGSERIAL PRIMARY KEY,
    scheduled_report_id BIGINT      NOT NULL REFERENCES scheduled_reports(id) ON DELETE CASCADE,
    user_id             BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    report_type         VARCHAR(64) NOT NULL,
    format              VARCHAR(16) NOT NULL CHECK (format IN ('pdf', 'csv', 'excel')),
    delivery_method     VARCHAR(16) NOT NULL CHECK (delivery_method IN ('email', 'webhook')),
    status              VARCHAR(16) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'success', 'failed')),
    file_name           VARCHAR(255) NOT NULL,
    file_path           TEXT,
    payload             TEXT,
    error_message       TEXT,
    generated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    delivered_at        TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_report_delivery_histories_schedule_id ON report_delivery_histories(scheduled_report_id);
CREATE INDEX IF NOT EXISTS idx_report_delivery_histories_user_id ON report_delivery_histories(user_id);
CREATE INDEX IF NOT EXISTS idx_report_delivery_histories_status ON report_delivery_histories(status);
CREATE INDEX IF NOT EXISTS idx_report_delivery_histories_created_at ON report_delivery_histories(created_at);
