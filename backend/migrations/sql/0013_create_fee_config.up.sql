CREATE TABLE IF NOT EXISTS fee_configs (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    fee_type VARCHAR(64) NOT NULL,
    base_basis_points INT NOT NULL,
    min_basis_points INT NOT NULL DEFAULT 0,
    max_basis_points INT NOT NULL,
    volume_tiers TEXT NOT NULL DEFAULT '[]',
    active BOOLEAN NOT NULL DEFAULT true,
    effective_from TIMESTAMPTZ NOT NULL,
    effective_until TIMESTAMPTZ,
    created_by_admin_id BIGINT NOT NULL,
    updated_by_admin_id BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT fee_configs_name_key UNIQUE (name)
);

CREATE INDEX IF NOT EXISTS idx_fee_configs_fee_type ON fee_configs (fee_type);
CREATE INDEX IF NOT EXISTS idx_fee_configs_active ON fee_configs (active);
CREATE INDEX IF NOT EXISTS idx_fee_configs_deleted_at ON fee_configs (deleted_at);

CREATE TABLE IF NOT EXISTS fee_audit_logs (
    id BIGSERIAL PRIMARY KEY,
    fee_config_id BIGINT NOT NULL REFERENCES fee_configs(id),
    admin_id BIGINT NOT NULL,
    action VARCHAR(64) NOT NULL,
    previous_json TEXT,
    new_json TEXT,
    reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_fee_audit_logs_fee_config_id ON fee_audit_logs (fee_config_id);
CREATE INDEX IF NOT EXISTS idx_fee_audit_logs_admin_id ON fee_audit_logs (admin_id);
CREATE INDEX IF NOT EXISTS idx_fee_audit_logs_created_at ON fee_audit_logs (created_at DESC);
