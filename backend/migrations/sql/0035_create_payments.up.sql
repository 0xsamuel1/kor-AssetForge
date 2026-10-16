-- Fiat payment gateway integration tables
CREATE TABLE IF NOT EXISTS payments (
    id                  BIGSERIAL    PRIMARY KEY,
    user_id             BIGINT       NOT NULL REFERENCES users(id),
    asset_id            BIGINT       NOT NULL REFERENCES assets(id),
    gateway             VARCHAR(20)  NOT NULL,
    gateway_payment_id  VARCHAR(255) NOT NULL UNIQUE,
    amount_fiat         BIGINT       NOT NULL,
    currency            VARCHAR(3)   NOT NULL DEFAULT 'USD',
    token_amount        BIGINT       NOT NULL,
    status              VARCHAR(20)  NOT NULL DEFAULT 'pending',
    failure_reason      TEXT,
    webhook_payload     TEXT,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at          TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS payment_reconciliations (
    id              BIGSERIAL    PRIMARY KEY,
    gateway         VARCHAR(20)  NOT NULL,
    period_start    TIMESTAMPTZ  NOT NULL,
    period_end      TIMESTAMPTZ  NOT NULL,
    total_payments  INT          NOT NULL DEFAULT 0,
    total_amount    BIGINT       NOT NULL DEFAULT 0,
    discrepancies   INT          NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_payments_user_id    ON payments(user_id);
CREATE INDEX IF NOT EXISTS idx_payments_asset_id   ON payments(asset_id);
CREATE INDEX IF NOT EXISTS idx_payments_status     ON payments(status);
CREATE INDEX IF NOT EXISTS idx_payments_gateway    ON payments(gateway);
CREATE INDEX IF NOT EXISTS idx_payments_deleted_at ON payments(deleted_at);
