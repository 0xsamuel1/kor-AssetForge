CREATE TABLE IF NOT EXISTS fee_distribution_rules (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    platform_share_bps INT NOT NULL DEFAULT 0,
    liquidity_providers_share_bps INT NOT NULL DEFAULT 0,
    token_holders_share_bps INT NOT NULL DEFAULT 0,
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_fee_distribution_shares_sum CHECK (
        platform_share_bps + liquidity_providers_share_bps + token_holders_share_bps = 10000
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_fee_distribution_rules_active ON fee_distribution_rules(active) WHERE active = true;

CREATE TABLE IF NOT EXISTS fee_distribution_runs (
    id BIGSERIAL PRIMARY KEY,
    rule_id BIGINT NOT NULL REFERENCES fee_distribution_rules(id),
    period_start TIMESTAMPTZ NOT NULL,
    period_end TIMESTAMPTZ NOT NULL,
    total_fees_stroops NUMERIC(20, 8) NOT NULL DEFAULT 0,
    platform_amount_stroops NUMERIC(20, 8) NOT NULL DEFAULT 0,
    liquidity_providers_amount_stroops NUMERIC(20, 8) NOT NULL DEFAULT 0,
    token_holders_amount_stroops NUMERIC(20, 8) NOT NULL DEFAULT 0,
    recipient_count INT NOT NULL DEFAULT 0,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    triggered_by VARCHAR(20) NOT NULL DEFAULT 'manual',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_fee_distribution_runs_period ON fee_distribution_runs(period_start, period_end);
CREATE INDEX IF NOT EXISTS idx_fee_distribution_runs_status ON fee_distribution_runs(status);

CREATE TABLE IF NOT EXISTS fee_distribution_allocations (
    id BIGSERIAL PRIMARY KEY,
    run_id BIGINT NOT NULL REFERENCES fee_distribution_runs(id) ON DELETE CASCADE,
    recipient_type VARCHAR(20) NOT NULL,
    recipient_address VARCHAR(56) NOT NULL,
    amount_stroops NUMERIC(20, 8) NOT NULL,
    tx_hash VARCHAR(255),
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_fee_distribution_allocations_run_id ON fee_distribution_allocations(run_id);
CREATE INDEX IF NOT EXISTS idx_fee_distribution_allocations_recipient ON fee_distribution_allocations(recipient_address);
